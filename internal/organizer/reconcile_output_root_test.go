package organizer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReconcileScansExistingOutputLibrary(t *testing.T) {
	output := t.TempDir()
	input := filepath.Join(output, "incoming")
	if err := os.MkdirAll(input, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(input, "作品", "作品 第01巻.zip")
	existing := filepath.Join(output, "作品", "作品 第02巻.zip")
	writeTestComicZIP(t, source, "volume 1")
	writeTestComicZIP(t, existing, "volume 2")
	o, err := New(Config{Root: input, OutputRoot: output, ConfidenceThreshold: .72})
	if err != nil {
		t.Fatal(err)
	}
	report, err := o.ReconcileScanMulti([]string{input}, output)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Roots) != 2 {
		t.Fatalf("roots=%v; existing output library was not added", report.Roots)
	}
	if report.Summary.Files != 2 || report.Summary.Move != 1 || report.Summary.Keep != 1 {
		t.Fatalf("output library was not compared exactly once: summary=%+v items=%+v", report.Summary, report.Items)
	}
}

func TestReconcileOutputLibraryResolvesBilingualAliasAndKeepsDerivative(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()
	folder := "BLACK LAGOON ブラック・ラグーン"
	incoming := filepath.Join(input, folder, "[広江礼威] BLACK LAGOON 第01巻.zip")
	existing := filepath.Join(output, folder, "[広江礼威] BLACK LAGOON ブラック・ラグーン 第01巻.zip")
	existingSecond := filepath.Join(output, folder, "[広江礼威] BLACK LAGOON ブラック・ラグーン 第02巻.zip")
	derivative := filepath.Join(output, folder, "[広江礼威] BLACK LAGOON エダ イニシャルステージ 第01巻.zip")
	writeTestComicZIP(t, incoming, "incoming-main")
	writeTestComicZIP(t, existing, "existing-main")
	writeTestComicZIP(t, existingSecond, "existing-second")
	writeTestComicZIP(t, derivative, "derivative")
	now := time.Now()
	if err := os.Chtimes(existing, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(incoming, now, now); err != nil {
		t.Fatal(err)
	}

	o, err := New(Config{Root: input, OutputRoot: output, ConfidenceThreshold: .72})
	if err != nil {
		t.Fatal(err)
	}
	report, err := o.ReconcileScanMulti([]string{input}, output)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Files != 4 || report.Summary.Superseded != 1 || report.Summary.Review != 0 {
		t.Fatalf("source/output alias comparison failed: summary=%+v items=%+v", report.Summary, report.Items)
	}
	for _, item := range report.Items {
		if filepath.Clean(item.Source) == filepath.Clean(derivative) && item.Action != "keep" {
			t.Fatalf("output derivative was changed: %#v", item)
		}
	}
}
