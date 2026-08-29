package organizer

import (
	"os"
	"path/filepath"
	"strings"
)

type ReconcileScanOptions struct {
	IncludeQuarantine bool `json:"include_quarantine,omitempty"`
}

func skipReconcileDir(d os.DirEntry, opts ReconcileScanOptions) bool {
	return d.IsDir() && d.Name() == ".docExtractor-duplicates" && !opts.IncludeQuarantine
}

func inQuarantine(root, path string) bool {
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil {
		return false
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	return len(parts) > 0 && parts[0] == ".docExtractor-duplicates"
}

func parentSeriesEvidence(root, path, inferred string, confidence, threshold float64) (string, float64) {
	parent := filepath.Base(filepath.Dir(path))
	if filepath.Dir(path) == root || parent == "." || parent == "" || parent == ".docExtractor-duplicates" || !seriesNameUsable(parent) {
		return inferred, confidence
	}
	parent = cleanLegacyFolderName(parent)
	if !seriesNameUsable(parent) {
		return inferred, confidence
	}
	if seriesNameUsable(inferred) {
		if score, _ := sameSeries(inferred, parent); score >= .90 {
			if richerSeries(parent, inferred) {
				inferred = parent
			}
			if confidence < .90 {
				confidence = .90
			}
			return inferred, confidence
		}
	}
	if !seriesNameUsable(inferred) || confidence < threshold {
		return parent, .78
	}
	return inferred, confidence
}
