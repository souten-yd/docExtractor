package web

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
)

type reconcileRequest struct { Root string `json:"root"` }

func (s *Server) reconcileScan(w http.ResponseWriter,r *http.Request){
	root,ok:=s.validReconcileRoot(w,r);if !ok{return}
	report,err:=s.Organizer.ReconcileScan(root);if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return};writeJSON(w,report)
}
func (s *Server) reconcileExecute(w http.ResponseWriter,r *http.Request){
	if summary:=s.Jobs.Summary();summary.Running>0||summary.Queued>0{http.Error(w,"archive jobs are running or queued",http.StatusConflict);return}
	root,ok:=s.validReconcileRoot(w,r);if !ok{return}
	result,err:=s.Organizer.ReconcileExecute(root);if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return};writeJSON(w,result)
}
func (s *Server) validReconcileRoot(w http.ResponseWriter,r *http.Request)(string,bool){
	r.Body=http.MaxBytesReader(w,r.Body,8*1024);var req reconcileRequest;dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields();if err:=dec.Decode(&req);err!=nil{http.Error(w,"invalid request",http.StatusBadRequest);return "",false}
	root:=filepath.Clean(strings.TrimSpace(req.Root));if root=="."||!filepath.IsAbs(root)||!withinPath(s.browseBase(),root){http.Error(w,"root must be an absolute path inside the allowed share root",http.StatusBadRequest);return "",false};return root,true
}
