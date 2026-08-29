package archive

import (
	"archive/zip"
	"errors"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nwaples/rardecode/v2"
)

const (
	maxNameInspectEntries    = 5000
	maxNameInspectCandidates = 256
)

type NameCandidateKind string

const (
	CandidateNestedArchive NameCandidateKind = "nested-archive"
	CandidateTopDirectory  NameCandidateKind = "top-directory"
	CandidateNamedImage    NameCandidateKind = "named-image"
)

type NameCandidate struct {
	Name string            `json:"name"`
	Kind NameCandidateKind `json:"kind"`
}

type NameInspection struct {
	Entries    int             `json:"entries"`
	Truncated  bool            `json:"truncated"`
	Candidates []NameCandidate `json:"candidates,omitempty"`
}

func InspectNames(filename string) (NameInspection, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".zip":
		return inspectZIPNames(filename)
	case ".rar":
		return inspectRARNames(filename)
	default:
		return NameInspection{}, errors.New("unsupported archive for name inspection")
	}
}

func inspectZIPNames(filename string) (NameInspection, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return NameInspection{}, err
	}
	defer zr.Close()
	collector := newNameCollector()
	for i, f := range zr.File {
		if i >= maxNameInspectEntries {
			collector.out.Truncated = true
			break
		}
		collector.out.Entries++
		collector.add(f.Name, f.FileInfo().IsDir())
	}
	return collector.finish(), nil
}

func inspectRARNames(filename string) (NameInspection, error) {
	files, err := rardecode.List(filename, rardecode.SkipCheck)
	if err != nil {
		return NameInspection{}, err
	}
	collector := newNameCollector()
	for i, f := range files {
		if i >= maxNameInspectEntries {
			collector.out.Truncated = true
			break
		}
		collector.out.Entries++
		if f.LinkType != rardecode.LinkTypeNone {
			continue
		}
		collector.add(f.Name, f.IsDir)
	}
	return collector.finish(), nil
}

type nameCollector struct {
	out       NameInspection
	seen      map[string]struct{}
	topDirs   map[string]struct{}
	candidate []NameCandidate
}

func newNameCollector() *nameCollector {
	return &nameCollector{seen: make(map[string]struct{}), topDirs: make(map[string]struct{})}
}

func (c *nameCollector) add(raw string, isDir bool) {
	if len(c.candidate) >= maxNameInspectCandidates {
		c.out.Truncated = true
		return
	}
	name := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	name = strings.TrimPrefix(path.Clean("/"+name), "/")
	if name == "" || name == "." {
		return
	}
	parts := strings.Split(name, "/")
	if len(parts) > 1 {
		top := strings.TrimSpace(parts[0])
		if usefulDirectoryName(top) {
			c.topDirs[top] = struct{}{}
		}
	}
	if isDir {
		if len(parts) == 1 && usefulDirectoryName(parts[0]) {
			c.topDirs[parts[0]] = struct{}{}
		}
		return
	}
	base := path.Base(name)
	ext := strings.ToLower(path.Ext(base))
	if ext == ".zip" || ext == ".rar" {
		c.push(strings.TrimSuffix(base, path.Ext(base)), CandidateNestedArchive)
		return
	}
	if isImageExt(ext) {
		stem := strings.TrimSuffix(base, path.Ext(base))
		if usefulImageStem(stem) {
			c.push(stem, CandidateNamedImage)
		}
	}
}

func (c *nameCollector) finish() NameInspection {
	dirs := make([]string, 0, len(c.topDirs))
	for d := range c.topDirs {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		c.push(d, CandidateTopDirectory)
	}
	c.out.Candidates = c.candidate
	return c.out
}

func (c *nameCollector) push(name string, kind NameCandidateKind) {
	name = strings.TrimSpace(name)
	if name == "" || len(c.candidate) >= maxNameInspectCandidates {
		return
	}
	key := string(kind) + "\x00" + strings.ToLower(name)
	if _, ok := c.seen[key]; ok {
		return
	}
	c.seen[key] = struct{}{}
	c.candidate = append(c.candidate, NameCandidate{Name: name, Kind: kind})
}

func usefulDirectoryName(s string) bool {
	n := strings.ToLower(strings.TrimSpace(s))
	if n == "" {
		return false
	}
	switch n {
	case "image", "images", "img", "imgs", "page", "pages", "scan", "scans", "jpg", "jpeg", "png", "webp", "book", "comic", "manga", "__macosx":
		return false
	}
	return !numericLike(n)
}

func usefulImageStem(s string) bool {
	n := strings.TrimSpace(s)
	if len([]rune(n)) < 3 || numericLike(n) {
		return false
	}
	// Camera/scanner-style names are poor title evidence.
	lower := strings.ToLower(n)
	for _, prefix := range []string{"img_", "dsc_", "scan_", "page_", "p_"} {
		if strings.HasPrefix(lower, prefix) && numericLike(strings.TrimPrefix(lower, prefix)) {
			return false
		}
	}
	return true
}

func numericLike(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= '０' && r <= '９') || strings.ContainsRune("-_.()[] ", r) {
			continue
		}
		return false
	}
	return true
}

func isImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".webp", ".avif", ".gif", ".bmp", ".tif", ".tiff":
		return true
	default:
		return false
	}
}

// Keep io imported in builds using older rardecode signatures where EOF may
// surface during metadata enumeration. This also documents that inspection is
// metadata-only and never copies entry bodies.
var _ = io.EOF
