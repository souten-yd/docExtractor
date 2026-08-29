package web

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/souten-yd/docExtractor/internal/diagnostics"
	"github.com/souten-yd/docExtractor/internal/jobs"
	"github.com/souten-yd/docExtractor/internal/organizer"
	appsettings "github.com/souten-yd/docExtractor/internal/settings"
	"github.com/souten-yd/docExtractor/internal/updater"
)

//go:embed static/index.html
var indexHTML string

type Server struct {
	Organizer   *organizer.Organizer
	Jobs        *jobs.Manager
	Diagnostics *diagnostics.Manager
	Settings    *appsettings.Store
	Updater     *updater.Manager
	BrowseRoot  string
	Version     string
}

type submitRequest struct {
	Names       []string `json:"names"`
	AllowReview bool     `json:"allow_review"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.index)
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.updateSettings)
	mux.HandleFunc("GET /api/directories", s.listDirectories)
	mux.HandleFunc("POST /api/scan", s.scan)
	mux.HandleFunc("GET /api/jobs", s.listJobs)
	mux.HandleFunc("GET /api/jobs/{jobID}", s.getJob)
	mux.HandleFunc("POST /api/jobs", s.submitJobs)
	mux.HandleFunc("POST /api/jobs/{jobID}/cancel", s.cancelJob)
	s.registerUpdateRoutes(mux)
	DiagnosticsHandler{
		Manager: s.Diagnostics, Version: s.Version,
		ArchiveRoot: s.Organizer.Root, Workers: s.Jobs.Workers(),
	}.Register(mux)

	// QTS exposes the app through QPKG_PROXY_PATH=/docExtractor. Some QTS
	// versions strip the prefix before proxying and some integrations may not,
	// so accept both forms without changing the API implementation.
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/docExtractor" {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			mux.ServeHTTP(w, r2)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/docExtractor/") {
			http.StripPrefix("/docExtractor", mux).ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
	return securityHeaders(router)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	status := map[string]any{
		"version": s.Version,
		"root":    s.Organizer.Root(),
		"workers": s.Jobs.Workers(),
		"jobs":    s.Jobs.Summary(),
		"cpus":    runtime.NumCPU(),
	}
	for k, v := range lightweightSystemMetrics() {
		status[k] = v
	}
	writeJSON(w, status)
}

func lightweightSystemMetrics() map[string]any {
	out := map[string]any{}
	if raw, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) >= 3 {
			if v, err := strconv.ParseFloat(fields[0], 64); err == nil {
				out["load_1m"] = v
			}
			if v, err := strconv.ParseFloat(fields[1], 64); err == nil {
				out["load_5m"] = v
			}
		}
	}
	if raw, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.TrimSuffix(fields[0], ":") == "MemAvailable" {
				if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					out["mem_available_bytes"] = kb * 1024
				}
				break
			}
		}
	}
	return out
}

func (s *Server) scan(w http.ResponseWriter, r *http.Request) {
	plans, err := s.Organizer.Scan()
	if err != nil {
		http.Error(w, "scan failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, plans)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Jobs.List()) }

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	j, ok := s.Jobs.Get(r.PathValue("jobID"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, j)
}

func (s *Server) submitJobs(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var req submitRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if len(req.Names) == 0 || len(req.Names) > 500 {
		http.Error(w, "select 1 to 500 files", http.StatusBadRequest)
		return
	}
	submitted := make([]jobs.Job, 0, len(req.Names))
	for _, name := range req.Names {
		name = strings.TrimSpace(name)
		plan, err := s.Organizer.PlanName(name)
		if err != nil || plan.Error != "" || (plan.NeedsReview && !req.AllowReview) {
			continue
		}
		j, err := s.Jobs.Submit(jobs.Task{Source: plan.Source, Destination: plan.Destination, DeleteSource: true})
		if err != nil {
			if strings.Contains(err.Error(), "queue is full") {
				break
			}
			continue
		}
		submitted = append(submitted, j)
	}
	if len(submitted) == 0 {
		http.Error(w, "no executable files were submitted", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(submitted)
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	if !s.Jobs.Cancel(r.PathValue("jobID")) {
		http.Error(w, "job cannot be cancelled", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
