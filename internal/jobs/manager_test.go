package jobs

import (
	"context"
	"testing"
	"time"
)

func TestManagerCompletes(t *testing.T) {
	m, err := New(2, 8, func(ctx context.Context, id string, task Task, update func(Update)) error {
		update(Update{Stage: "work", Progress: 0.5})
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
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not complete")
}
