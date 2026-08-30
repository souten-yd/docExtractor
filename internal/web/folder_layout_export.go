package web

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Server) folderLayoutExport(w http.ResponseWriter, r *http.Request) {
	raw, err := url.QueryUnescape(r.URL.Query().Get("root"))
	if err != nil {
		http.Error(w, "invalid root", http.StatusBadRequest)
		return
	}
	root := filepath.Clean(strings.TrimSpace(raw))
	if root == "." || !filepath.IsAbs(root) || !withinPath(s.browseBase(), root) {
		http.Error(w, "root must be an absolute path inside the allowed share root", http.StatusBadRequest)
		return
	}
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		http.Error(w, "root is not an accessible directory", http.StatusBadRequest)
		return
	}

	// Count first so the header remains useful when the TXT is reviewed later.
	// WalkDir does not follow directory symlinks. Internal application state is
	// intentionally omitted because this export describes the user library.
	dirs, files := 0, 0
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root { return walkErr }
			return nil
		}
		if path == root { return nil }
		if d.IsDir() && d.Name() == ".docExtractor-state" { return filepath.SkipDir }
		if d.IsDir() { dirs++ } else { files++ }
		return nil
	})
	if err != nil {
		http.Error(w, "layout scan failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	stamp := time.Now().Format("20060102-150405")
	filename := "docextractor-layout-" + stamp + ".txt"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")
	bw := bufio.NewWriterSize(w, 64*1024)
	defer bw.Flush()
	fmt.Fprintf(bw, "docExtractor folder layout\n")
	fmt.Fprintf(bw, "generated_at: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(bw, "root: %s\n", root)
	fmt.Fprintf(bw, "directories: %d\nfiles: %d\n\n", dirs, files)

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			rel, _ := filepath.Rel(root, path)
			fmt.Fprintf(bw, "!ERROR\t%s\t%s\n", filepath.ToSlash(rel), walkErr.Error())
			if path == root { return walkErr }
			return nil
		}
		if path == root { return nil }
		if d.IsDir() && d.Name() == ".docExtractor-state" { return filepath.SkipDir }
		rel, err := filepath.Rel(root, path)
		if err != nil { return nil }
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			fmt.Fprintf(bw, "D\t%s/\n", rel)
		} else if d.Type()&os.ModeSymlink != 0 {
			fmt.Fprintf(bw, "L\t%s\n", rel)
		} else {
			fmt.Fprintf(bw, "F\t%s\n", rel)
		}
		return nil
	})
}
