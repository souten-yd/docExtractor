package archive

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func readZIPEntry(t *testing.T, filename, name string) string {
	t.Helper()
	zr, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	t.Fatalf("entry %q not found in %s", name, filename)
	return ""
}

func TestReconcileOverlappingVolumesKeepsNewerAndQuarantinesOlder(t *testing.T) {
	dir := t.TempDir()
	outRoot := filepath.Join(dir, "out")
	series := "作品名"
	oldSource := filepath.Join(dir, "old.zip")
	newSource := filepath.Join(dir, "new.zip")
	makeTestZIP(t, oldSource, map[string]string{"作品名 第01巻/001.jpg": "one", "作品名 第02巻/001.jpg": "old-two"})
	makeTestZIP(t, newSource, map[string]string{"作品名 第02巻/001.jpg": "new-two", "作品名 第03巻/001.jpg": "three"})
	oldTime := time.Unix(1700000000, 0)
	newTime := oldTime.Add(time.Hour)
	if err := os.Chtimes(oldSource, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newSource, newTime, newTime); err != nil {
		t.Fatal(err)
	}
	targets := map[string]string{
		"作品名 第01巻": filepath.Join(outRoot, series, "作品名 第01巻.zip"),
		"作品名 第02巻": filepath.Join(outRoot, series, "作品名 第02巻.zip"),
		"作品名 第03巻": filepath.Join(outRoot, series, "作品名 第03巻.zip"),
	}
	p := New(Config{})
	for _, source := range []string{oldSource, newSource} {
		_, err := p.Process(context.Background(), Task{Source: source, Destination: filepath.Join(outRoot, series, filepath.Base(source)), DeleteSource: true, OutputTargets: targets, ReconcileOutputs: true}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := readZIPEntry(t, targets["作品名 第01巻"], "001.jpg"); got != "one" {
		t.Fatalf("volume 1=%q", got)
	}
	if got := readZIPEntry(t, targets["作品名 第02巻"], "001.jpg"); got != "new-two" {
		t.Fatalf("volume 2=%q", got)
	}
	if got := readZIPEntry(t, targets["作品名 第03巻"], "001.jpg"); got != "three" {
		t.Fatalf("volume 3=%q", got)
	}
	q := filepath.Join(outRoot, ".docExtractor-duplicates", series, "作品名 第02巻.zip")
	if got := readZIPEntry(t, q, "001.jpg"); got != "old-two" {
		t.Fatalf("quarantined volume 2=%q", got)
	}
}

func TestReconcileExactDuplicateKeepsActiveAndQuarantinesCandidate(t *testing.T) {
	dir := t.TempDir()
	outRoot := filepath.Join(dir, "out")
	series := "作品名"
	target := filepath.Join(outRoot, series, "作品名 第01巻.zip")
	a := filepath.Join(dir, "a.zip")
	makeTestZIP(t, a, map[string]string{"作品名 第01巻/001.jpg": "same"})
	raw, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	b := filepath.Join(dir, "b.zip")
	if err := os.WriteFile(b, raw, 0o640); err != nil {
		t.Fatal(err)
	}
	base := time.Unix(1700000000, 0)
	_ = os.Chtimes(a, base, base)
	_ = os.Chtimes(b, base.Add(time.Hour), base.Add(time.Hour))
	p := New(Config{})
	mapping := map[string]string{"作品名 第01巻": target}
	for _, source := range []string{a, b} {
		if _, err := p.Process(context.Background(), Task{Source: source, Destination: filepath.Join(outRoot, series, filepath.Base(source)), DeleteSource: true, OutputTargets: mapping, ReconcileOutputs: true}, nil); err != nil {
			t.Fatal(err)
		}
	}
	active, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(active) != string(raw) {
		t.Fatal("active exact duplicate changed")
	}
	if _, err := os.Stat(filepath.Join(outRoot, ".docExtractor-duplicates", series, "作品名 第01巻.zip")); err != nil {
		t.Fatal(err)
	}
}
