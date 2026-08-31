package archive

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type ZIPInfo struct {
	Entries        int    `json:"entries"`
	RegularFiles   int    `json:"regular_files"`
	NestedArchives int    `json:"nested_archives"`
	CommonRoot     string `json:"common_root,omitempty"`
}

func InspectZIP(filename string) (ZIPInfo, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil { return ZIPInfo{}, err }
	defer zr.Close()
	info := ZIPInfo{Entries: len(zr.File)}
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		if _, err := safeArchiveName(f.Name, ""); err != nil { return ZIPInfo{}, err }
		if f.FileInfo().IsDir() { continue }
		if f.Mode()&os.ModeSymlink != 0 { return ZIPInfo{}, fmt.Errorf("ZIP symlink is not allowed: %s", f.Name) }
		info.RegularFiles++
		names = append(names, f.Name)
		if isArchiveLikeName(f.Name) { info.NestedArchives++ }
	}
	info.CommonRoot = commonRoot(names)
	if info.RegularFiles == 0 { return ZIPInfo{}, errors.New("ZIP contains no files") }
	return info, nil
}

func (p *Processor) processZIP(ctx context.Context, src, dst string, _ bool, targets map[string]string, reconcile bool, report func(Progress)) (Result, error) {
	report(Progress{Stage: "inspect-zip"})
	info, err := InspectZIP(src)
	if err != nil { return Result{}, err }
	st, err := os.Stat(src)
	if err != nil { return Result{}, err }
	groups, err := zipGroups(src)
	if err != nil { return Result{}, err }

	// No nested archive: keep the low-write path.  A ZIP containing exactly one
	// folder is not recompressed; the ZIP itself is renamed to that folder name.
	if info.NestedArchives == 0 {
		if len(groups) == 1 && groups[0].Name != "" {
			target,targetErr:=configuredOutputTarget(dst,groups[0].Name,targets);if targetErr!=nil{return Result{},targetErr}
			res, err := moveOrCopyVerifiedZIP(ctx, src, target, false, reconcile, report)
			if err == nil { report(Progress{Stage:"done", Progress:1, BytesRead:res.BytesRead, BytesWritten:res.BytesWritten}) }
			return res, err
		}
		// Multiple contained folders become one ZIP per folder. Root-level loose
		// files, if present, are kept in the default output ZIP so nothing is lost.
		if len(groups) > 1 {
			res, err := p.splitNormalizedZIP(ctx, src, dst, "", st.ModTime(), targets, reconcile, report)
			if err != nil { return Result{}, err }
			res.BytesRead = st.Size()
			if err := os.Remove(src); err != nil { return res, fmt.Errorf("outputs complete but source removal failed: %w", err) }
			report(Progress{Stage:"done", Progress:1, BytesRead:res.BytesRead, BytesWritten:res.BytesWritten})
			return res, nil
		}
		// ZIP with files directly at root: retain the planned name, supporting a
		// user-selected destination on another QNAP volume via verified copy.
		res, err := moveOrCopyVerifiedZIP(ctx, src, dst, false, reconcile, report)
		if err == nil { report(Progress{Stage:"done", Progress:1, BytesRead:res.BytesRead, BytesWritten:res.BytesWritten}) }
		return res, err
	}

	// Nested archive(s): recursively flatten first. The normalized intermediate
	// ZIP is never published. It is then split by the folders revealed after
	// recursive expansion, and every published ZIP is verified before source
	// removal.
	if err := ensureFreeSpace(filepath.Dir(dst), estimatedRewriteBytes(st.Size())); err != nil { return Result{}, err }
	normalized := dst + ".normalized.partial"
	entries, _, nested, err := p.flattenArchiveToZIP(ctx, src, normalized, report)
	if err != nil { return Result{}, err }
	defer os.Remove(normalized)
	report(Progress{Stage:"verify-output-no-nested", Progress:.72, BytesRead:st.Size()})
	verified, err := VerifyZIPNoNestedArchives(ctx, normalized, p.cfg.Verify)
	if err != nil { return Result{}, fmt.Errorf("normalized ZIP verification failed: %w", err) }
	if verified != entries { return Result{}, fmt.Errorf("normalized ZIP entry count changed during verification: wrote %d verified %d", entries, verified) }
	res, err := p.splitNormalizedZIP(ctx, normalized, dst, info.CommonRoot, st.ModTime(), targets, reconcile, report)
	if err != nil { return Result{}, err }
	res.Operation = fmt.Sprintf("flatten-and-split(%d archives)", nested)
	res.BytesRead = st.Size()
	if err := os.Remove(src); err != nil { return res, fmt.Errorf("outputs complete but source removal failed: %w", err) }
	report(Progress{Stage:"done", Progress:1, BytesRead:res.BytesRead, BytesWritten:res.BytesWritten})
	return res, nil
}

// Retained for compatibility with older tests/callers; new ZIP processing uses
// moveOrCopyVerifiedZIP so a configured result directory on another volume is
// supported instead of failing with EXDEV.
func (p *Processor) moveZIPWithoutRewrite(ctx context.Context, src, dst string, sourceBytes int64, report func(Progress)) (Result, error) {
	res, err := moveOrCopyVerifiedZIP(ctx, src, dst, false, false, report)
	if err == nil && res.BytesRead == 0 { res.BytesRead = sourceBytes }
	return res, err
}

func estimatedRewriteBytes(sourceBytes int64) int64 {
	if sourceBytes <= 0 { return 512 * 1024 * 1024 }
	const maxInt64 = int64(^uint64(0) >> 1)
	base := sourceBytes
	if sourceBytes <= maxInt64/2 { base = sourceBytes * 2 } else { base = maxInt64 }
	safety := int64(256 * 1024 * 1024)
	if tenPercent := sourceBytes / 10; tenPercent > safety { safety = tenPercent }
	if base > maxInt64-safety { return maxInt64 }
	return base + safety
}
