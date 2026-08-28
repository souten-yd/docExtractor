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
	"syscall"
	"time"

	"github.com/nwaples/rardecode/v2"
)

type CompressionMode string

type VerifyMode string

const (
	CompressionFast     CompressionMode = "fast"
	CompressionBalanced CompressionMode = "balanced"
	CompressionCompact  CompressionMode = "compact"

	VerifyCentral VerifyMode = "central"
	VerifyFull    VerifyMode = "full"
)

var ErrCrossDeviceRename = errors.New("cross-device rename is disabled to avoid an implicit full-file rewrite")

type Config struct {
	BufferSize        int
	MaxDictionarySize int64
	Compression       CompressionMode
	Verify            VerifyMode
}

type Processor struct {
	cfg Config
}

type Task struct {
	Source       string
	Destination  string
	DeleteSource bool
}

type Progress struct {
	Stage        string
	Progress     float64
	BytesRead    int64
	BytesWritten int64
}

type Result struct {
	Operation    string `json:"operation"`
	Entries      int    `json:"entries"`
	BytesRead    int64  `json:"bytes_read"`
	BytesWritten int64  `json:"bytes_written"`
}

type RARInfo struct {
	Entries         int    `json:"entries"`
	RegularFiles    int    `json:"regular_files"`
	UnpackedBytes   int64  `json:"unpacked_bytes"`
	SingleNestedZIP bool   `json:"single_nested_zip"`
	NestedZIPName   string `json:"nested_zip_name,omitempty"`
	CommonRoot      string `json:"common_root,omitempty"`
}

func New(cfg Config) *Processor {
	if cfg.BufferSize < 256*1024 {
		cfg.BufferSize = 8 * 1024 * 1024
	}
	if cfg.BufferSize > 64*1024*1024 {
		cfg.BufferSize = 64 * 1024 * 1024
	}
	if cfg.MaxDictionarySize <= 0 {
		cfg.MaxDictionarySize = 512 * 1024 * 1024
	}
	if cfg.MaxDictionarySize > 2*1024*1024*1024 {
		cfg.MaxDictionarySize = 2 * 1024 * 1024 * 1024
	}
	if cfg.Compression == "" {
		cfg.Compression = CompressionBalanced
	}
	if cfg.Verify == "" {
		cfg.Verify = VerifyCentral
	}
	return &Processor{cfg: cfg}
}

func (p *Processor) Process(ctx context.Context, task Task, report func(Progress)) (Result, error) {
	if report == nil {
		report = func(Progress) {}
	}
	src := filepath.Clean(task.Source)
	dst := filepath.Clean(task.Destination)
	if src == dst {
		return Result{}, errors.New("source and destination must differ")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if _, err := os.Stat(dst); err == nil {
		return Result{}, fmt.Errorf("destination already exists: %s", filepath.Base(dst))
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return Result{}, err
	}

	switch strings.ToLower(filepath.Ext(src)) {
	case ".zip":
		return p.moveZIP(ctx, src, dst, report)
	case ".rar":
		return p.convertRAR(ctx, src, dst, task.DeleteSource, report)
	default:
		return Result{}, fmt.Errorf("unsupported archive extension: %s", filepath.Ext(src))
	}
}

func (p *Processor) moveZIP(ctx context.Context, src, dst string, report func(Progress)) (Result, error) {
	report(Progress{Stage: "verify-zip"})
	entries, err := VerifyZIP(ctx, src, p.cfg.Verify)
	if err != nil {
		return Result{}, err
	}
	st, err := os.Stat(src)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	report(Progress{Stage: "rename", Progress: 0.95, BytesRead: st.Size()})
	if err := os.Rename(src, dst); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			return Result{}, ErrCrossDeviceRename
		}
		return Result{}, err
	}
	report(Progress{Stage: "done", Progress: 1, BytesRead: st.Size()})
	return Result{Operation: "rename-zip", Entries: entries, BytesRead: st.Size(), BytesWritten: 0}, nil
}

