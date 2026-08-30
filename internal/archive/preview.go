package archive

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nwaples/rardecode/v2"
)

const (
	previewMaxNestedDepth    = 8
	previewMaxNestedArchives = 1024
	previewMaxArchiveEntries = 250000
	previewMaxSpoolBytes     = int64(128 * 1024 * 1024 * 1024)
)

// OutputPreview describes the output paths that archive processing is expected
// to publish without modifying the source archive.
type OutputPreview struct {
	Targets []string
	Groups  []string
	Nested  bool
	Warning string
}

type outputPreviewState struct {
	tempDir             string
	groups              map[string]struct{}
	preferredSingleName string
	archives            int
	nestedArchives      int
	entries             int
	spooledBytes        int64
}

// PreviewOutputTargets mirrors the recursive grouping performed by Process,
// but only reads archive metadata and embedded archive payloads. Ordinary file
// bodies are not extracted or rewritten.
func PreviewOutputTargets(source, defaultDst string) (OutputPreview, error) {
	switch strings.ToLower(filepath.Ext(source)) {
	case ".zip", ".cbz", ".rar", ".cbr":
	default:
		return OutputPreview{Targets: []string{defaultDst}}, nil
	}

	tempDir, err := os.MkdirTemp(filepath.Dir(source), ".docextractor-preview-")
	if err != nil {
		return OutputPreview{}, err
	}
	defer os.RemoveAll(tempDir)

	state := &outputPreviewState{tempDir: tempDir, groups: map[string]struct{}{}}
	if err := previewArchivePath(source, "", 0, state); err != nil {
		return OutputPreview{}, err
	}
	if len(state.groups) == 0 {
		return OutputPreview{}, errors.New("archive contains no regular files after normalization")
	}
	if len(state.groups) == 1 {
		if _, ok := state.groups[""]; ok && state.preferredSingleName != "" {
			state.groups = map[string]struct{}{state.preferredSingleName: {}}
		}
	}

	groups := make([]string, 0, len(state.groups))
	for group := range state.groups {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return strings.ToLower(groups[i]) < strings.ToLower(groups[j])
	})
	targets := make([]string, 0, len(groups))
	for _, group := range groups {
		targets = append(targets, outputForFolder(defaultDst, group))
	}
	nested := state.nestedArchives > 0
	return OutputPreview{Targets: targets, Groups: groups, Nested: nested, Warning: previewWarning(nested)}, nil
}

func previewArchivePath(filename, prefix string, depth int, state *outputPreviewState) error {
	if depth > previewMaxNestedDepth {
		return fmt.Errorf("nested archive depth exceeds preview limit %d", previewMaxNestedDepth)
	}
	if state.archives >= previewMaxNestedArchives {
		return fmt.Errorf("nested archive count exceeds preview limit %d", previewMaxNestedArchives)
	}
	state.archives++

	switch strings.ToLower(filepath.Ext(filename)) {
	case ".zip", ".cbz":
		return previewZIP(filename, prefix, depth, state)
	case ".rar", ".cbr":
		return previewRAR(filename, prefix, depth, state)
	case ".7z":
		return fmt.Errorf("nested 7z is not supported: %s", filepath.Base(filename))
	default:
		return fmt.Errorf("unsupported nested archive type: %s", filepath.Ext(filename))
	}
}

func previewZIP(filename, prefix string, depth int, state *outputPreviewState) error {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return err
	}
	defer zr.Close()

	regular := make([]*zip.File, 0, len(zr.File))
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		if _, err := safeArchiveName(f.Name, ""); err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("ZIP symlink is not allowed: %s", f.Name)
		}
		if err := countPreviewEntry(state); err != nil {
			return err
		}
		regular = append(regular, f)
		names = append(names, f.Name)
	}

	stripRoot := ""
	if depth == 0 {
		stripRoot = commonRoot(names)
		state.preferredSingleName = stripRoot
	}
	transparent := len(regular) == 1 && isArchiveLikeName(regular[0].Name)
	for _, f := range regular {
		logical, err := safeArchiveName(f.Name, stripRoot)
		if err != nil {
			return err
		}
		if isSupportedNestedArchiveName(logical) {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			temp, spoolErr := spoolPreviewArchive(rc, filepath.Ext(logical), state)
			closeErr := rc.Close()
			if spoolErr != nil {
				return spoolErr
			}
			if closeErr != nil {
				_ = os.Remove(temp)
				return closeErr
			}
			state.nestedArchives++
			childPrefix := nestedChildPrefix(prefix, logical, transparent)
			err = previewArchivePath(temp, childPrefix, depth+1, state)
			_ = os.Remove(temp)
			if err != nil {
				return err
			}
			continue
		}
		if isArchiveLikeName(logical) {
			return fmt.Errorf("unsupported nested archive would remain in output: %s", logical)
		}
		if err := addPreviewLeaf(prefix, logical, state); err != nil {
			return err
		}
	}
	return nil
}

func previewRAR(filename, prefix string, depth int, state *outputPreviewState) error {
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
		if err := countPreviewEntry(state); err != nil {
			return err
		}
		regularCount++
		soleName = f.Name
		names = append(names, f.Name)
	}

	stripRoot := ""
	if depth == 0 {
		stripRoot = commonRoot(names)
		state.preferredSingleName = stripRoot
	}
	transparent := regularCount == 1 && isArchiveLikeName(soleName)
	rr, err := rardecode.OpenReader(filename)
	if err != nil {
		return err
	}
	defer rr.Close()
	for {
		h, err := rr.Next()
		if errors.Is(err, io.EOF) {
			return nil
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
			temp, err := spoolPreviewArchive(rr, filepath.Ext(logical), state)
			if err != nil {
				return err
			}
			state.nestedArchives++
			childPrefix := nestedChildPrefix(prefix, logical, transparent)
			err = previewArchivePath(temp, childPrefix, depth+1, state)
			_ = os.Remove(temp)
			if err != nil {
				return err
			}
			continue
		}
		if isArchiveLikeName(logical) {
			return fmt.Errorf("unsupported nested archive would remain in output: %s", logical)
		}
		if err := addPreviewLeaf(prefix, logical, state); err != nil {
			return err
		}
	}
}

func countPreviewEntry(state *outputPreviewState) error {
	state.entries++
	if state.entries > previewMaxArchiveEntries {
		return fmt.Errorf("archive entry count exceeds preview limit %d", previewMaxArchiveEntries)
	}
	return nil
}

func addPreviewLeaf(prefix, logical string, state *outputPreviewState) error {
	name, err := joinedArchiveName(prefix, logical)
	if err != nil {
		return err
	}
	parts := strings.Split(name, "/")
	group := ""
	if len(parts) > 1 {
		group = parts[0]
	}
	state.groups[group] = struct{}{}
	return nil
}

func spoolPreviewArchive(src io.Reader, ext string, state *outputPreviewState) (string, error) {
	remaining := previewMaxSpoolBytes - state.spooledBytes
	if remaining <= 0 {
		return "", errors.New("recursive output preview spool limit exceeded")
	}
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
	n, err := io.Copy(f, io.LimitReader(src, remaining+1))
	if err != nil {
		return "", err
	}
	if n > remaining {
		return "", errors.New("recursive output preview spool limit exceeded")
	}
	state.spooledBytes += n
	if err := f.Close(); err != nil {
		return "", err
	}
	ok = true
	return name, nil
}

func previewWarning(nested bool) string {
	if nested {
		return "nested ZIP/RAR detected; predicted outputs include recursive normalization and final folder split"
	}
	return ""
}
