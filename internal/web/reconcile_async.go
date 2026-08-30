package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/souten-yd/docExtractor/internal/organizer"
)

type asyncReconcileRequest struct {
	Mode              string            `json:"mode"`
	Root              string            `json:"root,omitempty"`
	Roots             []string          `json:"roots,omitempty"`
	OutputRoot        string            `json:"output_root,omitempty"`
	Selections        map[string]string `json:"selections,omitempty"`
	IncludeQuarantine bool              `json:"include_quarantine,omitempty"`
}

type asyncReconcileState struct {
	mu                sync.RWMutex
	ID                string
	Mode              string
	Running           bool
	Kind              string
	StartedAt         time.Time
	UpdatedAt         time.Time
	Progress          organizer.ReconcileProgress
	Summary           organizer.ReconcileSummary
	Result            organizer.ReconcileResult
	Err               string
	SnapshotDir       string
	ItemsTotal        int
	ChoicesTotal      int
	IncludeQuarantine bool
	Roots             []string
	OutputRoot        string
}

type asyncReconcileManager struct {
	mu     sync.Mutex
	states map[string]*asyncReconcileState
}

var largeReconcile = &asyncReconcileManager{states: map[string]*asyncReconcileState{}}

func (m *asyncReconcileManager) state(mode string) *asyncReconcileState {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.states[mode]
	if s == nil {
		s = &asyncReconcileState{Mode: mode}
		m.states[mode] = s
	}
	return s
}

func validAsyncMode(v string) string {
	if v == "reprocess" { return v }
	return "manage"
}

func (s *Server) validateAsyncRequest(req *asyncReconcileRequest) error {
	req.Mode = validAsyncMode(req.Mode)
	if len(req.Roots) == 0 && strings.TrimSpace(req.Root) != "" { req.Roots = []string{req.Root} }
	if len(req.Roots) == 0 || len(req.Roots) > 32 { return fmt.Errorf("select 1 to 32 library roots") }
	clean := make([]string, 0, len(req.Roots))
	seen := map[string]struct{}{}
	for _, raw := range req.Roots {
		root := filepath.Clean(strings.TrimSpace(raw))
		if root == "." || !filepath.IsAbs(root) || !withinPath(s.browseBase(), root) { return fmt.Errorf("all roots must be absolute paths inside the allowed share root") }
		if _, ok := seen[root]; !ok { seen[root] = struct{}{}; clean = append(clean, root) }
	}
	req.Roots = clean
	if strings.TrimSpace(req.OutputRoot) == "" { req.OutputRoot = req.Roots[0] }
	req.OutputRoot = filepath.Clean(strings.TrimSpace(req.OutputRoot))
	if req.OutputRoot == "." || !filepath.IsAbs(req.OutputRoot) || !withinPath(s.browseBase(), req.OutputRoot) { return fmt.Errorf("output_root must be inside the allowed share root") }
	return nil
}

func markReconcileCancelled(state *asyncReconcileState) {
	state.mu.Lock()
	state.Running = false
	state.UpdatedAt = time.Now()
	state.Err = ""
	state.Progress.Phase = "cancelled"
	state.Progress.Message = "ユーザー操作により停止しました"
	state.mu.Unlock()
	releaseProcessMemory()
}

func recoverReconcileCancellation(state *asyncReconcileState) {
	if v := recover(); v != nil {
		if _, ok := v.(reconcileCancelledPanic); ok { markReconcileCancelled(state); return }
		panic(v)
	}
}

