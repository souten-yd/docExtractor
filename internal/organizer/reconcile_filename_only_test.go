package organizer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReconcileUsesArchiveFilenameOnlyEvenWhenZIPIsInvalid(t *testing.T) {
	root := t.TempDir()
	badBucket := filepath.Join(root, "単ページ")
	if err := os.MkdirAll(badBucket, 0o750); err != nil {
		t.Fatal(err)
	}
	name := "[山口ミコト×北河トウタ] DEAD Tube -デッドチューブ- 第05巻.zip"
	src := filepath.Join(badBucket, name)
	// Deliberately not a ZIP. Reconcile is filesystem metadata processing and
	// must not open archive members just to determine title/volume.
	if err := os.WriteFile(src, []byte("already-processed-library-placeholder"), 0o640); err != nil {
		t.Fatal(err)
	}
	org, err := New(Config{Root: root, ConfidenceThreshold: 0.72})
	if err != nil {
		t.Fatal(err)
	}
	report, err := org.ReconcileScan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("items=%d report=%+v", len(report.Items), report.Summary)
	}
	it := report.Items[0]
	if it.Series != "DEAD Tube -デッドチューブ-" {
		t.Fatalf("series=%q; bad parent bucket must not win", it.Series)
	}
	if !it.HasVolume || it.Volume != 5 {
		t.Fatalf("volume=%d has=%v", it.Volume, it.HasVolume)
	}
	if it.Action != "move" {
		t.Fatalf("action=%q reason=%q", it.Action, it.Reason)
	}
	want := filepath.Join(root, "DEAD Tube -デッドチューブ-", name)
	if filepath.Clean(it.Destination) != filepath.Clean(want) {
		t.Fatalf("destination=%q want=%q", it.Destination, want)
	}
}

func TestReconcileLowConfidenceFilenameStillBeatsUnrelatedLegacyBucket(t *testing.T) {
	root := t.TempDir()
	badBucket := filepath.Join(root, "net")
	if err := os.MkdirAll(badBucket, 0o750); err != nil {
		t.Fatal(err)
	}
	name := "[永川成基×白狼] 白い魔女.zip"
	if err := os.WriteFile(filepath.Join(badBucket, name), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	org, err := New(Config{Root: root, ConfidenceThreshold: 0.72})
	if err != nil {
		t.Fatal(err)
	}
	report, err := org.ReconcileScan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 || report.Items[0].Series != "白い魔女" {
		t.Fatalf("unexpected report: %+v", report.Items)
	}
}
