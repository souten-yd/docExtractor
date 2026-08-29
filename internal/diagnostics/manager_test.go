package diagnostics

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestPrivacyRedactsSecretsAndPaths(t *testing.T) {
	m, err := New(Config{RootDir: t.TempDir(), PrivacyMode: true})
	if err != nil {
		t.Fatal(err)
	}
	l, err := m.Job("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Write(Event{Message: "test", Fields: map[string]any{
		"source_path": "/share/Download/private/book.rar",
		"api_token":   "secret-value",
	}}); err != nil {
		t.Fatal(err)
	}

	events, err := m.Tail("job-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := events[0].Fields["source_path"]; got != "book.rar" {
		t.Fatalf("path not redacted: %v", got)
	}
	if got := events[0].Fields["api_token"]; got != "[REDACTED]" {
		t.Fatalf("secret not redacted: %v", got)
	}
}

func TestBundleContainsLogAndMetadata(t *testing.T) {
	m, err := New(Config{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := m.Job("job-2")
	_ = l.Write(Event{Message: "hello"})

	var b bytes.Buffer
	if err := m.WriteBundle(&b, "job-2", map[string]any{"version": "test"}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	foundLog, foundDiag := false, false
	for _, f := range zr.File {
		switch f.Name {
		case "logs/job-2.jsonl":
			foundLog = true
		case "diagnostics.json":
			foundDiag = true
			rc, _ := f.Open()
			data, _ := io.ReadAll(rc)
			_ = rc.Close()
			var v map[string]any
			if json.Unmarshal(data, &v) != nil {
				t.Fatal("invalid diagnostics json")
			}
		}
	}
	if !foundLog || !foundDiag {
		t.Fatalf("bundle missing files: log=%v diagnostics=%v", foundLog, foundDiag)
	}
}

func TestGlobalBundleContainsTenMostRecentLogs(t *testing.T) {
	m, err := New(Config{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		id := "job-" + string(rune('a'+i))
		l, err := m.Job(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := l.Write(Event{Message: id}); err != nil {
			t.Fatal(err)
		}
	}

	var b bytes.Buffer
	if err := m.WriteBundle(&b, "", map[string]any{"version": "test"}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	logs := 0
	for _, f := range zr.File {
		if len(f.Name) > len("logs/") && f.Name[:len("logs/")] == "logs/" {
			logs++
		}
	}
	if logs != 10 {
		t.Fatalf("expected 10 recent logs, got %d", logs)
	}
}
