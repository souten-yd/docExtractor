package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/souten-yd/docExtractor/internal/organizer"
)

const reconcileSnapshotPageSize = 200

type reconcileSnapshotMeta struct {
	Report       organizer.ReconcileReport `json:"report"`
	ItemsTotal   int                       `json:"items_total"`
	ChoicesTotal int                       `json:"choices_total"`
}

func reconcileSnapshotDir(outputRoot, mode string) string {
	return filepath.Join(outputRoot, ".docExtractor-state", "reconcile-"+validAsyncMode(mode))
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil { return err }
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil { return err }
	ok := false
	defer func(){ _ = f.Close(); if !ok { _ = os.Remove(tmp) } }()
	enc := json.NewEncoder(f)
	if err := enc.Encode(value); err != nil { return err }
	if err := f.Sync(); err != nil { return err }
	if err := f.Close(); err != nil { return err }
	if err := os.Rename(tmp, path); err != nil { return err }
	ok = true
	return nil
}

func saveReconcileSnapshot(outputRoot, mode string, report organizer.ReconcileReport) (string, reconcileSnapshotMeta, error) {
	dir := reconcileSnapshotDir(outputRoot, mode)
	if err := os.RemoveAll(dir); err != nil { return "", reconcileSnapshotMeta{}, err }
	if err := os.MkdirAll(dir, 0o750); err != nil { return "", reconcileSnapshotMeta{}, err }
	for start, page := 0, 0; start < len(report.Items); start, page = start+reconcileSnapshotPageSize, page+1 {
		end := start + reconcileSnapshotPageSize; if end > len(report.Items) { end = len(report.Items) }
		if err := writeJSONAtomic(filepath.Join(dir, fmt.Sprintf("items-%06d.json", page)), report.Items[start:end]); err != nil { return "", reconcileSnapshotMeta{}, err }
	}
	for start, page := 0, 0; start < len(report.Choices); start, page = start+reconcileSnapshotPageSize, page+1 {
		end := start + reconcileSnapshotPageSize; if end > len(report.Choices) { end = len(report.Choices) }
		if err := writeJSONAtomic(filepath.Join(dir, fmt.Sprintf("choices-%06d.json", page)), report.Choices[start:end]); err != nil { return "", reconcileSnapshotMeta{}, err }
	}
	metaReport := report
	metaReport.Items = nil
	metaReport.Choices = nil
	meta := reconcileSnapshotMeta{Report: metaReport, ItemsTotal: len(report.Items), ChoicesTotal: len(report.Choices)}
	if err := writeJSONAtomic(filepath.Join(dir, "meta.json"), meta); err != nil { return "", reconcileSnapshotMeta{}, err }
	return dir, meta, nil
}

func loadSnapshotMeta(dir string) (reconcileSnapshotMeta, error) {
	var meta reconcileSnapshotMeta
	f, err := os.Open(filepath.Join(dir, "meta.json")); if err != nil { return meta, err }; defer f.Close()
	err = json.NewDecoder(f).Decode(&meta)
	return meta, err
}

func loadSnapshotPage[T any](dir, prefix string, offset, limit, total int) ([]T, error) {
	if offset < 0 { offset = 0 }; if limit < 1 { limit = 100 }; if limit > 500 { limit = 500 }; if offset > total { offset = total }
	end := offset + limit; if end > total { end = total }; if end <= offset { return []T{}, nil }
	out := make([]T, 0, end-offset)
	firstPage, lastPage := offset/reconcileSnapshotPageSize, (end-1)/reconcileSnapshotPageSize
	for page := firstPage; page <= lastPage; page++ {
		var rows []T
		f, err := os.Open(filepath.Join(dir, fmt.Sprintf("%s-%06d.json", prefix, page))); if err != nil { return nil, err }
		err = json.NewDecoder(f).Decode(&rows); _ = f.Close(); if err != nil { return nil, err }
		pageStart := page * reconcileSnapshotPageSize
		lo := 0; if offset > pageStart { lo = offset-pageStart }
		hi := len(rows); if end < pageStart+hi { hi = end-pageStart }
		if lo < hi { out = append(out, rows[lo:hi]...) }
	}
	return out, nil
}

func loadReconcileSnapshot(dir string) (organizer.ReconcileReport, error) {
	meta, err := loadSnapshotMeta(dir); if err != nil { return organizer.ReconcileReport{}, err }
	report := meta.Report
	report.Items = make([]organizer.ReconcileItem, 0, meta.ItemsTotal)
	for offset := 0; offset < meta.ItemsTotal; offset += reconcileSnapshotPageSize {
		rows, err := loadSnapshotPage[organizer.ReconcileItem](dir, "items", offset, reconcileSnapshotPageSize, meta.ItemsTotal); if err != nil { return organizer.ReconcileReport{}, err }
		report.Items = append(report.Items, rows...)
	}
	report.Choices = make([]organizer.ReconcileChoice, 0, meta.ChoicesTotal)
	for offset := 0; offset < meta.ChoicesTotal; offset += reconcileSnapshotPageSize {
		rows, err := loadSnapshotPage[organizer.ReconcileChoice](dir, "choices", offset, reconcileSnapshotPageSize, meta.ChoicesTotal); if err != nil { return organizer.ReconcileReport{}, err }
		report.Choices = append(report.Choices, rows...)
	}
	return report, nil
}

func releaseProcessMemory() {
	runtime.GC()
	debug.FreeOSMemory()
}
