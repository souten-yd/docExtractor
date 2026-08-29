package web

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) registerUpdateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/update", s.updateStatus)
	mux.HandleFunc("POST /api/update/check", s.checkUpdate)
	mux.HandleFunc("POST /api/update/install", s.installUpdate)
	mux.HandleFunc("GET /api/update/log", s.updateLog)
}

func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.Updater == nil {
		http.Error(w, "updater unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.Updater.Status())
}

func (s *Server) checkUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.Updater == nil {
		http.Error(w, "updater unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	status, _ := s.Updater.Check(ctx)
	// Update check failures are returned as structured status so the Web UI can
	// show a useful message instead of replacing it with a generic HTTP error.
	writeJSON(w, status)
}

func (s *Server) installUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.Updater == nil {
		http.Error(w, "updater unavailable", http.StatusServiceUnavailable)
		return
	}
	// A custom same-origin header makes accidental/cross-site form submission
	// insufficient to trigger a NAS package installation.
	if r.Header.Get("X-docExtractor-Confirm") != "update" {
		http.Error(w, "update confirmation header is required", http.StatusBadRequest)
		return
	}
	summary := s.Jobs.Summary()
	if summary.Running > 0 || summary.Queued > 0 {
		http.Error(w, "archive jobs are running or queued; wait for them to finish before updating", http.StatusConflict)
		return
	}
	if err := s.Updater.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	writeJSONBody(w, s.Updater.Status())
}

func (s *Server) updateLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.Updater == nil {
		http.Error(w, "updater unavailable", http.StatusServiceUnavailable)
		return
	}
	raw, err := s.Updater.InstallLog(256 * 1024)
	if err != nil {
		http.Error(w, "cannot read update log", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(raw)
}

func writeJSONBody(w http.ResponseWriter, v any) {
	// writeJSON sets headers as well, but installUpdate has already sent a 202.
	// Keep this helper small so the status body can still be emitted correctly.
	enc := jsonEncoder(w)
	_ = enc.Encode(v)
}
