package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/souten-yd/docExtractor/internal/diagnostics"
)

type DiagnosticsHandler struct {
	Manager *diagnostics.Manager
	Version string
}

func (h DiagnosticsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/logs/jobs", h.listJobs)
	mux.HandleFunc("GET /api/logs/jobs/{jobID}", h.getJobLog)
	mux.HandleFunc("GET /api/logs/jobs/{jobID}/download", h.downloadJobLog)
	mux.HandleFunc("GET /api/diagnostics/download", h.downloadBundle)
}

func (h DiagnosticsHandler) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.Manager.ListJobs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, jobs)
}

func (h DiagnosticsHandler) getJobLog(w http.ResponseWriter, r *http.Request) {
	events, err := h.Manager.Tail(r.PathValue("jobID"), 500)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, events)
}

func (h DiagnosticsHandler) downloadJobLog(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	f, err := h.Manager.OpenJobLog(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="docExtractor-job-%s.jsonl"`, jobID))
	http.ServeContent(w, r, "", time.Time{}, f)
}

func (h DiagnosticsHandler) downloadBundle(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	snapshot := map[string]any{
		"generated_at": time.Now().UTC(),
		"version":      h.Version,
		"go_version":   runtime.Version(),
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"cpus":         runtime.NumCPU(),
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="docExtractor-diagnostics.zip"`)
	if err := h.Manager.WriteBundle(w, jobID, snapshot); err != nil {
		// Headers may already be sent; keep body terse and avoid leaking paths.
		return
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
