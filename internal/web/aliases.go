package web

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/souten-yd/docExtractor/internal/classifier"
)

type aliasRequest struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
}
type aliasEntry struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
}

func (s *Server) listAliases(w http.ResponseWriter,r *http.Request){
	if s.Settings==nil{http.Error(w,"settings store unavailable",http.StatusServiceUnavailable);return}
	aliases:=s.Settings.Get().SeriesAliases;out:=make([]aliasEntry,0,len(aliases));for a,c:=range aliases{out=append(out,aliasEntry{Alias:a,Canonical:c})};sort.Slice(out,func(i,j int)bool{return strings.ToLower(out[i].Alias)<strings.ToLower(out[j].Alias)});writeJSON(w,out)
}
func (s *Server) saveAlias(w http.ResponseWriter,r *http.Request){
	if s.Settings==nil{http.Error(w,"settings store unavailable",http.StatusServiceUnavailable);return}
	r.Body=http.MaxBytesReader(w,r.Body,16*1024);var req aliasRequest;dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields();if err:=dec.Decode(&req);err!=nil{http.Error(w,"invalid request",http.StatusBadRequest);return}
	req.Alias=strings.TrimSpace(req.Alias);req.Canonical=classifier.SafeFolderName(strings.TrimSpace(req.Canonical));if req.Alias==""||req.Canonical==""||req.Canonical=="Unknown"{http.Error(w,"alias and canonical are required",http.StatusBadRequest);return};if len([]rune(req.Alias))>240||len([]rune(req.Canonical))>180{http.Error(w,"series name too long",http.StatusBadRequest);return}
	current:=s.Settings.Get();if current.SeriesAliases==nil{current.SeriesAliases=map[string]string{}};current.SeriesAliases[req.Alias]=req.Canonical;if err:=s.Settings.Save(current);err!=nil{http.Error(w,"failed to persist alias",http.StatusInternalServerError);return};s.Organizer.SetAliases(current.SeriesAliases);writeJSON(w,aliasEntry{Alias:req.Alias,Canonical:req.Canonical})
}
func (s *Server) deleteAlias(w http.ResponseWriter,r *http.Request){
	if s.Settings==nil{http.Error(w,"settings store unavailable",http.StatusServiceUnavailable);return};alias:=strings.TrimSpace(r.URL.Query().Get("alias"));if alias==""{http.Error(w,"alias is required",http.StatusBadRequest);return};current:=s.Settings.Get();delete(current.SeriesAliases,alias);if err:=s.Settings.Save(current);err!=nil{http.Error(w,"failed to persist alias",http.StatusInternalServerError);return};s.Organizer.SetAliases(current.SeriesAliases);w.WriteHeader(http.StatusNoContent)
}
