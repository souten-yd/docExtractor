package organizer

import (
	"os"
	"path/filepath"
	"testing"
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
