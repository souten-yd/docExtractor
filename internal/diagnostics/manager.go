package diagnostics

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Config struct {
	RootDir       string
	RetentionDays int
	PrivacyMode   bool
}

type Event struct {
	Time         time.Time      `json:"time"`
	Level        string         `json:"level"`
	Component    string         `json:"component,omitempty"`
	JobID        string         `json:"job_id,omitempty"`
	Stage        string         `json:"stage,omitempty"`
	Message      string         `json:"message"`
	Error        string         `json:"error,omitempty"`
	DurationMS   int64          `json:"duration_ms,omitempty"`
	BytesRead    int64          `json:"bytes_read,omitempty"`
	BytesWritten int64          `json:"bytes_written,omitempty"`
	Fields       map[string]any `json:"fields,omitempty"`
}

type Manager struct {
	cfg Config
	mu  sync.Mutex
}

type JobLogger struct {
	manager *Manager
	jobID   string
}

type JobLogInfo struct {
	JobID     string    `json:"job_id"`
	SizeBytes int64     `json:"size_bytes"`
	UpdatedAt time.Time `json:"updated_at"`
}

func New(cfg Config) (*Manager, error) {
	if cfg.RootDir == "" {
		return nil, errors.New("diagnostics root directory is required")
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = 14
	}
	if err := os.MkdirAll(filepath.Join(cfg.RootDir, "jobs"), 0o750); err != nil {
		return nil, err
	}
	return &Manager{cfg: cfg}, nil
}

func (m *Manager) Job(jobID string) (*JobLogger, error) {
	if !safeID(jobID) {
		return nil, fmt.Errorf("invalid job id")
	}
	return &JobLogger{manager: m, jobID: jobID}, nil
}

func (l *JobLogger) Write(e Event) error {
	e.JobID = l.jobID
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if e.Level == "" {
		e.Level = "info"
	}
	if l.manager.cfg.PrivacyMode {
		e.Fields = redactFields(e.Fields)
	}

	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	l.manager.mu.Lock()
	defer l.manager.mu.Unlock()

	f, err := os.OpenFile(l.manager.jobPath(l.jobID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(payload)
	return err
}

func (m *Manager) ListJobs() ([]JobLogInfo, error) {
	entries, err := os.ReadDir(filepath.Join(m.cfg.RootDir, "jobs"))
	if err != nil {
		return nil, err
	}
	out := make([]JobLogInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, JobLogInfo{
			JobID:     strings.TrimSuffix(entry.Name(), ".jsonl"),
			SizeBytes: info.Size(),
			UpdatedAt: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (m *Manager) OpenJobLog(jobID string) (*os.File, error) {
	if !safeID(jobID) {
		return nil, fmt.Errorf("invalid job id")
	}
	return os.Open(m.jobPath(jobID))
}

func (m *Manager) Tail(jobID string, maxLines int) ([]Event, error) {
	if maxLines <= 0 || maxLines > 5000 {
		maxLines = 500
	}
	f, err := m.OpenJobLog(jobID)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	lines := make([][]byte, 0, maxLines)
	s := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	s.Buffer(buf, 4*1024*1024)
	for s.Scan() {
		cp := append([]byte(nil), s.Bytes()...)
		if len(lines) == maxLines {
			copy(lines, lines[1:])
			lines[maxLines-1] = cp
		} else {
			lines = append(lines, cp)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(lines))
	for _, line := range lines {
		var e Event
		if json.Unmarshal(line, &e) == nil {
			out = append(out, e)
		}
	}
	return out, nil
}

// WriteBundle emits a support ZIP without copying archive payloads. It only contains logs and small diagnostic metadata.
func (m *Manager) WriteBundle(w io.Writer, jobID string, snapshot any) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	if jobID != "" {
		f, err := m.OpenJobLog(jobID)
		if err != nil {
			return err
		}
		dst, err := zw.Create(filepath.ToSlash(filepath.Join("logs", jobID+".jsonl")))
		if err != nil {
			f.Close()
			return err
		}
		if _, err := io.Copy(dst, f); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}

	meta, err := zw.Create("diagnostics.json")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(meta)
	enc.SetIndent("", "  ")
	return enc.Encode(snapshot)
}

func (m *Manager) Cleanup(now time.Time) error {
	cutoff := now.AddDate(0, 0, -m.cfg.RetentionDays)
	entries, err := os.ReadDir(filepath.Join(m.cfg.RootDir, "jobs"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(m.cfg.RootDir, "jobs", entry.Name()))
		}
	}
	return nil
}

func (m *Manager) jobPath(jobID string) string {
	return filepath.Join(m.cfg.RootDir, "jobs", jobID+".jsonl")
}

func safeID(v string) bool {
	if v == "" || len(v) > 128 {
		return false
	}
	for _, r := range v {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func redactFields(in map[string]any) map[string]any {
	if len(in) == 0 {
		return in
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		lk := strings.ToLower(k)
		switch {
		case strings.Contains(lk, "password"), strings.Contains(lk, "secret"), strings.Contains(lk, "token"):
			out[k] = "[REDACTED]"
		case strings.Contains(lk, "path"):
			if s, ok := v.(string); ok {
				out[k] = filepath.Base(s)
			} else {
				out[k] = "[REDACTED_PATH]"
			}
		default:
			out[k] = v
		}
	}
	return out
}
