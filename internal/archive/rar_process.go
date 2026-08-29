package archive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nwaples/rardecode/v2"
)

func InspectRAR(filename string) (RARInfo, error) {
	files, err := rardecode.List(filename, rardecode.SkipCheck)
	if err != nil { return RARInfo{}, err }
	info := RARInfo{Entries: len(files)}
	names := make([]string, 0, len(files))
	for _, f := range files {
		if f.IsDir { continue }
		if f.LinkType != rardecode.LinkTypeNone { continue }
		info.RegularFiles++
		if !f.UnKnownSize && f.UnPackedSize > 0 { info.UnpackedBytes += f.UnPackedSize }
		names = append(names, f.Name)
		if isArchiveLikeName(f.Name) { info.NestedArchives++ }
	}
	info.CommonRoot = commonRoot(names)
	if info.RegularFiles == 1 && len(names) == 1 && (strings.EqualFold(filepath.Ext(names[0]), ".zip") || strings.EqualFold(filepath.Ext(names[0]), ".cbz")) {
		info.SingleNestedZIP = true
		info.NestedZIPName = names[0]
	}
	return info, nil
}

func (p *Processor) processRAR(ctx context.Context, src, dst string, deleteSource bool, report func(Progress)) (Result, error) {
	report(Progress{Stage: "inspect-rar"})
	info, err := InspectRAR(src)
	if err != nil { return Result{}, err }
	st, err := os.Stat(src)
	if err != nil { return Result{}, err }
	required := estimatedOutputBytes(info, st.Size())
	if nestedEstimate := estimatedRewriteBytes(st.Size()); nestedEstimate > required { required = nestedEstimate }
	if err := ensureFreeSpace(filepath.Dir(dst), required); err != nil { return Result{}, err }

	// RAR is always fully decoded through the recursive normalizer.  The
	// normalized intermediate is private; only verified per-folder ZIPs are
	// published. This also removes nested ZIP/RAR/CBZ/CBR layers completely.
	normalized := dst + ".normalized.partial"
	entries, _, nested, err := p.flattenArchiveToZIP(ctx, src, normalized, report)
	if err != nil { return Result{}, err }
	defer os.Remove(normalized)
	report(Progress{Stage:"verify-output-no-nested", Progress:.72, BytesRead:st.Size()})
	verifiedEntries, err := VerifyZIPNoNestedArchives(ctx, normalized, p.cfg.Verify)
	if err != nil { return Result{}, fmt.Errorf("generated ZIP verification failed: %w", err) }
	if verifiedEntries != entries { return Result{}, fmt.Errorf("generated ZIP entry count changed during verification: wrote %d verified %d", entries, verifiedEntries) }

	res, err := p.splitNormalizedZIP(ctx, normalized, dst, info.CommonRoot, false, report)
	if err != nil { return Result{}, err }
	if nested > 0 { res.Operation = fmt.Sprintf("rar-recursive-split(%d archives)", nested) } else { res.Operation = "rar-folder-split" }
	res.BytesRead = st.Size()
	if deleteSource {
		if err := RemoveMultipartRARVolumes(src); err != nil { return res, fmt.Errorf("outputs complete but source removal failed: %w", err) }
	}
	report(Progress{Stage:"done", Progress:1, BytesRead:res.BytesRead, BytesWritten:res.BytesWritten})
	return res, nil
}

func estimatedOutputBytes(info RARInfo, sourceBytes int64) int64 {
	base := info.UnpackedBytes
	if base <= 0 { base = sourceBytes * 2 }
	if info.SingleNestedZIP && info.UnpackedBytes > 0 { base = info.UnpackedBytes }
	safety := int64(256 * 1024 * 1024)
	if pct := base / 20; pct > safety { safety = pct }
	const maxInt64 = int64(^uint64(0) >> 1)
	if base > maxInt64-safety { return maxInt64 }
	return base + safety
}
