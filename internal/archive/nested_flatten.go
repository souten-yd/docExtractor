package archive

import (
	"archive/zip"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwaples/rardecode/v2"
)

type flattenState struct {
	tempDir        string
	seen           map[string]struct{}
	scannedEntries int
	outputEntries  int
	nestedArchives int
	expandedBytes  int64
}

func isSupportedNestedArchiveName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".zip", ".cbz", ".rar", ".cbr":
		return true
	default:
		return false
	}
}

func isArchiveLikeName(name string) bool {
	if isSupportedNestedArchiveName(name) {
		return true
	}
	// 7z cannot currently be decoded by the QPKG. Treat it as an archive so a
	// normalization never claims success while silently leaving one embedded.
	return strings.EqualFold(filepath.Ext(name), ".7z")
}

func (p *Processor) flattenArchiveToZIP(ctx context.Context, src, partial string, report func(Progress)) (entries int, expanded int64, nested int, err error) {
	_ = os.Remove(partial)
	out, err := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return 0, 0, 0, err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(partial)
		}
	}()

	tempDir, err := os.MkdirTemp(filepath.Dir(partial), ".docextractor-nested-")
	if err != nil {
		return 0, 0, 0, err
	}
	defer os.RemoveAll(tempDir)

	zw := zip.NewWriter(out)
	p.registerCompressor(zw)
	state := &flattenState{tempDir: tempDir, seen: map[string]struct{}{}}
	report(Progress{Stage: "flatten-nested-archives"})
	if err := p.flattenPath(ctx, src, "", 0, zw, state, report); err != nil {
		_ = zw.Close()
		return state.outputEntries, state.expandedBytes, state.nestedArchives, err
	}
	if state.outputEntries == 0 {
		_ = zw.Close()
		return 0, state.expandedBytes, state.nestedArchives, errors.New("archive contains no regular files after normalization")
	}
	if err := zw.Close(); err != nil {
		return state.outputEntries, state.expandedBytes, state.nestedArchives, err
	}
	if err := out.Sync(); err != nil {
		return state.outputEntries, state.expandedBytes, state.nestedArchives, err
	}
	if err := out.Close(); err != nil {
		return state.outputEntries, state.expandedBytes, state.nestedArchives, err
	}
	ok = true
	return state.outputEntries, state.expandedBytes, state.nestedArchives, nil
}

func (p *Processor) registerCompressor(zw *zip.Writer) {
	level := flate.BestSpeed
	if p.cfg.Compression == CompressionCompact {
		level = flate.DefaultCompression
	}
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, level)
	})
}

func (p *Processor) flattenPath(ctx context.Context, filename, prefix string, depth int, zw *zip.Writer, state *flattenState, report func(Progress)) error {
	if depth > p.cfg.MaxNestedDepth {
		return fmt.Errorf("nested archive depth exceeds limit %d", p.cfg.MaxNestedDepth)
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".zip", ".cbz":
		return p.flattenZIP(ctx, filename, prefix, depth, zw, state, report)
	case ".rar", ".cbr":
		return p.flattenRAR(ctx, filename, prefix, depth, zw, state, report)
	case ".7z":
		return fmt.Errorf("nested 7z is not supported: %s", filepath.Base(filename))
	default:
		return fmt.Errorf("unsupported nested archive type: %s", filepath.Ext(filename))
	}
}

