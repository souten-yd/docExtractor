package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type quarantineFile struct {
	Root string `json:"root"`
	Path string `json:"path"`
	Relative string `json:"relative"`
	Size int64 `json:"size"`
	Modified time.Time `json:"modified"`
}

type quarantineListResponse struct {
	Files []quarantineFile `json:"files"`
	Count int `json:"count"`
	TotalBytes int64 `json:"total_bytes"`
}

type quarantineDeleteRequest struct {
	Roots []string `json:"roots"`
	Paths []string `json:"paths"`
	DeleteAll bool `json:"delete_all"`
	Confirm string `json:"confirm"`
}

type quarantineDeleteResult struct {
	Deleted int `json:"deleted"`
	FreedBytes int64 `json:"freed_bytes"`
	Errors []string `json:"errors,omitempty"`
}

func (s *Server) quarantineList(w http.ResponseWriter,r *http.Request){
	roots:=r.URL.Query()["root"]
	if len(roots)==0{http.Error(w,"at least one root is required",http.StatusBadRequest);return}
	if len(roots)>32{http.Error(w,"too many roots",http.StatusBadRequest);return}
	resp:=quarantineListResponse{}
	seen:=map[string]struct{}{}
	for _,raw:=range roots{
		root:=filepath.Clean(strings.TrimSpace(raw));if !s.validSharePath(root){http.Error(w,"invalid root",http.StatusBadRequest);return}
		qroot:=filepath.Join(root,".docExtractor-duplicates")
		_ = filepath.WalkDir(qroot,func(path string,d os.DirEntry,err error)error{
			if err!=nil{return nil};if d.IsDir(){return nil};st,e:=os.Lstat(path);if e!=nil||!st.Mode().IsRegular()||st.Mode()&os.ModeSymlink!=0{return nil}
			clean:=filepath.Clean(path);if !withinPath(qroot,clean){return nil};if _,ok:=seen[clean];ok{return nil};seen[clean]=struct{}{}
			rel,_:=filepath.Rel(qroot,clean);resp.Files=append(resp.Files,quarantineFile{Root:root,Path:clean,Relative:rel,Size:st.Size(),Modified:st.ModTime()});resp.Count++;resp.TotalBytes+=st.Size();return nil
		})
	}
	sort.Slice(resp.Files,func(i,j int)bool{if resp.Files[i].Modified.Equal(resp.Files[j].Modified){return resp.Files[i].Relative<resp.Files[j].Relative};return resp.Files[i].Modified.After(resp.Files[j].Modified)})
	writeJSON(w,resp)
}

func (s *Server) quarantineDelete(w http.ResponseWriter,r *http.Request){
	if summary:=s.Jobs.Summary();summary.Running>0||summary.Queued>0{http.Error(w,"archive jobs are running or queued",http.StatusConflict);return}
	r.Body=http.MaxBytesReader(w,r.Body,256*1024);var req quarantineDeleteRequest;dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields();if err:=dec.Decode(&req);err!=nil{http.Error(w,"invalid request",http.StatusBadRequest);return}
	if req.Confirm!="DELETE QUARANTINED"{http.Error(w,"confirmation required",http.StatusBadRequest);return}
	if len(req.Roots)==0||len(req.Roots)>32{http.Error(w,"1 to 32 roots required",http.StatusBadRequest);return}
	allowed:=make([]string,0,len(req.Roots));for _,raw:=range req.Roots{root:=filepath.Clean(strings.TrimSpace(raw));if !s.validSharePath(root){http.Error(w,"invalid root",http.StatusBadRequest);return};allowed=append(allowed,filepath.Join(root,".docExtractor-duplicates"))}
	var targets []string
	if req.DeleteAll{
		for _,qroot:=range allowed{_ = filepath.WalkDir(qroot,func(path string,d os.DirEntry,err error)error{if err!=nil{return nil};if d.IsDir(){return nil};targets=append(targets,path);return nil})}
	}else{
		if len(req.Paths)==0||len(req.Paths)>10000{http.Error(w,"select 1 to 10000 files",http.StatusBadRequest);return}
		targets=append(targets,req.Paths...)
	}
	result:=quarantineDeleteResult{};seen:=map[string]struct{}{}
	for _,raw:=range targets{
		p:=filepath.Clean(strings.TrimSpace(raw));if _,ok:=seen[p];ok{continue};seen[p]=struct{}{}
		ok:=false;for _,qroot:=range allowed{if withinPath(qroot,p){ok=true;break}};if !ok{result.Errors=append(result.Errors,p+": outside quarantine");continue}
		st,err:=os.Lstat(p);if err!=nil{if errors.Is(err,os.ErrNotExist){continue};result.Errors=append(result.Errors,p+": "+err.Error());continue};if !st.Mode().IsRegular()||st.Mode()&os.ModeSymlink!=0{result.Errors=append(result.Errors,p+": not a regular quarantined file");continue}
		if err:=os.Remove(p);err!=nil{result.Errors=append(result.Errors,p+": "+err.Error());continue};result.Deleted++;result.FreedBytes+=st.Size()
	}
	for _,qroot:=range allowed{removeEmptyQuarantineDirs(qroot)}
	writeJSON(w,result)
}

func (s *Server) validSharePath(path string)bool{return path!="."&&filepath.IsAbs(path)&&withinPath(s.browseBase(),path)}
func removeEmptyQuarantineDirs(root string){var dirs []string;_ = filepath.WalkDir(root,func(path string,d os.DirEntry,err error)error{if err==nil&&d.IsDir()&&path!=root{dirs=append(dirs,path)};return nil});sort.Slice(dirs,func(i,j int)bool{return len(dirs[i])>len(dirs[j])});for _,d:=range dirs{if es,e:=os.ReadDir(d);e==nil&&len(es)==0{_ = os.Remove(d)}}}
