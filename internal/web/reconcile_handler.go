package web

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
)

type reconcileRequest struct {
	Root       string            `json:"root,omitempty"`
	Roots      []string          `json:"roots,omitempty"`
	OutputRoot string            `json:"output_root,omitempty"`
	Selections map[string]string `json:"selections,omitempty"`
}

func (s *Server) reconcileScan(w http.ResponseWriter,r *http.Request){
	req,ok:=s.validReconcileRequest(w,r);if !ok{return}
	report,err:=s.Organizer.ReconcileScanMulti(req.Roots,req.OutputRoot);if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return};writeJSON(w,report)
}
func (s *Server) reconcileExecute(w http.ResponseWriter,r *http.Request){
	if summary:=s.Jobs.Summary();summary.Running>0||summary.Queued>0{http.Error(w,"archive jobs are running or queued",http.StatusConflict);return}
	req,ok:=s.validReconcileRequest(w,r);if !ok{return}
	result,err:=s.Organizer.ReconcileExecuteMulti(req.Roots,req.OutputRoot,req.Selections);if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return};writeJSON(w,result)
}
func (s *Server) validReconcileRequest(w http.ResponseWriter,r *http.Request)(reconcileRequest,bool){
	r.Body=http.MaxBytesReader(w,r.Body,64*1024);var req reconcileRequest;dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields();if err:=dec.Decode(&req);err!=nil{http.Error(w,"invalid request",http.StatusBadRequest);return req,false}
	if len(req.Roots)==0&&strings.TrimSpace(req.Root)!=""{req.Roots=[]string{req.Root}}
	if len(req.Roots)==0||len(req.Roots)>32{http.Error(w,"select 1 to 32 library roots",http.StatusBadRequest);return req,false}
	clean:=make([]string,0,len(req.Roots));seen:=map[string]struct{}{}
	for _,raw:=range req.Roots{root:=filepath.Clean(strings.TrimSpace(raw));if root=="."||!filepath.IsAbs(root)||!withinPath(s.browseBase(),root){http.Error(w,"all roots must be absolute paths inside the allowed share root",http.StatusBadRequest);return req,false};if _,ok:=seen[root];!ok{seen[root]=struct{}{};clean=append(clean,root)}}
	req.Roots=clean
	if strings.TrimSpace(req.OutputRoot)==""{req.OutputRoot=req.Roots[0]}
	req.OutputRoot=filepath.Clean(strings.TrimSpace(req.OutputRoot));if req.OutputRoot=="."||!filepath.IsAbs(req.OutputRoot)||!withinPath(s.browseBase(),req.OutputRoot){http.Error(w,"output_root must be inside the allowed share root",http.StatusBadRequest);return req,false}
	if _,ok:=seen[req.OutputRoot];!ok{req.Roots=append(req.Roots,req.OutputRoot);seen[req.OutputRoot]=struct{}{}}
	for id,path:=range req.Selections{path=filepath.Clean(strings.TrimSpace(path));if id==""||path=="."||!filepath.IsAbs(path){delete(req.Selections,id);continue};allowed:=false;for _,root:=range req.Roots{if withinPath(root,path){allowed=true;break}};if !allowed{http.Error(w,"selection source must be inside a selected library root",http.StatusBadRequest);return req,false};req.Selections[id]=path}
	return req,true
}
