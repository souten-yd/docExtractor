package jobs

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type State string

const (
	Queued    State = "queued"
	Running   State = "running"
	Succeeded State = "succeeded"
	Failed    State = "failed"
	Cancelled State = "cancelled"
)

type Task struct {
	Source       string `json:"source"`
	Destination  string `json:"destination"`
	DeleteSource bool   `json:"delete_source"`
}

type Job struct {
	ID           string    `json:"id"`
	Task         Task      `json:"task"`
	State        State     `json:"state"`
	Stage        string    `json:"stage,omitempty"`
	Progress     float64   `json:"progress"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	FinishedAt   time.Time `json:"finished_at,omitempty"`
	BytesRead    int64     `json:"bytes_read,omitempty"`
	BytesWritten int64     `json:"bytes_written,omitempty"`
}

type Update struct {
	Stage        string
	Progress     float64
	BytesRead    int64
	BytesWritten int64
}

type Processor func(ctx context.Context, jobID string, task Task, update func(Update)) error

type item struct {
	id   string
	task Task
}

type Manager struct {
	mu        sync.RWMutex
	jobs      map[string]*Job
	cancel    map[string]context.CancelFunc
	queue     chan item
	processor Processor
	rootCtx   context.Context
	stop      context.CancelFunc
	wg        sync.WaitGroup
	seq       atomic.Uint64
	workers   int
}

func New(workers, queueSize int, processor Processor) (*Manager, error) {
	if processor == nil {
		return nil, errors.New("processor is required")
	}
	if workers < 1 {
		workers = 1
	}
	if workers > 3 {
		workers = 3
	}
	if queueSize < workers {
		queueSize = workers * 8
	}
	rootCtx, stop := context.WithCancel(context.Background())
	m := &Manager{
		jobs:      make(map[string]*Job),
		cancel:    make(map[string]context.CancelFunc),
		queue:     make(chan item, queueSize),
		processor: processor,
		rootCtx:   rootCtx,
		stop:      stop,
		workers:   workers,
	}
	for i := 0; i < workers; i++ {
		m.wg.Add(1)
		go m.worker()
	}
	return m, nil
}

func (m *Manager) Submit(task Task) (Job, error) {
	if task.Source == "" || task.Destination == "" {
		return Job{}, errors.New("source and destination are required")
	}
	id := fmt.Sprintf("%d-%06d", time.Now().UTC().Unix(), m.seq.Add(1))
	now := time.Now().UTC()
	j := &Job{ID: id, Task: task, State: Queued, CreatedAt: now}
	m.mu.Lock()
	m.jobs[id] = j
	m.mu.Unlock()

	select {
	case m.queue <- item{id: id, task: task}:
		return cloneJob(j), nil
	default:
		m.mu.Lock()
		delete(m.jobs, id)
		m.mu.Unlock()
		return Job{}, errors.New("job queue is full")
	}
}

func (m *Manager) List() []Job {
	m.mu.RLock()
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, cloneJob(j))
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (m *Manager) Get(id string) (Job, bool) {
	m.mu.RLock()
	j, ok := m.jobs[id]
	if ok {
		cp := cloneJob(j)
		m.mu.RUnlock()
		return cp, true
	}
	m.mu.RUnlock()
	return Job{}, false
}

func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok || j.State == Succeeded || j.State == Failed || j.State == Cancelled {
		return false
	}
	if cancel := m.cancel[id]; cancel != nil {
		cancel()
	}
	if j.State == Queued {
		j.State = Cancelled
		j.FinishedAt = time.Now().UTC()
	}
	return true
}

func (m *Manager) Workers() int { return m.workers }

func (m *Manager) Close() {
	m.stop()
	m.wg.Wait()
}

func (m *Manager) worker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.rootCtx.Done():
			return
		case it := <-m.queue:
			m.run(it)
		}
	}
}

func (m *Manager) run(it item) {
	m.mu.Lock()
	j := m.jobs[it.id]
	if j == nil || j.State == Cancelled {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.rootCtx)
	m.cancel[it.id] = cancel
	j.State = Running
	j.StartedAt = time.Now().UTC()
	m.mu.Unlock()

	err := m.processor(ctx, it.id, it.task, func(u Update) {
		m.mu.Lock()
		if j := m.jobs[it.id]; j != nil && j.State == Running {
			j.Stage = u.Stage
			if u.Progress >= 0 && u.Progress <= 1 {
				j.Progress = u.Progress
			}
			if u.BytesRead >= 0 {
				j.BytesRead = u.BytesRead
			}
			if u.BytesWritten >= 0 {
				j.BytesWritten = u.BytesWritten
			}
		}
		m.mu.Unlock()
	})

	cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancel, it.id)
	j = m.jobs[it.id]
	if j == nil {
		return
	}
	j.FinishedAt = time.Now().UTC()
	if errors.Is(err, context.Canceled) {
		j.State = Cancelled
		j.Error = "cancelled"
	} else if err != nil {
		j.State = Failed
		j.Error = err.Error()
	} else {
		j.State = Succeeded
		j.Progress = 1
		j.Stage = "done"
	}
}

func cloneJob(j *Job) Job { return *j }
