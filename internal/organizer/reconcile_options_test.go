package organizer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuarantineIsExcludedByDefaultAndIncludedOnRequest(t *testing.T) {
	root := t.TempDir()
	q := filepath.Join(root, ".docExtractor-duplicates", "ブラックラグーン", "ブラックラグーン 第02巻.zip")
	writeTestComicZIP(t, q, "ブラックラグーン 第02巻")
	o, err := New(Config{Root: root, OutputRoot: root, ConfidenceThreshold: .72})
	if err != nil { t.Fatal(err) }

	def, err := o.ReconcileScanMultiProgress([]string{root}, root, nil)
	if err != nil { t.Fatal(err) }
	if def.Summary.Files != 0 { t.Fatalf("default scan included quarantine: %d", def.Summary.Files) }

	report, err := o.ReconcileScanMultiProgressWithOptions([]string{root}, root, ReconcileScanOptions{IncludeQuarantine: true}, nil)
	if err != nil { t.Fatal(err) }
	if report.Summary.Files != 1 { t.Fatalf("files=%d want 1", report.Summary.Files) }
	if report.Items[0].Action != "move" { t.Fatalf("action=%q item=%#v", report.Items[0].Action, report.Items[0]) }
	want := filepath.Join(root, "ブラックラグーン", filepath.Base(q))
	if report.Items[0].Destination != want { t.Fatalf("destination=%q want %q", report.Items[0].Destination, want) }

	res, err := o.ReconcileExecuteReportProgress(report, nil, nil)
	if err != nil { t.Fatal(err) }
	if res.Moved != 1 { t.Fatalf("moved=%d want 1", res.Moved) }
	if _, err := os.Stat(want); err != nil { t.Fatalf("restored file missing: %v", err) }
}

func TestBadLegacyParentDoesNotOverrideGoodArchiveEvidence(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "第13巻", "正しい作品 第13巻.zip")
	writeTestComicZIP(t, p, "正しい作品 第13巻")
	o, err := New(Config{Root: root, OutputRoot: root, ConfidenceThreshold: .72})
	if err != nil { t.Fatal(err) }
	report, err := o.ReconcileScanMultiProgress([]string{root}, root, nil)
	if err != nil { t.Fatal(err) }
	if len(report.Items) != 1 { t.Fatalf("items=%d", len(report.Items)) }
	if report.Items[0].Series != "正しい作品" { t.Fatalf("series=%q", report.Items[0].Series) }
}

func TestMojibakeInternalFolderDoesNotOverrideOuterName(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "old", "ブラックラグーン 第01巻.zip")
	writeTestComicZIP(t, p, "���broken-title")
	o, err := New(Config{Root: root, OutputRoot: root, ConfidenceThreshold: .72})
	if err != nil { t.Fatal(err) }
	report, err := o.ReconcileScanMultiProgress([]string{root}, root, nil)
	if err != nil { t.Fatal(err) }
	if len(report.Items) != 1 { t.Fatalf("items=%d", len(report.Items)) }
	if report.Items[0].Series != "ブラックラグーン" { t.Fatalf("series=%q", report.Items[0].Series) }
}
