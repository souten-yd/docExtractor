package web

import (
	"net/http"
	"sync"
)

type reconcileStopRegistry struct {
	mu sync.RWMutex
	requested map[string]bool
}

var reconcileStops = &reconcileStopRegistry{requested: map[string]bool{}}

func (r *reconcileStopRegistry) clear(mode string) {
	r.mu.Lock()
	delete(r.requested, validAsyncMode(mode))
	r.mu.Unlock()
}

func (r *reconcileStopRegistry) request(mode string) {
	r.mu.Lock()
	r.requested[validAsyncMode(mode)] = true
	r.mu.Unlock()
}

func (r *reconcileStopRegistry) isRequested(mode string) bool {
	r.mu.RLock()
	v := r.requested[validAsyncMode(mode)]
	r.mu.RUnlock()
	return v
}

type reconcileCancelledPanic struct{}

func checkReconcileCancelled(mode string) {
	if reconcileStops.isRequested(mode) {
		panic(reconcileCancelledPanic{})
	}
}

func (s *Server) reconcileAsyncStop(w http.ResponseWriter, r *http.Request) {
	mode := validAsyncMode(r.URL.Query().Get("mode"))
	state := largeReconcile.state(mode)
	state.mu.RLock()
	running := state.Running
	state.mu.RUnlock()
	if !running {
		http.Error(w, "operation is not running", http.StatusConflict)
		return
	}
	reconcileStops.request(mode)
	writeJSON(w, map[string]any{"mode": mode, "stop_requested": true})
}
