package archive

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
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

	// Recursive archive normalization limits. Processing remains streaming;
	// these limits are for malformed archives / archive bombs, not RAM sizing.
	MaxNestedDepth    int
	MaxNestedArchives int
	MaxArchiveEntries int
	MaxExpandedBytes  int64
}

type Processor struct{ cfg Config }

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
	NestedArchives  int    `json:"nested_archives,omitempty"`
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
	if cfg.MaxNestedDepth <= 0 {
		cfg.MaxNestedDepth = 8
	}
	if cfg.MaxNestedDepth > 32 {
		cfg.MaxNestedDepth = 32
	}
	if cfg.MaxNestedArchives <= 0 {
		cfg.MaxNestedArchives = 1024
	}
	if cfg.MaxArchiveEntries <= 0 {
		cfg.MaxArchiveEntries = 250000
	}
	if cfg.MaxExpandedBytes <= 0 {
		cfg.MaxExpandedBytes = 128 * 1024 * 1024 * 1024
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
	case ".zip", ".cbz":
		return p.processZIP(ctx, src, dst, task.DeleteSource, report)
	case ".rar", ".cbr":
		return p.processRAR(ctx, src, dst, task.DeleteSource, report)
	default:
		return Result{}, fmt.Errorf("unsupported archive extension: %s", filepath.Ext(src))
	}
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
		prefix := strings.TrimSuffix(strings.ReplaceAll(stripRoot, "\\", "/"), "/") + "/"
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