func (s *Server) reconcileAsyncStart(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
	var req asyncReconcileRequest
	dec := json.NewDecoder(r.Body); dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil { http.Error(w, "invalid request", http.StatusBadRequest); return }
	if err := s.validateAsyncRequest(&req); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
	state := largeReconcile.state(req.Mode)
	state.mu.Lock()
	if state.Running { state.mu.Unlock(); http.Error(w, "analysis already running", http.StatusConflict); return }
	reconcileStops.clear(req.Mode)
	id := fmt.Sprintf("%s-%d", req.Mode, time.Now().UnixNano())
	state.ID = id; state.Running = true; state.Kind = "scan"; state.StartedAt = time.Now(); state.UpdatedAt = state.StartedAt
	state.Progress = organizer.ReconcileProgress{Phase: "starting"}; state.Summary = organizer.ReconcileSummary{}; state.Result = organizer.ReconcileResult{}; state.Err = ""
	state.ItemsTotal = 0; state.ChoicesTotal = 0; state.SnapshotDir = ""; state.IncludeQuarantine = req.IncludeQuarantine
	state.Roots = append([]string(nil), req.Roots...); state.OutputRoot = req.OutputRoot
	state.mu.Unlock()

	go func() {
		defer recoverReconcileCancellation(state)
		opts := organizer.ReconcileScanOptions{IncludeQuarantine: req.IncludeQuarantine}
		report, err := s.Organizer.ReconcileScanMultiProgressWithOptions(req.Roots, req.OutputRoot, opts, func(p organizer.ReconcileProgress) {
			checkReconcileCancelled(req.Mode)
			state.mu.Lock(); state.Progress = p; state.UpdatedAt = time.Now(); state.mu.Unlock()
		})
		checkReconcileCancelled(req.Mode)
		if err != nil {
			state.mu.Lock(); state.Running = false; state.UpdatedAt = time.Now(); state.Err = err.Error(); state.Progress.Phase = "failed"; state.mu.Unlock(); releaseProcessMemory(); return
		}
		state.mu.Lock(); state.Progress = organizer.ReconcileProgress{Phase: "saving", Completed: 0, Total: 1, Message: "解析結果をディスクへ保存中"}; state.UpdatedAt = time.Now(); state.mu.Unlock()
		dir, meta, err := saveReconcileSnapshot(report.OutputRoot, req.Mode, report)
		report = organizer.ReconcileReport{}
		checkReconcileCancelled(req.Mode)
		if err != nil {
			state.mu.Lock(); state.Running = false; state.UpdatedAt = time.Now(); state.Err = "snapshot failed: "+err.Error(); state.Progress.Phase = "failed"; state.mu.Unlock(); releaseProcessMemory(); return
		}
		state.mu.Lock()
		state.Running = false; state.UpdatedAt = time.Now(); state.SnapshotDir = dir; state.Summary = meta.Report.Summary; state.ItemsTotal = meta.ItemsTotal; state.ChoicesTotal = meta.ChoicesTotal
		msg := "解析完了・結果をディスクへ退避済み"; if req.IncludeQuarantine { msg = "隔離フォルダを含む解析完了・結果をディスクへ退避済み" }
		state.Progress = organizer.ReconcileProgress{Phase: "done", Completed: meta.Report.Summary.Files, Total: meta.Report.Summary.Files, Message: msg}
		state.mu.Unlock(); releaseProcessMemory()
	}()

	w.Header().Set("Content-Type", "application/json; charset=utf-8"); w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]any{"id": id, "mode": req.Mode, "include_quarantine": req.IncludeQuarantine})
}

func overallReconcileProgress(kind, phase string, completed, total int) float64 {
	frac := 0.0
	if total > 0 { frac = float64(completed)/float64(total); if frac < 0 { frac=0 }; if frac > 1 { frac=1 } }
	if kind == "execute" {
		switch phase { case "starting": return 0.02; case "executing": return 0.05+0.95*frac; case "done": return 1; case "cancelled", "failed": return 0 }
	}
	switch phase {
	case "starting": return 0.01
	case "counting": return 0.03
	case "inspecting": return 0.05+0.40*frac
	case "clustering":
		// Older scans reported completed==total when entering this phase. Do not
		// let that masquerade as 100%; the expensive series merge is still active.
		if completed >= total && total > 0 { frac = 0 }
		return 0.45+0.30*frac
	case "duplicates": return 0.75+0.20*frac
	case "variants": return 0.95+0.02*frac
	case "saving": return 0.98+0.02*frac
	case "done": return 1
	case "cancelled", "failed": return 0
	}
	return 0
}

func (s *Server) reconcileAsyncStatus(w http.ResponseWriter, r *http.Request) {
	mode := validAsyncMode(r.URL.Query().Get("mode")); state := largeReconcile.state(mode)
	state.mu.RLock(); defer state.mu.RUnlock()
	now := time.Now(); elapsed := int64(0); if !state.StartedAt.IsZero() { elapsed = now.Sub(state.StartedAt).Milliseconds() }
	p := state.Progress
	overall := overallReconcileProgress(state.Kind, p.Phase, p.Completed, p.Total)
	eta := int64(0); rate := float64(0)
	if p.Completed > 0 && elapsed > 0 { rate = float64(p.Completed)/(float64(elapsed)/1000) }
	if state.Running && overall > 0.02 && overall < 1 && elapsed > 0 { eta = int64(float64(elapsed)*(1-overall)/overall) }
	writeJSON(w, map[string]any{
		"id": state.ID, "mode": mode, "running": state.Running, "kind": state.Kind,
		"phase": p.Phase, "completed": p.Completed, "total": p.Total, "message": p.Message,
		"overall_progress": overall, "elapsed_ms": elapsed, "estimated_remaining_ms": eta, "items_per_second": rate,
		"error": state.Err, "summary": state.Summary, "items": state.ItemsTotal, "choices": state.ChoicesTotal,
		"include_quarantine": state.IncludeQuarantine, "started_at": state.StartedAt, "updated_at": state.UpdatedAt,
		"roots": state.Roots, "output_root": state.OutputRoot, "stop_requested": reconcileStops.isRequested(mode),
	})
}

