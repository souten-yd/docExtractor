package archive

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoverPartials(t *testing.T) {
	root := t.TempDir()
	series := filepath.Join(root, "Series")
	if err := os.MkdirAll(series, 0o750); err != nil {
		t.Fatal(err)
	}

	valid := filepath.Join(series, "valid.zip.partial")
	writeRecoveryTestZIP(t, valid)
	invalid := filepath.Join(series, "invalid.zip.partial")
	if err := os.WriteFile(invalid, []byte("broken"), 0o640); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(series, "stale.zip.partial")
	if err := os.WriteFile(stale, []byte("broken"), 0o640); err != nil {
		t.Fatal(err)
	}
	writeRecoveryTestZIP(t, filepath.Join(series, "stale.zip"))

	summary := RecoverPartials(context.Background(), root, VerifyCentral)
	if summary.Found != 3 || summary.Promoted != 1 || summary.RemovedStale != 1 || summary.InvalidKept != 1 || summary.Errors != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if _, err := os.Stat(filepath.Join(series, "valid.zip")); err != nil {
		t.Fatalf("valid partial was not promoted: %v", err)
	}
	if _, err := os.Stat(invalid); err != nil {
		t.Fatalf("invalid partial should be kept: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale partial should be removed, err=%v", err)
	}
}

func writeRecoveryTestZIP(t *testing.T, filename string) {
	t.Helper()
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("001.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
