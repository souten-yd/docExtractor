package web

import (
	"net/http"
	"sync"
	"time"

	"github.com/souten-yd/docExtractor/internal/organizer"
)

type archiveScanState struct {
	mu sync.RWMutex
	Running bool `json:"running"`
	Phase string `json:"phase"`
	Completed int `json:"completed"`
	Total int `json:"total"`
	Current string `json:"current,omitempty"`
	Message string `json:"message,omitempty"`
	Error string `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	ElapsedMS int64 `json:"elapsed_ms"`
	ItemsPerSecond float64 `json:"items_per_second"`
	Plans []organizer.Plan `json:"-"`
}

var archiveScanStates sync.Map // *Server -> *archiveScanState

func (s *Server) archiveScanState() *archiveScanState {
	if v, ok := archiveScanStates.Load(s); ok { return v.(*archiveScanState) }
	st := &archiveScanState{Phase:"idle"}
	actual, _ := archiveScanStates.LoadOrStore(s, st)
	return actual.(*archiveScanState)
}

func (s *Server) startArchiveScan() *archiveScanState {
	st := s.archiveScanState()
	st.mu.Lock()
	if st.Running { st.mu.Unlock(); return st }
	st.Running = true; st.Phase = "starting"; st.Completed = 0; st.Total = 0; st.Current = ""; st.Message = "スキャンを開始しています"; st.Error = ""; st.StartedAt = time.Now(); st.FinishedAt = time.Time{}; st.ElapsedMS = 0; st.ItemsPerSecond = 0
	st.mu.Unlock()
	go func(){
		plans, err := s.Organizer.ScanWithProgress(func(p organizer.ScanProgress){
			st.mu.Lock(); st.Phase=p.Phase; st.Completed=p.Completed; st.Total=p.Total; st.Current=p.Current; st.Message=p.Message
			elapsed:=time.Since(st.StartedAt); st.ElapsedMS=elapsed.Milliseconds(); if elapsed > 0 && p.Completed > 0 { st.ItemsPerSecond=float64(p.Completed)/elapsed.Seconds() }; st.mu.Unlock()
		})
		st.mu.Lock(); defer st.mu.Unlock()
		st.Running=false; st.FinishedAt=time.Now(); st.ElapsedMS=time.Since(st.StartedAt).Milliseconds(); st.Current=""
		if err != nil { st.Phase="failed"; st.Error=err.Error(); st.Message="スキャンに失敗しました"; return }
		st.Plans=plans; st.Phase="done"; st.Completed=len(plans); st.Total=len(plans); st.Message="解析完了"
	}()
	return st
}

func archiveScanStatusSnapshot(st *archiveScanState) map[string]any {
	st.mu.RLock(); defer st.mu.RUnlock()
	elapsed:=st.ElapsedMS; if st.Running && !st.StartedAt.IsZero() { elapsed=time.Since(st.StartedAt).Milliseconds() }
	return map[string]any{"running":st.Running,"phase":st.Phase,"completed":st.Completed,"total":st.Total,"current":st.Current,"message":st.Message,"error":st.Error,"elapsed_ms":elapsed,"items_per_second":st.ItemsPerSecond,"plans":len(st.Plans)}
}

func archiveScanPlansSnapshot(st *archiveScanState) []organizer.Plan {
	st.mu.RLock(); defer st.mu.RUnlock(); out:=make([]organizer.Plan,len(st.Plans)); copy(out,st.Plans); return out
}

// handleArchiveScan keeps POST /api/scan backward compatible: a plain request
// starts (or resumes) the background scan and immediately returns the last
// completed result set. New UI uses op=start/status/items for progress polling.
func (s *Server) handleArchiveScan(w http.ResponseWriter, r *http.Request) {
	op:=r.URL.Query().Get("op")
	st:=s.archiveScanState()
	switch op {
	case "status": writeJSON(w,archiveScanStatusSnapshot(st)); return
	case "items": writeJSON(w,archiveScanPlansSnapshot(st)); return
	case "start": s.startArchiveScan(); writeJSON(w,archiveScanStatusSnapshot(st)); return
	default:
		s.startArchiveScan()
		writeJSON(w,archiveScanPlansSnapshot(st))
	}
}
