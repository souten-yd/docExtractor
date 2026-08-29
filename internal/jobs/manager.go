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
	Overwrite    bool   `json:"overwrite,omitempty"`
}

type Job struct {
	ID             string    `json:"id"`
	Task           Task      `json:"task"`
	State          State     `json:"state"`
	Stage          string    `json:"stage,omitempty"`
	Progress       float64   `json:"progress"`
	Error          string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	FinishedAt     time.Time `json:"finished_at,omitempty"`
	Worker         int       `json:"worker,omitempty"`
	QueueWaitMS    int64     `json:"queue_wait_ms,omitempty"`
	DurationMS     int64     `json:"duration_ms,omitempty"`
	BytesRead      int64     `json:"bytes_read,omitempty"`
	BytesWritten   int64     `json:"bytes_written,omitempty"`
	ReadMiBPerSec  float64   `json:"read_mib_per_sec,omitempty"`
	WriteMiBPerSec float64   `json:"write_mib_per_sec,omitempty"`
	IOMiBPerSec    float64   `json:"io_mib_per_sec,omitempty"`
	Operation      string    `json:"operation,omitempty"`
	Entries        int       `json:"entries,omitempty"`
}

type Update struct { Stage string; Progress float64; BytesRead int64; BytesWritten int64; Operation string; Entries int }
type Summary struct { Workers int `json:"workers"`; Total int `json:"total"`; Queued int `json:"queued"`; Running int `json:"running"`; Succeeded int `json:"succeeded"`; Failed int `json:"failed"`; Cancelled int `json:"cancelled"`; BytesRead int64 `json:"bytes_read"`; BytesWritten int64 `json:"bytes_written"`; ActiveIOMiBPerSec float64 `json:"active_io_mib_per_sec"`; CompletedIOMiBPerSec float64 `json:"completed_io_mib_per_sec"`; MeasuredJobs int `json:"measured_jobs"` }
type Processor func(ctx context.Context, jobID string, task Task, update func(Update)) error
type item struct { id string; task Task }
type Manager struct { mu sync.RWMutex; jobs map[string]*Job; cancel map[string]context.CancelFunc; queue chan item; processor Processor; rootCtx context.Context; stop context.CancelFunc; wg sync.WaitGroup; seq atomic.Uint64; workers int }

