package organizer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/souten-yd/docExtractor/internal/archive"
)

type ReconcileItem struct {
	Source       string  `json:"source"`
	Relative     string  `json:"relative"`
	Series       string  `json:"series"`
	Destination  string  `json:"destination"`
	Action       string  `json:"action"`
	Confidence   float64 `json:"confidence"`
	Reason       string  `json:"reason,omitempty"`
	DuplicateOf  string  `json:"duplicate_of,omitempty"`
	Size         int64   `json:"size"`
}

type ReconcileSummary struct { Files int `json:"files"`; Keep int `json:"keep"`; Move int `json:"move"`; Duplicates int `json:"duplicates"`; Conflicts int `json:"conflicts"`; Errors int `json:"errors"` }
type ReconcileReport struct { Root string `json:"root"`; Items []ReconcileItem `json:"items"`; Summary ReconcileSummary `json:"summary"` }
type ReconcileResult struct { Moved int `json:"moved"`; Quarantined int `json:"quarantined"`; Skipped int `json:"skipped"`; Errors []string `json:"errors,omitempty"` }

type reconcileRaw struct { path string; rel string; name string; series string; confidence float64; size int64; err error }

func (o *Organizer) ReconcileScan(root string) (ReconcileReport,error) {
	root,err:=normalizeRoot(root);if err!=nil{return ReconcileReport{},err}
	raws:=make([]reconcileRaw,0)
	err=filepath.WalkDir(root,func(path string,d os.DirEntry,walkErr error)error{
		if walkErr!=nil{return walkErr};if path==root{return nil};rel,_:=filepath.Rel(root,path)
		if d.IsDir(){if d.Name()==".docExtractor-duplicates"{return filepath.SkipDir};return nil}
		ext:=strings.ToLower(filepath.Ext(d.Name()));if ext!=".zip"&&ext!=".rar"{return nil};if ext==".rar"&&archive.IsSecondaryRARVolume(d.Name()){return nil}
		st,e:=os.Lstat(path);if e!=nil{raws=append(raws,reconcileRaw{path:path,rel:rel,name:d.Name(),err:e});return nil};if st.Mode()&os.ModeSymlink!=0||!st.Mode().IsRegular(){return nil}
		n:=inferFromArchive(d.Name(),path);series:=n.Parsed.Series;confidence:=n.Parsed.Confidence
		parent:=filepath.Base(filepath.Dir(path));if filepath.Dir(path)!=root&&parent!="."&&parent!=""{
			if score,_:=sameSeries(series,parent);score>=.90{if richerSeries(parent,series){series=parent};if confidence<.90{confidence=.90}} else if confidence<o.confidenceThreshold { series=parent; confidence=.78 }
		}
		if canonical,ok:=aliasLookup(o.Aliases(),series);ok{series=canonical;confidence=1}
		raws=append(raws,reconcileRaw{path:path,rel:rel,name:d.Name(),series:series,confidence:confidence,size:st.Size()});return nil
	});if err!=nil{return ReconcileReport{},err}
	plans:=make([]Plan,len(raws));for i,r:=range raws{if r.err!=nil{plans[i]=Plan{Name:r.rel,Series:"",Error:r.err.Error()}}else{plans[i]=Plan{Name:r.rel,Series:r.series,Confidence:r.confidence}}
	plans=clusterPlans(plans,o.Aliases())
	items:=make([]ReconcileItem,len(raws));for i,r:=range raws{it:=ReconcileItem{Source:r.path,Relative:r.rel,Series:plans[i].Series,Confidence:r.confidence,Size:r.size};if r.err!=nil{it.Action="error";it.Reason=r.err.Error();items[i]=it;continue};if it.Series==""{it.Action="error";it.Reason="series could not be determined";items[i]=it;continue};it.Destination=filepath.Join(root,it.Series,filepath.Base(r.path));if filepath.Clean(it.Destination)==filepath.Clean(r.path){it.Action="keep"}else{it.Action="move";it.Reason="normalize series folder"};items[i]=it}
	markExactDuplicates(root,items)
	// Existing non-duplicate destination collisions are not overwritten in migration mode.
	for i:=range items{it:=&items[i];if it.Action!="move"{continue};if st,e:=os.Lstat(it.Destination);e==nil&&st.Mode().IsRegular()&&filepath.Clean(it.Destination)!=filepath.Clean(it.Source){it.Action="conflict";it.Reason="destination already exists but is not a verified duplicate"}}
	sort.Slice(items,func(i,j int)bool{return items[i].Relative<items[j].Relative});report:=ReconcileReport{Root:root,Items:items};for _,it:=range items{report.Summary.Files++;switch it.Action{case"keep":report.Summary.Keep++;case"move":report.Summary.Move++;case"duplicate":report.Summary.Duplicates++;case"conflict":report.Summary.Conflicts++;case"error":report.Summary.Errors++}};return report,nil
}

func markExactDuplicates(root string,items []ReconcileItem){groups:=map[int64][]int{};for i,it:=range items{if it.Action!="error"&&it.Size>=0{groups[it.Size]=append(groups[it.Size],i)}};for _,idxs:=range groups{if len(idxs)<2{continue};hashGroups:=map[string][]int{};for _,idx:=range idxs{h,err:=hashFile(items[idx].Source);if err!=nil{continue};hashGroups[h]=append(hashGroups[h],idx)};for _,dups:=range hashGroups{if len(dups)<2{continue};keeper:=dups[0];for _,idx:=range dups{if items[idx].Action=="keep"{keeper=idx;break}};for _,idx:=range dups{if idx==keeper{continue};items[idx].Action="duplicate";items[idx].DuplicateOf=items[keeper].Relative;items[idx].Destination=uniqueQuarantinePath(root,items[idx].Series,filepath.Base(items[idx].Source));items[idx].Reason="SHA-256 exact duplicate"}}}}
func hashFile(path string)(string,error){f,err:=os.Open(path);if err!=nil{return "",err};defer f.Close();h:=sha256.New();if _,err:=io.CopyBuffer(h,f,make([]byte,4*1024*1024));err!=nil{return "",err};return hex.EncodeToString(h.Sum(nil)),nil}
func uniqueQuarantinePath(root,series,name string)string{base:=filepath.Join(root,".docExtractor-duplicates",series,name);if _,err:=os.Lstat(base);errors.Is(err,os.ErrNotExist){return base};ext:=filepath.Ext(name);stem:=strings.TrimSuffix(name,ext);for n:=2;n<10000;n++{p:=filepath.Join(root,".docExtractor-duplicates",series,fmt.Sprintf("%s (%d)%s",stem,n,ext));if _,err:=os.Lstat(p);errors.Is(err,os.ErrNotExist){return p}};return base+".duplicate"}

func (o *Organizer) ReconcileExecute(root string)(ReconcileResult,error){report,err:=o.ReconcileScan(root);if err!=nil{return ReconcileResult{},err};result:=ReconcileResult{};for _,it:=range report.Items{if it.Action!="move"&&it.Action!="duplicate"{result.Skipped++;continue};if err:=os.MkdirAll(filepath.Dir(it.Destination),0o750);err!=nil{result.Errors=append(result.Errors,it.Relative+": "+err.Error());continue};if _,err:=os.Lstat(it.Destination);err==nil{result.Skipped++;continue};if err:=os.Rename(it.Source,it.Destination);err!=nil{result.Errors=append(result.Errors,it.Relative+": "+err.Error());continue};if it.Action=="duplicate"{result.Quarantined++}else{result.Moved++}}
	removeEmptyLibraryDirs(report.Root);return result,nil}
func removeEmptyLibraryDirs(root string){var dirs []string;_ = filepath.WalkDir(root,func(path string,d os.DirEntry,err error)error{if err!=nil{return nil};if d.IsDir()&&path!=root&&!strings.Contains(path,string(os.PathSeparator)+".docExtractor-duplicates"){dirs=append(dirs,path)};return nil});sort.Slice(dirs,func(i,j int)bool{return len(dirs[i])>len(dirs[j])});for _,d:=range dirs{entries,err:=os.ReadDir(d);if err==nil&&len(entries)==0{_ = os.Remove(d)}}}
