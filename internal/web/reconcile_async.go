package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/souten-yd/docExtractor/internal/organizer"
)

type asyncReconcileRequest struct {
	Mode string `json:"mode"`
	Root string `json:"root,omitempty"`
	Roots []string `json:"roots,omitempty"`
	OutputRoot string `json:"output_root,omitempty"`
	Selections map[string]string `json:"selections,omitempty"`
}

type asyncReconcileState struct {
	mu sync.RWMutex
	ID string
	Mode string
	Running bool
	Kind string
	StartedAt time.Time
	UpdatedAt time.Time
	Progress organizer.ReconcileProgress
	Report organizer.ReconcileReport
	Result organizer.ReconcileResult
	Err string
}

type asyncReconcileManager struct{ mu sync.Mutex; states map[string]*asyncReconcileState }
var largeReconcile = &asyncReconcileManager{states:map[string]*asyncReconcileState{}}

func (m *asyncReconcileManager) state(mode string)*asyncReconcileState{m.mu.Lock();defer m.mu.Unlock();s:=m.states[mode];if s==nil{s=&asyncReconcileState{Mode:mode};m.states[mode]=s};return s}
func validAsyncMode(v string)string{if v=="reprocess"{return v};return "manage"}

func (s *Server) reconcileAsyncStart(w http.ResponseWriter,r *http.Request){
	r.Body=http.MaxBytesReader(w,r.Body,64*1024);var req asyncReconcileRequest;if err:=json.NewDecoder(r.Body).Decode(&req);err!=nil{http.Error(w,"invalid request",http.StatusBadRequest);return};req.Mode=validAsyncMode(req.Mode)
	base:=reconcileRequest{Root:req.Root,Roots:req.Roots,OutputRoot:req.OutputRoot};raw,_:=json.Marshal(base);r2:=r.Clone(r.Context());r2.Body=http.NoBody;_ = raw
	if len(req.Roots)==0&&req.Root!=""{req.Roots=[]string{req.Root}};if len(req.Roots)==0{http.Error(w,"select at least one library root",http.StatusBadRequest);return};if req.OutputRoot==""{req.OutputRoot=req.Roots[0]}
	state:=largeReconcile.state(req.Mode);state.mu.Lock();if state.Running{state.mu.Unlock();http.Error(w,"analysis already running",http.StatusConflict);return};id:=fmt.Sprintf("%s-%d",req.Mode,time.Now().UnixNano());state.ID=id;state.Running=true;state.Kind="scan";state.StartedAt=time.Now();state.UpdatedAt=state.StartedAt;state.Progress=organizer.ReconcileProgress{Phase:"starting"};state.Report=organizer.ReconcileReport{};state.Result=organizer.ReconcileResult{};state.Err="";state.mu.Unlock()
	go func(){report,err:=s.Organizer.ReconcileScanMultiProgress(req.Roots,req.OutputRoot,func(p organizer.ReconcileProgress){state.mu.Lock();state.Progress=p;state.UpdatedAt=time.Now();state.mu.Unlock()});state.mu.Lock();defer state.mu.Unlock();state.Running=false;state.UpdatedAt=time.Now();if err!=nil{state.Err=err.Error();state.Progress.Phase="failed";return};state.Report=report;state.Progress=organizer.ReconcileProgress{Phase:"done",Completed:report.Summary.Files,Total:report.Summary.Files,Message:"解析完了"}}()
	w.Header().Set("Content-Type","application/json; charset=utf-8");w.WriteHeader(http.StatusAccepted);writeJSON(w,map[string]any{"id":id,"mode":req.Mode})
}

func (s *Server) reconcileAsyncStatus(w http.ResponseWriter,r *http.Request){mode:=validAsyncMode(r.URL.Query().Get("mode"));state:=largeReconcile.state(mode);state.mu.RLock();defer state.mu.RUnlock();now:=time.Now();elapsed:=int64(0);if !state.StartedAt.IsZero(){elapsed=now.Sub(state.StartedAt).Milliseconds()};p:=state.Progress;eta:=int64(0);rate:=float64(0);if p.Completed>0&&elapsed>0{rate=float64(p.Completed)/(float64(elapsed)/1000);if p.Total>p.Completed&&rate>0{eta=int64(float64(p.Total-p.Completed)/rate*1000)}};summary:=state.Report.Summary;writeJSON(w,map[string]any{"id":state.ID,"mode":mode,"running":state.Running,"kind":state.Kind,"phase":p.Phase,"completed":p.Completed,"total":p.Total,"message":p.Message,"elapsed_ms":elapsed,"estimated_remaining_ms":eta,"items_per_second":rate,"error":state.Err,"summary":summary,"items":len(state.Report.Items),"choices":len(state.Report.Choices),"started_at":state.StartedAt,"updated_at":state.UpdatedAt})}