func New(workers, queueSize int, processor Processor) (*Manager, error) { if processor==nil{return nil,errors.New("processor is required")};if workers<1{workers=1};if workers>3{workers=3};if queueSize<workers{queueSize=workers*8};rootCtx,stop:=context.WithCancel(context.Background());m:=&Manager{jobs:make(map[string]*Job),cancel:make(map[string]context.CancelFunc),queue:make(chan item,queueSize),processor:processor,rootCtx:rootCtx,stop:stop,workers:workers};for i:=0;i<workers;i++{m.wg.Add(1);go m.worker(i+1)};return m,nil }
func (m *Manager) Submit(task Task)(Job,error){if task.Source==""||task.Destination==""{return Job{},errors.New("source and destination are required")};id:=fmt.Sprintf("%d-%06d",time.Now().UTC().Unix(),m.seq.Add(1));now:=time.Now().UTC();j:=&Job{ID:id,Task:task,State:Queued,CreatedAt:now};m.mu.Lock();m.jobs[id]=j;m.mu.Unlock();select{case m.queue<-item{id:id,task:task}:return decorateJob(j,now),nil;default:m.mu.Lock();delete(m.jobs,id);m.mu.Unlock();return Job{},errors.New("job queue is full")}}
func (m *Manager) List()[]Job{now:=time.Now().UTC();m.mu.RLock();out:=make([]Job,0,len(m.jobs));for _,j:=range m.jobs{out=append(out,decorateJob(j,now))};m.mu.RUnlock();sort.Slice(out,func(i,j int)bool{return out[i].CreatedAt.After(out[j].CreatedAt)});return out}
func (m *Manager) Get(id string)(Job,bool){m.mu.RLock();j,ok:=m.jobs[id];if ok{cp:=decorateJob(j,time.Now().UTC());m.mu.RUnlock();return cp,true};m.mu.RUnlock();return Job{},false}
func (m *Manager) Summary()Summary{now:=time.Now().UTC();s:=Summary{Workers:m.workers};var measuredBytes,measuredDurationMS int64;m.mu.RLock();for _,raw:=range m.jobs{j:=decorateJob(raw,now);s.Total++;s.BytesRead+=j.BytesRead;s.BytesWritten+=j.BytesWritten;switch j.State{case Queued:s.Queued++;case Running:s.Running++;s.ActiveIOMiBPerSec+=j.IOMiBPerSec;case Succeeded:s.Succeeded++;if j.Operation!="rename-zip"&&j.DurationMS>0&&j.BytesRead+j.BytesWritten>0{measuredBytes+=j.BytesRead+j.BytesWritten;measuredDurationMS+=j.DurationMS;s.MeasuredJobs++};case Failed:s.Failed++;case Cancelled:s.Cancelled++}};m.mu.RUnlock();if measuredDurationMS>0{s.CompletedIOMiBPerSec=bytesPerSecondMiB(measuredBytes,measuredDurationMS)};return s}
func (m *Manager) Cancel(id string)bool{m.mu.Lock();defer m.mu.Unlock();j,ok:=m.jobs[id];if !ok||j.State==Succeeded||j.State==Failed||j.State==Cancelled{return false};if cancel:=m.cancel[id];cancel!=nil{cancel()};if j.State==Queued{j.State=Cancelled;j.FinishedAt=time.Now().UTC()};return true}
func (m *Manager) Workers()int{return m.workers}
func (m *Manager) Close(){m.stop();m.wg.Wait()}
func (m *Manager) worker(workerID int){defer m.wg.Done();for{select{case<-m.rootCtx.Done():return;case it:=<-m.queue:m.run(it,workerID)}}}
func (m *Manager) run(it item,workerID int){m.mu.Lock();j:=m.jobs[it.id];if j==nil||j.State==Cancelled{m.mu.Unlock();return};ctx,cancel:=context.WithCancel(m.rootCtx);m.cancel[it.id]=cancel;j.State=Running;j.Worker=workerID;j.StartedAt=time.Now().UTC();m.mu.Unlock();err:=m.processor(ctx,it.id,it.task,func(u Update){m.mu.Lock();if j:=m.jobs[it.id];j!=nil&&j.State==Running{if u.Stage!=""{j.Stage=u.Stage};if u.Progress>=0&&u.Progress<=1{j.Progress=u.Progress};if u.BytesRead>=0{j.BytesRead=u.BytesRead};if u.BytesWritten>=0{j.BytesWritten=u.BytesWritten};if u.Operation!=""{j.Operation=u.Operation};if u.Entries>0{j.Entries=u.Entries}};m.mu.Unlock()});cancel();m.mu.Lock();defer m.mu.Unlock();delete(m.cancel,it.id);j=m.jobs[it.id];if j==nil{return};j.FinishedAt=time.Now().UTC();if errors.Is(err,context.Canceled){j.State=Cancelled;j.Error="cancelled"}else if err!=nil{j.State=Failed;j.Error=err.Error()}else{j.State=Succeeded;j.Progress=1;j.Stage="done"}}
func decorateJob(j *Job,now time.Time)Job{cp:=*j;if !cp.StartedAt.IsZero(){cp.QueueWaitMS=cp.StartedAt.Sub(cp.CreatedAt).Milliseconds();end:=now;if !cp.FinishedAt.IsZero(){end=cp.FinishedAt};if end.Before(cp.StartedAt){end=cp.StartedAt};cp.DurationMS=end.Sub(cp.StartedAt).Milliseconds();if cp.Operation!="rename-zip"&&cp.DurationMS>0{cp.ReadMiBPerSec=bytesPerSecondMiB(cp.BytesRead,cp.DurationMS);cp.WriteMiBPerSec=bytesPerSecondMiB(cp.BytesWritten,cp.DurationMS);cp.IOMiBPerSec=bytesPerSecondMiB(cp.BytesRead+cp.BytesWritten,cp.DurationMS)}};return cp}
func bytesPerSecondMiB(bytes,durationMS int64)float64{if bytes<=0||durationMS<=0{return 0};return(float64(bytes)/(1024*1024))/(float64(durationMS)/1000)}
