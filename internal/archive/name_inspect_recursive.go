package archive

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nwaples/rardecode/v2"
)

const (
	maxRecursiveNameDepth      = 8
	maxRecursiveNameArchives   = 512
	maxRecursiveNameSpoolBytes = int64(32 * 1024 * 1024 * 1024)
)

type recursiveNameState struct {
	collector    *nameCollector
	tempDir      string
	archives     int
	spooledBytes int64
	entries      int
	truncated    bool
}

// InspectNamesRecursive is used only by the archive-processing workflow.
// Existing-library reprocess/management intentionally never calls it.
//
// It recursively opens embedded ZIP/RAR/CBZ/CBR files and collects naming
// evidence from every reachable level. Only embedded archive payloads are
// spooled to disk; ordinary image/file bodies are not extracted for inspection.
//
// The root archive must be readable. A malformed/unreadable embedded archive is
// non-fatal for name discovery because its filename is still useful evidence;
// the actual archive-normalization step remains strict and will refuse to
// publish an output if that embedded archive cannot be flattened.
func InspectNamesRecursive(filename string) (NameInspection, error) {
	tempDir, err := os.MkdirTemp(filepath.Dir(filename), ".docextractor-name-")
	if err != nil {
		return NameInspection{}, err
	}
	defer os.RemoveAll(tempDir)

	state := &recursiveNameState{collector: newNameCollector(), tempDir: tempDir}
	if err := inspectNamesRecursivePath(filename, 0, state); err != nil {
		return NameInspection{}, err
	}
	out := state.collector.finish()
	out.Entries = state.entries
	out.Truncated = out.Truncated || state.truncated
	return out, nil
}

func inspectNamesRecursivePath(filename string, depth int, state *recursiveNameState) error {
	if depth > maxRecursiveNameDepth {
		state.truncated = true
		return nil
	}
	if state.archives >= maxRecursiveNameArchives {
		state.truncated = true
		return nil
	}
	state.archives++

	switch strings.ToLower(filepath.Ext(filename)) {
	case ".zip", ".cbz":
		return inspectNamesRecursiveZIP(filename, depth, state)
	case ".rar", ".cbr":
		return inspectNamesRecursiveRAR(filename, depth, state)
	default:
		return fmt.Errorf("unsupported archive for recursive name inspection: %s", filepath.Ext(filename))
	}
}

func inspectNamesRecursiveZIP(filename string, depth int, state *recursiveNameState) error {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if state.entries >= maxNameInspectEntries {
			state.truncated = true
			return nil
		}
		state.entries++
		state.collector.add(f.Name, f.FileInfo().IsDir())
		if f.FileInfo().IsDir() || !isSupportedNestedArchiveName(f.Name) {
			continue
		}

		// The member name has already been collected above. Deep inspection is
		// best-effort for naming only; processing itself validates strictly.
		rc, err := f.Open()
		if err != nil {
			state.truncated = true
			continue
		}
		temp, spoolErr := spoolRecursiveNameArchive(rc, filepath.Ext(f.Name), state)
		closeErr := rc.Close()
		if spoolErr != nil || closeErr != nil {
			state.truncated = true
			if temp != "" {
				_ = os.Remove(temp)
			}
			continue
		}
		err = inspectNamesRecursivePath(temp, depth+1, state)
		_ = os.Remove(temp)
		if err != nil {
			state.truncated = true
			continue
		}
	}
	return nil
}

func inspectNamesRecursiveRAR(filename string, depth int, state *recursiveNameState) error {
	rr, err := rardecode.OpenReader(filename)
	if err != nil {
		return err
	}
	defer rr.Close()

	for {
		if state.entries >= maxNameInspectEntries {
			state.truncated = true
			return nil
		}
		h, err := rr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		state.entries++
		if h.LinkType != rardecode.LinkTypeNone {
			continue
		}
		state.collector.add(h.Name, h.IsDir)
		if h.IsDir || !isSupportedNestedArchiveName(h.Name) {
			continue
		}

		// rardecode exposes the current member as a stream. If its payload cannot
		// be recursively inspected, keep the member-name evidence and continue.
		temp, spoolErr := spoolRecursiveNameArchive(rr, filepath.Ext(h.Name), state)
		if spoolErr != nil {
			state.truncated = true
			continue
		}
		err = inspectNamesRecursivePath(temp, depth+1, state)
		_ = os.Remove(temp)
		if err != nil {
			state.truncated = true
			continue
		}
	}
}

func spoolRecursiveNameArchive(src io.Reader, ext string, state *recursiveNameState) (string, error) {
	remaining := maxRecursiveNameSpoolBytes - state.spooledBytes
	if remaining <= 0 {
		state.truncated = true
		return "", errors.New("recursive name inspection spool limit exceeded")
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
		state.truncated = true
		return "", errors.New("recursive name inspection spool limit exceeded")
	}
	state.spooledBytes += n
	if err := f.Close(); err != nil {
		return "", err
	}
	ok = true
	return name, nil
}
