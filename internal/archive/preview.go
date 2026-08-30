package archive

import (
	"archive/zip"
	"errors"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nwaples/rardecode/v2"
)

// OutputPreview describes the output paths that archive processing is expected
// to publish without modifying the source archive.
type OutputPreview struct {
	Targets []string
	Groups []string
	Nested bool
	Warning string
}

func PreviewOutputTargets(source, defaultDst string) (OutputPreview, error) {
	groups := map[string]struct{}{}
	nested := false
	add := func(name string, isDir bool) error {
		if isDir { return nil }
		clean, err := safeArchiveName(name, "")
		if err != nil { return err }
		if isSupportedNestedArchiveName(clean) { nested = true }
		parts := strings.Split(clean, "/")
		g := ""
		if len(parts) > 1 { g = parts[0] }
		groups[g] = struct{}{}
		return nil
	}
	switch strings.ToLower(filepath.Ext(source)) {
	case ".zip", ".cbz":
		zr, err := zip.OpenReader(source); if err != nil { return OutputPreview{}, err }; defer zr.Close()
		for _, f := range zr.File { if err := add(f.Name, f.FileInfo().IsDir()); err != nil { return OutputPreview{}, err } }
	case ".rar", ".cbr":
		rr, err := rardecode.OpenReader(source); if err != nil { return OutputPreview{}, err }; defer rr.Close()
		for { h, err := rr.Next(); if errors.Is(err, io.EOF) { break }; if err != nil { return OutputPreview{}, err }; if h.LinkType != rardecode.LinkTypeNone { continue }; if err := add(h.Name, h.IsDir); err != nil { return OutputPreview{}, err } }
	default:
		return OutputPreview{Targets: []string{defaultDst}}, nil
	}
	if len(groups) == 0 { return OutputPreview{Targets: []string{defaultDst}}, nil }
	names := make([]string,0,len(groups)); for g := range groups { names=append(names,g) }; sort.Strings(names)
	if len(names)==1 && names[0]=="" { return OutputPreview{Targets:[]string{defaultDst},Groups:names,Nested:nested,Warning:previewWarning(nested)},nil }
	targets := make([]string,0,len(names)); for _,g := range names { targets=append(targets, outputForFolder(defaultDst,g)) }
	return OutputPreview{Targets:targets,Groups:names,Nested:nested,Warning:previewWarning(nested)},nil
}

func previewWarning(nested bool) string {
	if nested { return "nested ZIP/RAR detected; final folder split can expand after recursive normalization" }
	return ""
}
