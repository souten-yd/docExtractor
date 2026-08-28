package archive

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyAndRenameZIPWithoutRewrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.zip")
	dst := filepath.Join(dir, "Series", "source.zip")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("001.jpg")
	_, _ = w.Write([]byte("fake-jpeg"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	p := New(Config{})
	result, err := p.Process(context.Background(), Task{Source: src, Destination: dst, DeleteSource: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation != "rename-zip" || result.BytesWritten != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still exists")
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatal(err)
	}
}

func TestCommonRoot(t *testing.T) {
	if got := commonRoot([]string{"book/001.jpg", "book/002.jpg"}); got != "book" {
		t.Fatalf("got %q", got)
	}
	if got := commonRoot([]string{"001.jpg", "002.jpg"}); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestSafeArchiveName(t *testing.T) {
	if _, err := safeArchiveName("../../etc/passwd", ""); err == nil {
		t.Fatal("expected traversal rejection")
	}
	got, err := safeArchiveName("book/001.jpg", "book")
	if err != nil || got != "001.jpg" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}