func (p *Processor) flattenZIP(ctx context.Context, filename, prefix string, depth int, zw *zip.Writer, state *flattenState, report func(Progress)) error {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return err
	}
	defer zr.Close()

	regular := make([]*zip.File, 0, len(zr.File))
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := safeArchiveName(f.Name, ""); err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("ZIP symlink is not allowed: %s", f.Name)
		}
		state.scannedEntries++
		if state.scannedEntries > p.cfg.MaxArchiveEntries {
			return fmt.Errorf("archive entry count exceeds limit %d", p.cfg.MaxArchiveEntries)
		}
		regular = append(regular, f)
		names = append(names, f.Name)
	}

	stripRoot := ""
	if depth == 0 {
		stripRoot = commonRoot(names)
	}
	transparent := len(regular) == 1 && isArchiveLikeName(regular[0].Name)
	for _, f := range regular {
		logical, err := safeArchiveName(f.Name, stripRoot)
		if err != nil {
			return err
		}
		if isSupportedNestedArchiveName(logical) {
			if err := p.consumeNestedZIPEntry(ctx, f, logical, prefix, transparent, depth, zw, state, report); err != nil {
				return err
			}
			continue
		}
		if isArchiveLikeName(logical) {
			return fmt.Errorf("unsupported nested archive would remain in output: %s", logical)
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = p.writeLeaf(ctx, zw, state, prefix, logical, f.Modified, rc, report)
		closeErr := rc.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (p *Processor) consumeNestedZIPEntry(ctx context.Context, f *zip.File, logical, prefix string, transparent bool, depth int, zw *zip.Writer, state *flattenState, report func(Progress)) error {
	if state.nestedArchives >= p.cfg.MaxNestedArchives {
		return fmt.Errorf("nested archive count exceeds limit %d", p.cfg.MaxNestedArchives)
	}
	state.nestedArchives++
	rc, err := f.Open()
	if err != nil {
		return err
	}
	temp, err := p.spoolNested(ctx, rc, filepath.Ext(logical), state)
	closeErr := rc.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		_ = os.Remove(temp)
		return closeErr
	}
	defer os.Remove(temp)
	childPrefix := nestedChildPrefix(prefix, logical, transparent)
	return p.flattenPath(ctx, temp, childPrefix, depth+1, zw, state, report)
}

func (p *Processor) flattenRAR(ctx context.Context, filename, prefix string, depth int, zw *zip.Writer, state *flattenState, report func(Progress)) error {
	listed, err := rardecode.List(filename, rardecode.SkipCheck)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(listed))
	regularCount := 0
	soleName := ""
	for _, f := range listed {
		if f.IsDir {
			continue
		}
		if f.LinkType != rardecode.LinkTypeNone {
			return fmt.Errorf("archive links are not allowed: %s", filepath.Base(f.Name))
		}
		if _, err := safeArchiveName(f.Name, ""); err != nil {
			return err
		}
		regularCount++
		soleName = f.Name
		names = append(names, f.Name)
		state.scannedEntries++
		if state.scannedEntries > p.cfg.MaxArchiveEntries {
			return fmt.Errorf("archive entry count exceeds limit %d", p.cfg.MaxArchiveEntries)
		}
	}
	stripRoot := ""
	if depth == 0 {
		stripRoot = commonRoot(names)
	}
	transparent := regularCount == 1 && isArchiveLikeName(soleName)

	rr, err := rardecode.OpenReader(filename, rardecode.BufferSize(p.cfg.BufferSize), rardecode.MaxDictionarySize(p.cfg.MaxDictionarySize))
	if err != nil {
		return err
	}
	defer rr.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := rr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if h.IsDir {
			continue
		}
		if h.LinkType != rardecode.LinkTypeNone {
			return fmt.Errorf("archive links are not allowed: %s", filepath.Base(h.Name))
		}
		logical, err := safeArchiveName(h.Name, stripRoot)
		if err != nil {
			return err
		}
		if isSupportedNestedArchiveName(logical) {
			if state.nestedArchives >= p.cfg.MaxNestedArchives {
				return fmt.Errorf("nested archive count exceeds limit %d", p.cfg.MaxNestedArchives)
			}
			state.nestedArchives++
			temp, err := p.spoolNested(ctx, rr, filepath.Ext(logical), state)
			if err != nil {
				return err
			}
			childPrefix := nestedChildPrefix(prefix, logical, transparent)
			err = p.flattenPath(ctx, temp, childPrefix, depth+1, zw, state, report)
			_ = os.Remove(temp)
			if err != nil {
				return err
			}
			continue
		}
		if isArchiveLikeName(logical) {
			return fmt.Errorf("unsupported nested archive would remain in output: %s", logical)
		}
		if err := p.writeLeaf(ctx, zw, state, prefix, logical, h.ModificationTime, rr, report); err != nil {
			return err
		}
	}
	return nil
}

