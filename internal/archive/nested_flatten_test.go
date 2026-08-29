package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func zipBytes(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entries[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func zipNames(t *testing.T, filename string) []string {
	t.Helper()
	zr, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() {
			out = append(out, f.Name)
		}
	}
	sort.Strings(out)
	return out
}

func TestZIPNestedArchivesAreFlattenedRecursively(t *testing.T) {
	dir := t.TempDir()
	deep := zipBytes(t, map[string][]byte{
		"pages/001.jpg": []byte("image-one"),
		"pages/002.jpg": []byte("image-two"),
	})
	inner := zipBytes(t, map[string][]byte{
		"日本語作品/deep.zip": deep,
	})
	outer := zipBytes(t, map[string][]byte{
		"inner.zip": inner,
	})
	src := filepath.Join(dir, "source.zip")
	dst := filepath.Join(dir, "Series", "source.zip")
	if err := os.WriteFile(src, outer, 0o640); err != nil {
		t.Fatal(err)
	}

	p := New(Config{})
	result, err := p.Process(context.Background(), Task{Source: src, Destination: dst, DeleteSource: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Operation, "flatten-nested-zip") {
		t.Fatalf("unexpected operation: %+v", result)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be removed only after verified output, stat err=%v", err)
	}
	if _, err := VerifyZIPNoNestedArchives(context.Background(), dst, VerifyFull); err != nil {
		t.Fatal(err)
	}
	names := zipNames(t, dst)
	if len(names) != 2 {
		t.Fatalf("unexpected output names: %#v", names)
	}
	for _, name := range names {
		if isArchiveLikeName(name) {
			t.Fatalf("nested archive remained: %q", name)
		}
		if !strings.Contains(name, "日本語作品") {
			t.Fatalf("meaningful nested Japanese folder should be preserved: %q", name)
		}
	}
}

func TestMultipleNestedZIPsUseArchiveStemFolders(t *testing.T) {
	dir := t.TempDir()
	vol1 := zipBytes(t, map[string][]byte{"001.jpg": []byte("one")})
	vol2 := zipBytes(t, map[string][]byte{"001.jpg": []byte("two")})
	outer := zipBytes(t, map[string][]byte{
		"第01巻.zip": vol1,
		"第02巻.zip": vol2,
	})
	src := filepath.Join(dir, "collection.zip")
	dst := filepath.Join(dir, "out", "collection.zip")
	if err := os.WriteFile(src, outer, 0o640); err != nil {
		t.Fatal(err)
	}
	p := New(Config{})
	if _, err := p.Process(context.Background(), Task{Source: src, Destination: dst, DeleteSource: true}, nil); err != nil {
		t.Fatal(err)
	}
	names := zipNames(t, dst)
	want := []string{"第01巻/001.jpg", "第02巻/001.jpg"}
	if len(names) != len(want) {
		t.Fatalf("got=%#v want=%#v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got=%#v want=%#v", names, want)
		}
	}
}

func TestUnsupportedNestedArchiveFailsWithoutReplacingSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.zip")
	dst := filepath.Join(dir, "out", "source.zip")
	outer := zipBytes(t, map[string][]byte{"inside.7z": []byte("not-a-real-7z")})
	if err := os.WriteFile(src, outer, 0o640); err != nil {
		t.Fatal(err)
	}
	p := New(Config{})
	if _, err := p.Process(context.Background(), Task{Source: src, Destination: dst, DeleteSource: true}, nil); err == nil {
		t.Fatal("expected unsupported nested archive error")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source must remain after failed normalization: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("destination must not be published on failure")
	}
	if _, err := os.Stat(dst + ".partial"); !os.IsNotExist(err) {
		t.Fatalf("partial output should be cleaned")
	}
}
