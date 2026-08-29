package archive

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type ZIPInfo struct {
	Entries        int    `json:"entries"`
	RegularFiles   int    `json:"regular_files"`
	NestedArchives int    `json:"nested_archives"`
	CommonRoot     string `json:"common_root,omitempty"`
}

func InspectZIP(filename string) (ZIPInfo, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return ZIPInfo{}, err
	}
	defer zr.Close()
	info := ZIPInfo{Entries: len(zr.File)}
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		if _, err := safeArchiveName(f.Name, ""); err != nil {
			return ZIPInfo{}, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return ZIPInfo{}, fmt.Errorf("ZIP symlink is not allowed: %s", f.Name)
		}
		info.RegularFiles++
		names = append(names, f.Name)
		if isArchiveLikeName(f.Name) {
			info.NestedArchives++
		}
	}
	info.CommonRoot = commonRoot(names)
	if info.RegularFiles == 0 {
		return ZIPInfo{}, errors.New("ZIP contains no files")
	}
	return info, nil
}

func (p *Processor) processZIP(ctx context.Context, src, dst string, _ bool, report func(Progress)) (Result, error) {
	report(Progress{Stage: "inspect-zip"})
	info, err := InspectZIP(src)
	if err != nil {
		return Result{}, err
	}
	st, err := os.Stat(src)
	if err != nil {
		return Result{}, err
	}

	// Preserve the original low-write fast path. If there is no embedded
	// archive, a valid ZIP is never recompressed or copied.
	if info.NestedArchives == 0 {
		return p.moveZIPWithoutRewrite(ctx, src, dst, st.Size(), report)
	}

	if err := ensureFreeSpace(filepath.Dir(dst), estimatedRewriteBytes(st.Size())); err != nil {
		return Result{}, err
	}
	partial := dst + ".partial"
	entries, _, nested, err := p.flattenArchiveToZIP(ctx, src, partial, report)
	if err != nil {
		return Result{}, err
	}
	outStat, err := os.Stat(partial)
	if err != nil {
		_ = os.Remove(partial)
		return Result{}, err
	}
	report(Progress{Stage: "verify-output-no-nested", Progress: 0.96, BytesRead: st.Size(), BytesWritten: outStat.Size()})
	verifiedEntries, err := VerifyZIPNoNestedArchives(ctx, partial, p.cfg.Verify)
	if err != nil {
		_ = os.Remove(partial)
		return Result{}, fmt.Errorf("normalized ZIP verification failed: %w", err)
	}
	if verifiedEntries != entries {
		_ = os.Remove(partial)
		return Result{}, fmt.Errorf("normalized ZIP entry count changed during verification: wrote %d verified %d", entries, verifiedEntries)
	}
	if err := os.Rename(partial, dst); err != nil {
		_ = os.Remove(partial)
		return Result{}, err
	}
	// ZIP processing historically has move semantics. Remove the source only
	// after the rewritten destination exists and has passed verification.
	if err := os.Remove(src); err != nil {
		return Result{Operation: "flatten-nested-zip", Entries: entries, BytesRead: st.Size(), BytesWritten: outStat.Size()}, fmt.Errorf("output complete but source removal failed: %w", err)
	}
	report(Progress{Stage: "done", Progress: 1, BytesRead: st.Size(), BytesWritten: outStat.Size()})
	return Result{Operation: fmt.Sprintf("flatten-nested-zip(%d)", nested), Entries: entries, BytesRead: st.Size(), BytesWritten: outStat.Size()}, nil
}

func (p *Processor) moveZIPWithoutRewrite(ctx context.Context, src, dst string, sourceBytes int64, report func(Progress)) (Result, error) {
	report(Progress{Stage: "verify-zip"})
	entries, err := VerifyZIP(ctx, src, p.cfg.Verify)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	report(Progress{Stage: "rename", Progress: 0.95, BytesRead: sourceBytes})
	if err := os.Rename(src, dst); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			return Result{}, ErrCrossDeviceRename
		}
		return Result{}, err
	}
	report(Progress{Stage: "done", Progress: 1, BytesRead: sourceBytes})
	return Result{Operation: "rename-zip", Entries: entries, BytesRead: sourceBytes, BytesWritten: 0}, nil
}

func estimatedRewriteBytes(sourceBytes int64) int64 {
	if sourceBytes <= 0 {
		return 512 * 1024 * 1024
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	base := sourceBytes
	if sourceBytes <= maxInt64/2 {
		base = sourceBytes * 2
	} else {
		base = maxInt64
	}
	safety := int64(256 * 1024 * 1024)
	if tenPercent := sourceBytes / 10; tenPercent > safety {
		safety = tenPercent
	}
	if base > maxInt64-safety {
		return maxInt64
	}
	return base + safety
}