func (p *Processor) spoolNested(ctx context.Context, src io.Reader, ext string, state *flattenState) (string, error) {
	if ext == "" {
		ext = ".archive"
	}
	f, err := os.CreateTemp(state.tempDir, "nested-*"+strings.ToLower(ext))
	if err != nil {
		return "", err
	}
	name := f.Name()
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	buf := make([]byte, p.cfg.BufferSize)
	if _, err := copyWithContext(ctx, f, src, buf, nil); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	ok = true
	return name, nil
}

func (p *Processor) writeLeaf(ctx context.Context, zw *zip.Writer, state *flattenState, prefix, logical string, modTime time.Time, src io.Reader, report func(Progress)) error {
	name, err := joinedArchiveName(prefix, logical)
	if err != nil {
		return err
	}
	name = uniqueArchiveName(name, state.seen)
	if isArchiveLikeName(name) {
		return fmt.Errorf("refusing to emit nested archive: %s", name)
	}
	state.outputEntries++
	if state.outputEntries > p.cfg.MaxArchiveEntries {
		return fmt.Errorf("output entry count exceeds limit %d", p.cfg.MaxArchiveEntries)
	}

	zh := &zip.FileHeader{Name: name, Method: p.methodFor(name)}
	zh.SetMode(0o640)
	if modTime.IsZero() {
		modTime = time.Unix(0, 0).UTC()
	}
	zh.SetModTime(modTime)
	w, err := zw.CreateHeader(zh)
	if err != nil {
		return err
	}

	remaining := p.cfg.MaxExpandedBytes - state.expandedBytes
	if remaining <= 0 {
		return fmt.Errorf("expanded data exceeds limit %d MiB", p.cfg.MaxExpandedBytes/(1024*1024))
	}
	limited := io.LimitReader(src, remaining+1)
	buf := make([]byte, p.cfg.BufferSize)
	n, err := copyWithContext(ctx, w, limited, buf, func(delta int64) {
		report(Progress{Stage: "flatten-nested-archives", BytesRead: state.expandedBytes + delta})
	})
	if err != nil {
		return err
	}
	if n > remaining {
		return fmt.Errorf("expanded data exceeds limit %d MiB", p.cfg.MaxExpandedBytes/(1024*1024))
	}
	state.expandedBytes += n
	report(Progress{Stage: "flatten-nested-archives", BytesRead: state.expandedBytes})
	return nil
}

func nestedChildPrefix(prefix, logical string, transparent bool) string {
	dir := path.Dir(logical)
	if dir == "." {
		dir = ""
	}
	if transparent {
		if dir == "" {
			return prefix
		}
		return path.Join(prefix, dir)
	}
	base := path.Base(logical)
	stem := strings.TrimSuffix(base, path.Ext(base))
	return path.Join(prefix, dir, stem)
}

func joinedArchiveName(prefix, logical string) (string, error) {
	if prefix == "" {
		return safeArchiveName(logical, "")
	}
	return safeArchiveName(path.Join(prefix, logical), "")
}

func uniqueArchiveName(name string, seen map[string]struct{}) string {
	key := strings.ToLower(name)
	if _, ok := seen[key]; !ok {
		seen[key] = struct{}{}
		return name
	}
	dir, base := path.Split(name)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for n := 2; n < 100000; n++ {
		candidate := path.Join(dir, fmt.Sprintf("%s (%d)%s", stem, n, ext))
		key = strings.ToLower(candidate)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			return candidate
		}
	}
	candidate := path.Join(dir, stem+" (duplicate)"+ext)
	seen[strings.ToLower(candidate)] = struct{}{}
	return candidate
}
