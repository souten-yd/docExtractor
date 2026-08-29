package organizer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ReconcileItem struct {
	Source       string    `json:"source"`
	LibraryRoot  string    `json:"library_root"`
	Relative     string    `json:"relative"`
	Series       string    `json:"series"`
	Destination  string    `json:"destination"`
	Action       string    `json:"action"`
	Confidence   float64   `json:"confidence"`
	Reason       string    `json:"reason,omitempty"`
	DuplicateOf  string    `json:"duplicate_of,omitempty"`
	Size         int64     `json:"size"`
	ModifiedAt   time.Time `json:"modified_at"`
	Volume       int       `json:"volume,omitempty"`
	HasVolume    bool      `json:"has_volume"`
	ReviewGroup  string    `json:"review_group,omitempty"`
	AutoSelected bool      `json:"auto_selected,omitempty"`
}

type ReconcileChoice struct {
	ID         string          `json:"id"`
	Series     string          `json:"series"`
	Volume     int             `json:"volume,omitempty"`
	HasVolume  bool            `json:"has_volume"`
	Reason     string          `json:"reason"`
	Candidates []ReconcileItem `json:"candidates"`
}

type ReconcileSummary struct {
	Files      int `json:"files"`
	Keep       int `json:"keep"`
	Move       int `json:"move"`
	Duplicates int `json:"duplicates"`
	Superseded int `json:"superseded"`
	Conflicts  int `json:"conflicts"`
	Review     int `json:"review"`
	Errors     int `json:"errors"`
}

type ReconcileReport struct {
	Root       string            `json:"root,omitempty"`
	Roots      []string          `json:"roots"`
	OutputRoot string            `json:"output_root"`
	Items      []ReconcileItem   `json:"items"`
	Choices    []ReconcileChoice `json:"choices,omitempty"`
	Summary    ReconcileSummary  `json:"summary"`
}

type ReconcileResult struct {
	Moved       int      `json:"moved"`
	Quarantined int      `json:"quarantined"`
	Skipped     int      `json:"skipped"`
	Errors      []string `json:"errors,omitempty"`
}

type reconcileRaw struct {
	path, root, rel, name, series string
	confidence                    float64
	size                          int64
	modified                      time.Time
	volume                        int
	hasVolume                     bool
	err                           error
}

func (o *Organizer) ReconcileScan(root string) (ReconcileReport, error) {
	return o.ReconcileScanMulti([]string{root}, root)
}

func (o *Organizer) ReconcileScanMulti(roots []string, outputRoot string) (ReconcileReport, error) {
	return o.ReconcileScanMultiProgressWithOptions(roots, outputRoot, ReconcileScanOptions{}, nil)
}

func (o *Organizer) ReconcileExecute(root string) (ReconcileResult, error) {
	return o.ReconcileExecuteMulti([]string{root}, root, nil)
}

func (o *Organizer) ReconcileExecuteMulti(roots []string, outputRoot string, selections map[string]string) (ReconcileResult, error) {
	report, err := o.ReconcileScanMulti(roots, outputRoot)
	if err != nil {
		return ReconcileResult{}, err
	}
	return o.ReconcileExecuteReportProgress(report, selections, nil)
}

func normalizeReconcileRoots(roots []string, outputRoot string) ([]string, string, error) {
	if len(roots) == 0 {
		return nil, "", errors.New("at least one library root is required")
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		n, err := normalizeRoot(strings.TrimSpace(root))
		if err != nil {
			return nil, "", err
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, "", errors.New("at least one library root is required")
	}
	if strings.TrimSpace(outputRoot) == "" {
		outputRoot = out[0]
	}
	nOut, err := normalizeRoot(outputRoot)
	if err != nil {
		return nil, "", fmt.Errorf("output root: %w", err)
	}
	return out, nOut, nil
}

func cleanLegacyFolderName(s string) string {
	s = strings.TrimSpace(s)
	for _, suffix := range []string{" 単行本", "【単行本】", "[単行本]", " コミックス", "【コミックス】", "[コミックス]"} {
		s = strings.TrimSpace(strings.TrimSuffix(s, suffix))
	}
	return s
}

func markDestinationConflicts(items []ReconcileItem) {
	for i := range items {
		it := &items[i]
		if it.Action != "move" && it.Action != "keep" {
			continue
		}
		if filepath.Clean(it.Destination) == filepath.Clean(it.Source) {
			continue
		}
		if st, err := os.Lstat(it.Destination); err == nil && st.Mode().IsRegular() {
			it.Action = "conflict"
			it.Reason = "destination already exists but is not a verified duplicate"
		}
	}
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.CopyBuffer(h, f, make([]byte, 4*1024*1024)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func uniqueQuarantinePath(root, series, name string) string {
	base := filepath.Join(root, ".docExtractor-duplicates", series, name)
	if _, err := os.Lstat(base); errors.Is(err, os.ErrNotExist) {
		return base
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 2; n < 10000; n++ {
		p := filepath.Join(root, ".docExtractor-duplicates", series, fmt.Sprintf("%s (%d)%s", stem, n, ext))
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
	}
	return base + ".duplicate"
}

func removeEmptyLibraryDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && path != root && !strings.Contains(path, string(os.PathSeparator)+".docExtractor-duplicates") {
			dirs = append(dirs, path)
		}
		return nil
	})
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(d)
		}
	}
}