func (p *Processor) convertRAR(ctx context.Context, src, dst string, deleteSource bool, report func(Progress)) (Result, error) {
	report(Progress{Stage: "inspect-rar"})
	info, err := InspectRAR(src)
	if err != nil {
		return Result{}, err
	}
	st, err := os.Stat(src)
	if err != nil {
		return Result{}, err
	}
	if err := ensureFreeSpace(filepath.Dir(dst), estimatedOutputBytes(info, st.Size())); err != nil {
		return Result{}, err
	}
	if info.SingleNestedZIP {
		result, err := p.extractNestedZIP(ctx, src, dst, info, report)
		if err != nil {
			return Result{}, err
		}
		result.BytesRead = st.Size()
		if deleteSource {
			if err := os.Remove(src); err != nil {
				return result, fmt.Errorf("output complete but source removal failed: %w", err)
			}
		}
		return result, nil
	}

	partial := dst + ".partial"
	_ = os.Remove(partial)
	f, err := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return Result{}, err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(partial)
		}
	}()

	zw := zip.NewWriter(f)
	switch p.cfg.Compression {
	case CompressionCompact:
		zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
			return flate.NewWriter(w, flate.DefaultCompression)
		})
	default:
		zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
			return flate.NewWriter(w, flate.BestSpeed)
		})
	}

	rr, err := rardecode.OpenReader(src, rardecode.BufferSize(p.cfg.BufferSize), rardecode.MaxDictionarySize(p.cfg.MaxDictionarySize))
	if err != nil {
		return Result{}, err
	}
	defer rr.Close()

	buf := make([]byte, p.cfg.BufferSize)
	var decoded int64
	entries := 0
	report(Progress{Stage: "rar-to-zip"})
	for {
		if err := ctx.Err(); err != nil {
			_ = zw.Close()
			return Result{}, err
		}
		h, err := rr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = zw.Close()
			return Result{}, err
		}
		if h.IsDir {
			continue
		}
		if h.LinkType != rardecode.LinkTypeNone {
			_ = zw.Close()
			return Result{}, fmt.Errorf("archive links are not allowed: %s", filepath.Base(h.Name))
		}
		name, err := safeArchiveName(h.Name, info.CommonRoot)
		if err != nil {
			_ = zw.Close()
			return Result{}, err
		}
		method := p.methodFor(name)
		zh := &zip.FileHeader{Name: name, Method: method}
		zh.SetMode(0o640)
		if !h.ModificationTime.IsZero() {
			zh.SetModTime(h.ModificationTime)
		} else {
			zh.SetModTime(time.Unix(0, 0).UTC())
		}
		w, err := zw.CreateHeader(zh)
		if err != nil {
			_ = zw.Close()
			return Result{}, err
		}
		_, err = copyWithContext(ctx, w, rr, buf, func(delta int64) {
			decoded += delta
			progress := 0.0
			if info.UnpackedBytes > 0 {
				progress = float64(decoded) / float64(info.UnpackedBytes)
				if progress > 0.94 {
					progress = 0.94
				}
			}
			report(Progress{Stage: "rar-to-zip", Progress: progress, BytesRead: decoded})
		})
		if err != nil {
			_ = zw.Close()
			return Result{}, err
		}
		entries++
	}
	if err := zw.Close(); err != nil {
		return Result{}, err
	}
	if err := f.Sync(); err != nil {
		return Result{}, err
	}
	if err := f.Close(); err != nil {
		return Result{}, err
	}

	outStat, err := os.Stat(partial)
	if err != nil {
		return Result{}, err
	}
	report(Progress{Stage: "verify-output", Progress: 0.96, BytesRead: st.Size(), BytesWritten: outStat.Size()})
	if _, err := VerifyZIP(ctx, partial, p.cfg.Verify); err != nil {
		return Result{}, fmt.Errorf("generated zip verification failed: %w", err)
	}
	if err := os.Rename(partial, dst); err != nil {
		return Result{}, err
	}
	ok = true
	if deleteSource {
		if err := os.Remove(src); err != nil {
			return Result{Operation: "rar-to-zip", Entries: entries, BytesRead: st.Size(), BytesWritten: outStat.Size()}, fmt.Errorf("output complete but source removal failed: %w", err)
		}
	}
	report(Progress{Stage: "done", Progress: 1, BytesRead: st.Size(), BytesWritten: outStat.Size()})
	return Result{Operation: "rar-to-zip", Entries: entries, BytesRead: st.Size(), BytesWritten: outStat.Size()}, nil
}

func (p *Processor) extractNestedZIP(ctx context.Context, src, dst string, info RARInfo, report func(Progress)) (Result, error) {
	partial := dst + ".partial"
	_ = os.Remove(partial)
	out, err := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return Result{}, err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(partial)
		}
	}()

	rr, err := rardecode.OpenReader(src, rardecode.BufferSize(p.cfg.BufferSize), rardecode.MaxDictionarySize(p.cfg.MaxDictionarySize))
	if err != nil {
		return Result{}, err
	}
	defer rr.Close()
	buf := make([]byte, p.cfg.BufferSize)
	var written int64
	found := false
	for {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		h, err := rr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Result{}, err
		}
		if h.IsDir {
			continue
		}
		if h.Name != info.NestedZIPName {
			return Result{}, errors.New("RAR layout changed between inspection and extraction")
		}
		found = true
		report(Progress{Stage: "unwrap-nested-zip"})
		_, err = copyWithContext(ctx, out, rr, buf, func(delta int64) {
			written += delta
			progress := 0.0
			if info.UnpackedBytes > 0 {
				progress = float64(written) / float64(info.UnpackedBytes)
				if progress > 0.94 {
					progress = 0.94
				}
			}
			report(Progress{Stage: "unwrap-nested-zip", Progress: progress, BytesWritten: written})
		})
		if err != nil {
			return Result{}, err
		}
	}
	if !found {
		return Result{}, errors.New("nested ZIP not found")
	}
	if err := out.Sync(); err != nil {
		return Result{}, err
	}
	if err := out.Close(); err != nil {
		return Result{}, err
	}
	report(Progress{Stage: "verify-output", Progress: 0.96, BytesWritten: written})
	entries, err := VerifyZIP(ctx, partial, p.cfg.Verify)
	if err != nil {
		return Result{}, fmt.Errorf("nested zip verification failed: %w", err)
	}
	if err := os.Rename(partial, dst); err != nil {
		return Result{}, err
	}
	ok = true
	report(Progress{Stage: "done", Progress: 1, BytesWritten: written})
	return Result{Operation: "unwrap-nested-zip", Entries: entries, BytesWritten: written}, nil
}

