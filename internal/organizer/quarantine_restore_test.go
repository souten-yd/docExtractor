package organizer

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestComicZIPData(t *testing.T, path, folder, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { t.Fatal(err) }
	f, err := os.Create(path); if err != nil { t.Fatal(err) }
	zw := zip.NewWriter(f)
	w, err := zw.Create(folder + "/001.jpg"); if err != nil { t.Fatal(err) }
	if _, err := w.Write([]byte(data)); err != nil { t.Fatal(err) }
	if err := zw.Close(); err != nil { t.Fatal(err) }
	if err := f.Close(); err != nil { t.Fatal(err) }
}

func TestNewerQuarantinedVariantCanReplaceOlderActiveCopy(t *testing.T) {
	root := t.TempDir()
	name := "ブラックラグーン 第03巻.zip"
	active := filepath.Join(root, "ブラックラグーン", name)
	quarantined := filepath.Join(root, ".docExtractor-duplicates", "ブラックラグーン", name)
	writeTestComicZIPData(t, active, "ブラックラグーン 第03巻", "old")
	writeTestComicZIPData(t, quarantined, "ブラックラグーン 第03巻", "new")
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(active, oldTime, oldTime); err != nil { t.Fatal(err) }
	if err := os.Chtimes(quarantined, newTime, newTime); err != nil { t.Fatal(err) }

	o, err := New(Config{Root: root, OutputRoot: root, ConfidenceThreshold: .72})
	if err != nil { t.Fatal(err) }
	report, err := o.ReconcileScanMultiProgressWithOptions([]string{root}, root, ReconcileScanOptions{IncludeQuarantine: true}, nil)
	if err != nil { t.Fatal(err) }
	var qItem *ReconcileItem
	for i := range report.Items {
		if filepath.Clean(report.Items[i].Source) == filepath.Clean(quarantined) { qItem = &report.Items[i]; break }
	}
	if qItem == nil { t.Fatal("quarantined candidate missing") }
	if qItem.Action != "move" { t.Fatalf("quarantined winner action=%q reason=%q", qItem.Action, qItem.Reason) }

	res, err := o.ReconcileExecuteReportProgress(report, nil, nil)
	if err != nil { t.Fatal(err) }
	if res.Moved < 1 || res.Quarantined < 1 { t.Fatalf("result=%+v", res) }
	f, err := zip.OpenReader(active); if err != nil { t.Fatal(err) }
	defer f.Close()
	r, err := f.File[0].Open(); if err != nil { t.Fatal(err) }
	buf := make([]byte, 3)
	if _, err := r.Read(buf); err != nil { t.Fatal(err) }
	_ = r.Close()
	if string(buf) != "new" { t.Fatalf("active copy is %q, want new", string(buf)) }
}