func pageBounds(r *http.Request, total int) (int, int) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset")); limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if offset < 0 { offset=0 }; if limit < 1 { limit=100 }; if limit > 500 { limit=500 }; if offset > total { offset=total }
	end := offset+limit; if end > total { end=total }; return offset,end
}

func (s *Server) reconcileAsyncItems(w http.ResponseWriter, r *http.Request) {
	mode := validAsyncMode(r.URL.Query().Get("mode")); state := largeReconcile.state(mode); state.mu.RLock(); dir,total:=state.SnapshotDir,state.ItemsTotal; state.mu.RUnlock()
	start,end:=pageBounds(r,total); if dir=="" { writeJSON(w,map[string]any{"offset":start,"limit":0,"total":total,"items":[]organizer.ReconcileItem{}}); return }
	items,err:=loadSnapshotPage[organizer.ReconcileItem](dir,"items",start,end-start,total); if err!=nil { http.Error(w,"result page unavailable: "+err.Error(),http.StatusInternalServerError); return }
	writeJSON(w,map[string]any{"offset":start,"limit":len(items),"total":total,"items":items})
}

func (s *Server) reconcileAsyncChoices(w http.ResponseWriter, r *http.Request) {
	mode:=validAsyncMode(r.URL.Query().Get("mode")); state:=largeReconcile.state(mode); state.mu.RLock(); dir,total:=state.SnapshotDir,state.ChoicesTotal; state.mu.RUnlock()
	start,end:=pageBounds(r,total); if dir=="" { writeJSON(w,map[string]any{"offset":start,"limit":0,"total":total,"choices":[]organizer.ReconcileChoice{}}); return }
	items,err:=loadSnapshotPage[organizer.ReconcileChoice](dir,"choices",start,end-start,total); if err!=nil { http.Error(w,"choice page unavailable: "+err.Error(),http.StatusInternalServerError); return }
	writeJSON(w,map[string]any{"offset":start,"limit":len(items),"total":total,"choices":items})
}

func (s *Server) reconcileAsyncExecute(w http.ResponseWriter, r *http.Request) {
	if summary:=s.Jobs.Summary(); summary.Running>0||summary.Queued>0 { http.Error(w,"archive jobs are running or queued",http.StatusConflict); return }
	r.Body=http.MaxBytesReader(w,r.Body,16*1024*1024); var req asyncReconcileRequest; dec:=json.NewDecoder(r.Body); dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil { http.Error(w,"invalid request",http.StatusBadRequest); return }
	req.Mode=validAsyncMode(req.Mode); state:=largeReconcile.state(req.Mode); state.mu.Lock()
	if state.Running { state.mu.Unlock(); http.Error(w,"operation already running",http.StatusConflict); return }
	if state.SnapshotDir=="" { state.mu.Unlock(); http.Error(w,"run analysis first",http.StatusConflict); return }
	snapshotDir:=state.SnapshotDir; reconcileStops.clear(req.Mode)
	state.Running=true; state.Kind="execute"; state.StartedAt=time.Now(); state.UpdatedAt=state.StartedAt; state.Progress=organizer.ReconcileProgress{Phase:"starting",Message:"解析結果を読み込み中"}; state.Result=organizer.ReconcileResult{}; state.Err=""
	id:=fmt.Sprintf("%s-exec-%d",req.Mode,time.Now().UnixNano()); state.ID=id; state.mu.Unlock()

	go func(){
		defer recoverReconcileCancellation(state)
		checkReconcileCancelled(req.Mode)
		report,err:=loadReconcileSnapshot(snapshotDir)
		if err==nil {
			var result organizer.ReconcileResult
			result,err=s.Organizer.ReconcileExecuteReportProgress(report,req.Selections,func(p organizer.ReconcileProgress){ checkReconcileCancelled(req.Mode); state.mu.Lock(); state.Progress=p; state.UpdatedAt=time.Now(); state.mu.Unlock() })
			report=organizer.ReconcileReport{}
			if err==nil { state.mu.Lock(); state.Result=result; state.mu.Unlock() }
		}
		checkReconcileCancelled(req.Mode)
		state.mu.Lock(); state.Running=false; state.UpdatedAt=time.Now()
		if err!=nil { state.Err=err.Error(); state.Progress.Phase="failed" } else { state.Progress.Phase="done"; state.Progress.Message="整理完了・RAM解放済み" }
		state.mu.Unlock(); releaseProcessMemory()
	}()
	w.Header().Set("Content-Type","application/json; charset=utf-8"); w.WriteHeader(http.StatusAccepted); writeJSON(w,map[string]any{"id":id,"mode":req.Mode})
}

func (s *Server) reconcileAsyncResult(w http.ResponseWriter, r *http.Request) {
	mode:=validAsyncMode(r.URL.Query().Get("mode")); state:=largeReconcile.state(mode); state.mu.RLock(); defer state.mu.RUnlock(); writeJSON(w,map[string]any{"running":state.Running,"error":state.Err,"result":state.Result})
}
