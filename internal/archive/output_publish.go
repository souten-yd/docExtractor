package archive

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/souten-yd/docExtractor/internal/dedupe"
)

var outputPublishMu sync.Mutex

// publishCandidate serializes only the short final publication step. Archive
// decoding and ZIP creation still run in parallel, while overlapping volumes
// cannot race each other during duplicate/variant selection.
func publishCandidate(candidate, target string, reconcile bool) (dedupe.Decision, error) {
	outputPublishMu.Lock()
	defer outputPublishMu.Unlock()

	st, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return dedupe.UseCandidate, os.Rename(candidate, target)
	}
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("output target is not a regular file: %s", target)
	}
	if !reconcile {
		return "", fmt.Errorf("destination already exists: %s", filepath.Base(target))
	}

	decision, err := dedupe.CompareFiles(target, candidate)
	if err != nil {
		return "", err
	}
	outputRoot := filepath.Dir(filepath.Dir(target))
	series := filepath.Base(filepath.Dir(target))
	quarantine := dedupe.UniqueQuarantinePath(outputRoot, series, filepath.Base(target))
	if err := os.MkdirAll(filepath.Dir(quarantine), 0o750); err != nil {
		return "", err
	}

	if decision != dedupe.UseCandidate {
		if err := os.Rename(candidate, quarantine); err != nil {
			return "", err
		}
		return decision, nil
	}
	if err := os.Rename(target, quarantine); err != nil {
		return "", err
	}
	if err := os.Rename(candidate, target); err != nil {
		if rollbackErr := os.Rename(quarantine, target); rollbackErr != nil {
			return "", fmt.Errorf("publish failed: %v; existing output rollback failed: %w", err, rollbackErr)
		}
		return "", err
	}
	return decision, nil
}