func InspectRAR(filename string) (RARInfo, error) {
	files, err := rardecode.List(filename, rardecode.SkipCheck)
	if err != nil {
		return RARInfo{}, err
	}
	info := RARInfo{Entries: len(files)}
	names := make([]string, 0, len(files))
	for _, f := range files {
		if f.IsDir {
			continue
		}
		if f.LinkType != rardecode.LinkTypeNone {
			continue
		}
		info.RegularFiles++
		if !f.UnKnownSize && f.UnPackedSize > 0 {
			info.UnpackedBytes += f.UnPackedSize
		}
		names = append(names, f.Name)
	}
	info.CommonRoot = commonRoot(names)
	if info.RegularFiles == 1 && len(names) == 1 && strings.EqualFold(filepath.Ext(names[0]), ".zip") {
		info.SingleNestedZIP = true
		info.NestedZIPName = names[0]
	}
	return info, nil
}

func estimatedOutputBytes(info RARInfo, sourceBytes int64) int64 {
	base := info.UnpackedBytes
	if base <= 0 {
		base = sourceBytes * 2
	}
	if info.SingleNestedZIP && info.UnpackedBytes > 0 {
		base = info.UnpackedBytes
	}
	safety := int64(256 * 1024 * 1024)
	if pct := base / 20; pct > safety {
		safety = pct
	}
	if base > (1<<63-1)-safety {
		return 1<<63 - 1
	}
	return base + safety
}

func ensureFreeSpace(dir string, required int64) error {
	if required <= 0 {
		return nil
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(dir, &fs); err != nil {
		return fmt.Errorf("cannot check destination free space: %w", err)
	}
	available := int64(fs.Bavail) * int64(fs.Bsize)
	if available < required {
		return fmt.Errorf("insufficient free space: need about %d MiB, available %d MiB", required/(1024*1024), available/(1024*1024))
	}
	return nil
}

func VerifyZIP(ctx context.Context, filename string, mode VerifyMode) (int, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	count := 0
	buf := make([]byte, 1024*1024)
	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		if _, err := safeArchiveName(f.Name, ""); err != nil {
			return count, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		count++
		if mode != VerifyFull {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return count, err
		}
		_, err = copyWithContext(ctx, io.Discard, rc, buf, nil)
		closeErr := rc.Close()
		if err != nil {
			return count, err
		}
		if closeErr != nil {
			return count, closeErr
		}
	}
	if count == 0 {
		return 0, errors.New("ZIP contains no files")
	}
	return count, nil
}

func (p *Processor) methodFor(name string) uint16 {
	if p.cfg.Compression == CompressionFast || alreadyCompressed(name) {
		return zip.Store
	}
	return zip.Deflate
}

func alreadyCompressed(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".avif", ".heic", ".gif", ".zip", ".rar", ".7z", ".cbz", ".cbr", ".pdf", ".mp3", ".mp4", ".mkv", ".gz", ".xz", ".zst":
		return true
	default:
		return false
	}
}

func safeArchiveName(name, stripRoot string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Clean(name)
	if strings.ContainsRune(name, 0) || name == "." || name == ".." || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("unsafe archive path: %s", filepath.Base(name))
	}
	if stripRoot != "" {
		prefix := strings.TrimSuffix(stripRoot, "/") + "/"
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
		}
	}
	if name == "" || name == "." {
		return "", errors.New("empty archive path")
	}
	return name, nil
}

func commonRoot(names []string) string {
	if len(names) == 0 {
		return ""
	}
	root := ""
	for _, name := range names {
		clean, err := safeArchiveName(name, "")
		if err != nil {
			return ""
		}
		parts := strings.Split(clean, "/")
		if len(parts) < 2 {
			return ""
		}
		if root == "" {
			root = parts[0]
		} else if root != parts[0] {
			return ""
		}
	}
	return root
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader, buf []byte, progress func(int64)) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			wn, writeErr := dst.Write(buf[:n])
			total += int64(wn)
			if progress != nil && wn > 0 {
				progress(int64(wn))
			}
			if writeErr != nil {
				return total, writeErr
			}
			if wn != n {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
