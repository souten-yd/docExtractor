package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/souten-yd/docExtractor/internal/diagnostics"
	appsettings "github.com/souten-yd/docExtractor/internal/settings"
)

type settingsRequest struct {
	Root string `json:"root"`
}

type directoryEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type directoryResponse struct {
	Path    string           `json:"path"`
	Parent  string           `json:"parent,omitempty"`
	Entries []directoryEntry `json:"entries"`
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, appsettings.Settings{Root: s.Organizer.Root()})
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		http.Error(w, "settings store unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req settingsRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	root := strings.TrimSpace(req.Root)
	if root == "" {
		http.Error(w, "root is required", http.StatusBadRequest)
		return
	}
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) || !withinPath(s.browseBase(), root) {
		http.Error(w, "root must be an absolute path inside the allowed share root", http.StatusBadRequest)
		return
	}

	oldRoot := s.Organizer.Root()
	if err := s.Organizer.SetRoot(root); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.Settings.Save(appsettings.Settings{Root: s.Organizer.Root()}); err != nil {
		_ = s.Organizer.SetRoot(oldRoot)
		http.Error(w, "failed to persist settings", http.StatusInternalServerError)
		return
	}
	if s.Diagnostics != nil {
		if logger, err := s.Diagnostics.Job("system"); err == nil {
			_ = logger.Write(diagnostics.Event{
				Component: "settings", Stage: "save", Message: "archive root changed",
				Fields: map[string]any{"old_path": oldRoot, "new_path": s.Organizer.Root()},
			})
		}
	}
	writeJSON(w, appsettings.Settings{Root: s.Organizer.Root()})
}

func (s *Server) listDirectories(w http.ResponseWriter, r *http.Request) {
	base := s.browseBase()
	requested := strings.TrimSpace(r.URL.Query().Get("path"))
	if requested == "" {
		requested = base
	}
	requested = filepath.Clean(requested)
	if !filepath.IsAbs(requested) || !withinPath(base, requested) {
		http.Error(w, "path is outside the allowed share root", http.StatusBadRequest)
		return
	}
	st, err := os.Stat(requested)
	if err != nil {
		http.Error(w, "directory is not accessible", http.StatusBadRequest)
		return
	}
	if !st.IsDir() {
		http.Error(w, "path is not a directory", http.StatusBadRequest)
		return
	}
	entries, err := os.ReadDir(requested)
	if err != nil {
		http.Error(w, "directory cannot be read", http.StatusForbidden)
		return
	}
	out := directoryResponse{Path: requested, Entries: make([]directoryEntry, 0)}
	if requested != base {
		parent := filepath.Dir(requested)
		if withinPath(base, parent) {
			out.Parent = parent
		}
	}
	for _, entry := range entries {
		if len(out.Entries) >= 500 {
			break
		}
		full := filepath.Join(requested, entry.Name())
		info, err := os.Stat(full)
		if err != nil || !info.IsDir() {
			continue
		}
		out.Entries = append(out.Entries, directoryEntry{Name: entry.Name(), Path: full})
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		return strings.ToLower(out.Entries[i].Name) < strings.ToLower(out.Entries[j].Name)
	})
	writeJSON(w, out)
}

func (s *Server) browseBase() string {
	base := strings.TrimSpace(s.BrowseRoot)
	if base == "" {
		base = "/share"
	}
	if abs, err := filepath.Abs(base); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(base)
}

func withinPath(base, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(target))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