func pageBounds(r *http.Request,total int)(int,int){offset,_:=strconv.Atoi(r.URL.Query().Get("offset"));limit,_:=strconv.Atoi(r.URL.Query().Get("limit"));if offset<0{offset=0};if limit<1{limit=100};if limit>500{limit=500};if offset>total{offset=total};end:=offset+limit;if end>total{end=total};return offset,end}
func (s *Server) reconcileAsyncItems(w http.ResponseWriter,r *http.Request){mode:=validAsyncMode(r.URL.Query().Get("mode"));state:=largeReconcile.state(mode);state.mu.RLock();defer state.mu.RUnlock();start,end:=pageBounds(r,len(state.Report.Items));items:=append([]organizer.ReconcileItem(nil),state.Report.Items[start:end]...);writeJSON(w,map[string]any{"offset":start,"limit":end-start,"total":len(state.Report.Items),"items":items})}
func (s *Server) reconcileAsyncChoices(w http.ResponseWriter,r *http.Request){mode:=validAsyncMode(r.URL.Query().Get("mode"));state:=largeReconcile.state(mode);state.mu.RLock();defer state.mu.RUnlock();start,end:=pageBounds(r,len(state.Report.Choices));items:=append([]organizer.ReconcileChoice(nil),state.Report.Choices[start:end]...);writeJSON(w,map[string]any{"offset":start,"limit":end-start,"total":len(state.Report.Choices),"choices":items})}

func (s *Server) reconcileAsyncExecute(w http.ResponseWriter,r *http.Request){
	if summary:=s.Jobs.Summary();summary.Running>0||summary.Queued>0{http.Error(w,"archive jobs are running or queued",http.StatusConflict);return}
	r.Body=http.MaxBytesReader(w,r.Body,256*1024);var req asyncReconcileRequest;if err:=json.NewDecoder(r.Body).Decode(&req);err!=nil{http.Error(w,"invalid request",http.StatusBadRequest);return};req.Mode=validAsyncMode(req.Mode);state:=largeReconcile.state(req.Mode);state.mu.Lock();if state.Running{state.mu.Unlock();http.Error(w,"operation already running",http.StatusConflict);return};if len(state.Report.Roots)==0{state.mu.Unlock();http.Error(w,"run analysis first",http.StatusConflict);return};report:=state.Report;state.Running=true;state.Kind="execute";state.StartedAt=time.Now();state.UpdatedAt=state.StartedAt;state.Progress=organizer.ReconcileProgress{Phase:"starting"};state.Result=organizer.ReconcileResult{};state.Err="";id:=fmt.Sprintf("%s-exec-%d",req.Mode,time.Now().UnixNano());state.ID=id;state.mu.Unlock()
	go func(){result,err:=s.Organizer.ReconcileExecuteReportProgress(report,req.Selections,func(p organizer.ReconcileProgress){state.mu.Lock();state.Progress=p;state.UpdatedAt=time.Now();state.mu.Unlock()});state.mu.Lock();defer state.mu.Unlock();state.Running=false;state.UpdatedAt=time.Now();if err!=nil{state.Err=err.Error();state.Progress.Phase="failed";return};state.Result=result;state.Progress.Phase="done";state.Progress.Message="整理完了"}()
	w.Header().Set("Content-Type","application/json; charset=utf-8");w.WriteHeader(http.StatusAccepted);writeJSON(w,map[string]any{"id":id,"mode":req.Mode})
}

func (s *Server) reconcileAsyncResult(w http.ResponseWriter,r *http.Request){mode:=validAsyncMode(r.URL.Query().Get("mode"));state:=largeReconcile.state(mode);state.mu.RLock();defer state.mu.RUnlock();writeJSON(w,map[string]any{"running":state.Running,"error":state.Err,"result":state.Result})}
