package jobs

import (
	"context"
	"testing"
	"time"
)

func TestManagerCompletesAndReportsPerformance(t *testing.T) {
	m, err := New(2, 8, func(ctx context.Context, id string, task Task, update func(Update)) error {
		update(Update{Stage: "work", Progress: 0.5, BytesRead: 4 * 1024 * 1024, BytesWritten: 2 * 1024 * 1024})
		time.Sleep(15 * time.Millisecond)
		update(Update{Stage: "done", Progress: 1, BytesRead: 8 * 1024 * 1024, BytesWritten: 4 * 1024 * 1024, Operation: "rar-to-zip", Entries: 12})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	j, err := m.Submit(Task{Source: "a", Destination: "b"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, _ := m.Get(j.ID)
		if got.State == Succeeded {
			if got.DurationMS <= 0 {
				t.Fatalf("expected positive duration, got %d", got.DurationMS)
			}
			if got.Worker < 1 || got.Worker > 2 {
				t.Fatalf("unexpected worker %d", got.Worker)
			}
			if got.IOMiBPerSec <= 0 || got.ReadMiBPerSec <= 0 || got.WriteMiBPerSec <= 0 {
				t.Fatalf("expected throughput metrics, got %+v", got)
			}
			if got.Operation != "rar-to-zip" || got.Entries != 12 {
				t.Fatalf("result metadata missing: %+v", got)
			}
			s := m.Summary()
			if s.Succeeded != 1 || s.MeasuredJobs != 1 || s.CompletedIOMiBPerSec <= 0 {
				t.Fatalf("unexpected summary: %+v", s)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not complete")
}

func TestRenameOnlyExcludedFromThroughputSummary(t *testing.T) {
	m, err := New(1, 2, func(ctx context.Context, id string, task Task, update func(Update)) error {
		time.Sleep(2 * time.Millisecond)
		update(Update{BytesRead: 2 * 1024 * 1024 * 1024, Operation: "rename-zip", Entries: 100})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	j, err := m.Submit(Task{Source: "a", Destination: "b"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, _ := m.Get(j.ID)
		if got.State == Succeeded {
			if got.IOMiBPerSec != 0 {
				t.Fatalf("rename-only job should not report throughput: %+v", got)
			}
			s := m.Summary()
			if s.MeasuredJobs != 0 || s.CompletedIOMiBPerSec != 0 {
				t.Fatalf("rename-only job should be excluded from aggregate throughput: %+v", s)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not complete")
}
