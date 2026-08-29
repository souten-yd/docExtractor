package archive

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type RecoverySummary struct {
	Found        int `json:"found"`
	Promoted     int `json:"promoted"`
	RemovedStale int `json:"removed_stale"`
	InvalidKept  int `json:"invalid_kept"`
	Errors       int `json:"errors"`
}

// RecoverPartials reconciles ZIP .partial files left by a power loss or forced stop.
// A valid partial is atomically promoted only when the final path does not exist.
// Invalid partials are kept for diagnosis and are replaced on the next RAR retry.
func RecoverPartials(ctx context.Context, root string, mode VerifyMode) RecoverySummary {
	var summary RecoverySummary
	_ = filepath.WalkDir(root, func(filename string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			summary.Errors++
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".zip.partial") {
			return nil
		}
		summary.Found++
		if d.Type()&os.ModeSymlink != 0 {
			summary.Errors++
			return nil
		}

		final := strings.TrimSuffix(filename, ".partial")
		if _, err := os.Lstat(final); err == nil {
			if err := os.Remove(filename); err != nil {
				summary.Errors++
			} else {
				summary.RemovedStale++
			}
			return nil
		} else if !os.IsNotExist(err) {
			summary.Errors++
			return nil
		}

		if _, err := VerifyZIP(ctx, filename, mode); err != nil {
			summary.InvalidKept++
			return nil
		}
		if err := os.Rename(filename, final); err != nil {
			summary.Errors++
			return nil
		}
		summary.Promoted++
		return nil
	})
	return summary
}
