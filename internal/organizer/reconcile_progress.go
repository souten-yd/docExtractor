package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/souten-yd/docExtractor/internal/archive"
)

type ReconcileProgress struct {
	Phase string `json:"phase"`
	Completed int `json:"completed"`
	Total int `json:"total"`
	Message string `json:"message,omitempty"`
}

type ReconcileProgressFunc func(ReconcileProgress)

func emitReconcileProgress(cb ReconcileProgressFunc, phase string, completed,total int,msg string){if cb!=nil{cb(ReconcileProgress{Phase:phase,Completed:completed,Total:total,Message:msg})}}

// ReconcileScanMultiProgress is the large-library variant. It performs a cheap count pass first,
// then reports file-level progress while inspecting archives and hashing duplicate candidates.
func (o *Organizer) ReconcileScanMultiProgress(roots []string, outputRoot string, cb ReconcileProgressFunc) (ReconcileReport,error){
	roots,outputRoot,err:=normalizeReconcileRoots(roots,outputRoot);if err!=nil{return ReconcileReport{},err}
	total:=0
	emitReconcileProgress(cb,"counting",0,0,"対象ファイル数を確認中")
	for _,root:=range roots{err=filepath.WalkDir(root,func(path string,d os.DirEntry,e error)error{if e!=nil{return e};if path==root{return nil};if d.IsDir(){if d.Name()==".docExtractor-duplicates"{return filepath.SkipDir};return nil};ext:=strings.ToLower(filepath.Ext(d.Name()));if ext==".zip"||(ext==".rar"&&!archive.IsSecondaryRARVolume(d.Name())){total++};return nil});if err!=nil{return ReconcileReport{},err}}
	emitReconcileProgress(cb,"inspecting",0,total,"内部名とシリーズを解析中")
	raws:=make([]reconcileRaw,0,total);done:=0
	for _,root:=range roots{
		err=filepath.WalkDir(root,func(path string,d os.DirEntry,walkErr error)error{
			if walkErr!=nil{return walkErr};if path==root{return nil};rel,_:=filepath.Rel(root,path)
			if d.IsDir(){if d.Name()==".docExtractor-duplicates"{return filepath.SkipDir};return nil}
			ext:=strings.ToLower(filepath.Ext(d.Name()));if ext!=".zip"&&ext!=".rar"{return nil};if ext==".rar"&&archive.IsSecondaryRARVolume(d.Name()){return nil}
			st,e:=os.Lstat(path);if e!=nil{raws=append(raws,reconcileRaw{path:path,root:root,rel:rel,name:d.Name(),err:e});done++;emitReconcileProgress(cb,"inspecting",done,total,d.Name());return nil};if st.Mode()&os.ModeSymlink!=0||!st.Mode().IsRegular(){done++;emitReconcileProgress(cb,"inspecting",done,total,d.Name());return nil}
			n:=inferFromArchive(d.Name(),path);series,confidence:=n.Parsed.Series,n.Parsed.Confidence;parent:=filepath.Base(filepath.Dir(path))
			if filepath.Dir(path)!=root&&parent!="."&&parent!=""{if score,_:=sameSeries(series,parent);score>=.90{if richerSeries(parent,series){series=parent};if confidence<.90{confidence=.90}}else if confidence<o.confidenceThreshold{series,confidence=cleanLegacyFolderName(parent),.78}}
			if canonical,ok:=aliasLookup(o.Aliases(),series);ok{series,confidence=canonical,1}
			raws=append(raws,reconcileRaw{path:path,root:root,rel:rel,name:d.Name(),series:series,confidence:confidence,size:st.Size(),modified:st.ModTime(),volume:n.Parsed.Volume,hasVolume:n.Parsed.HasVolume});done++;emitReconcileProgress(cb,"inspecting",done,total,d.Name());return nil
		});if err!=nil{return ReconcileReport{},err}
	}
	emitReconcileProgress(cb,"clustering",done,total,"シリーズ表記を統合中")
	plans:=make([]Plan,len(raws));for i,r:=range raws{if r.err!=nil{plans[i]=Plan{Name:r.rel,Error:r.err.Error()}}else{plans[i]=Plan{Name:r.rel,Series:r.series,Confidence:r.confidence}}};plans=clusterPlans(plans,o.Aliases())
	items:=make([]ReconcileItem,len(raws));for i,r:=range raws{it:=ReconcileItem{Source:r.path,LibraryRoot:r.root,Relative:r.rel,Series:plans[i].Series,Confidence:r.confidence,Size:r.size,ModifiedAt:r.modified,Volume:r.volume,HasVolume:r.hasVolume};if r.err!=nil{it.Action,it.Reason="error",r.err.Error();items[i]=it;continue};if it.Series==""{it.Action,it.Reason="error","series could not be determined";items[i]=it;continue};it.Destination=filepath.Join(outputRoot,it.Series,filepath.Base(r.path));if filepath.Clean(it.Destination)==filepath.Clean(r.path){it.Action="keep"}else{it.Action,it.Reason="move","normalize series folder"};items[i]=it}
	emitReconcileProgress(cb,"duplicates",0,total,"同サイズ候補のSHA-256を確認中");markExactDuplicatesProgress(outputRoot,items,cb,total)
	emitReconcileProgress(cb,"variants",total,total,"同一巻の版を比較中");choices:=resolveSameVolumeVariants(outputRoot,items);markDestinationConflicts(items)
	sort.Slice(items,func(i,j int)bool{if items[i].Series!=items[j].Series{return strings.ToLower(items[i].Series)<strings.ToLower(items[j].Series)};if items[i].HasVolume&&items[j].HasVolume&&items[i].Volume!=items[j].Volume{return items[i].Volume<items[j].Volume};return items[i].Source<items[j].Source})
	report:=ReconcileReport{Root:roots[0],Roots:roots,OutputRoot:outputRoot,Items:items,Choices:choices};for _,it:=range items{report.Summary.Files++;switch it.Action{case"keep":report.Summary.Keep++;case"move":report.Summary.Move++;case"duplicate":report.Summary.Duplicates++;case"superseded":report.Summary.Superseded++;case"conflict":report.Summary.Conflicts++;case"review":report.Summary.Review++;case"error":report.Summary.Errors++}}
	emitReconcileProgress(cb,"done",total,total,"解析完了");return report,nil
}

func markExactDuplicatesProgress(root string,items []ReconcileItem,cb ReconcileProgressFunc,total int){groups:=map[int64][]int{};for i,it:=range items{if it.Action!="error"&&it.Size>=0{groups[it.Size]=append(groups[it.Size],i)}};hashTotal:=0;for _,idxs:=range groups{if len(idxs)>1{hashTotal+=len(idxs)}};hashed:=0;for _,idxs:=range groups{if len(idxs)<2{continue};hashGroups:=map[string][]int{};for _,idx:=range idxs{h,err:=hashFile(items[idx].Source);hashed++;emitReconcileProgress(cb,"duplicates",hashed,hashTotal,filepath.Base(items[idx].Source));if err!=nil{continue};hashGroups[h]=append(hashGroups[h],idx)};for _,dups:=range hashGroups{if len(dups)<2{continue};keeper:=dups[0];for _,idx:=range dups[1:]{if items[idx].ModifiedAt.After(items[keeper].ModifiedAt){keeper=idx}};for _,idx:=range dups{if idx==keeper{continue};items[idx].Action="duplicate";items[idx].DuplicateOf=items[keeper].Source;items[idx].Destination=uniqueQuarantinePath(root,items[idx].Series,filepath.Base(items[idx].Source));items[idx].Reason="SHA-256 exact duplicate; newer copy kept"}}};if hashTotal==0{emitReconcileProgress(cb,"duplicates",total,total,"重複ハッシュ候補なし")}}

// ReconcileExecuteReportProgress executes a previously scanned report, avoiding a second full scan.
func (o *Organizer) ReconcileExecuteReportProgress(report ReconcileReport,selections map[string]string,cb ReconcileProgressFunc)(ReconcileResult,error){
	if len(report.Roots)==0{return ReconcileResult{},errors.New("reconcile report has no roots")}
	if len(report.Choices)>0{for _,choice:=range report.Choices{selected:=filepath.Clean(selections[choice.ID]);if selections==nil||selected=="."||selected==""{return ReconcileResult{},fmt.Errorf("selection required for %s volume %d",choice.Series,choice.Volume)};found:=false;for i:=range report.Items{it:=&report.Items[i];if it.ReviewGroup!=choice.ID{continue};if filepath.Clean(it.Source)==selected{found=true;if filepath.Clean(it.Source)==filepath.Clean(filepath.Join(report.OutputRoot,it.Series,filepath.Base(it.Source))){it.Action="keep"}else{it.Action="move"};it.Destination=filepath.Join(report.OutputRoot,it.Series,filepath.Base(it.Source));it.Reason="selected by user"}else{it.Action="superseded";it.Destination=uniqueQuarantinePath(report.OutputRoot,it.Series,filepath.Base(it.Source));it.DuplicateOf=selected;it.Reason="not selected for same series/volume"}};if !found{return ReconcileResult{},fmt.Errorf("invalid selection for %s volume %d",choice.Series,choice.Volume)}}}
	total:=0;for _,it:=range report.Items{if it.Action=="move"||it.Action=="duplicate"||it.Action=="superseded"{total++}};emitReconcileProgress(cb,"executing",0,total,"整理を実行中");result:=ReconcileResult{};done:=0
	for _,it:=range report.Items{if it.Action!="move"&&it.Action!="duplicate"&&it.Action!="superseded"{result.Skipped++;continue};if err:=os.MkdirAll(filepath.Dir(it.Destination),0o750);err!=nil{result.Errors=append(result.Errors,it.Relative+": "+err.Error());done++;emitReconcileProgress(cb,"executing",done,total,it.Relative);continue};if _,err:=os.Lstat(it.Destination);err==nil{result.Skipped++;done++;emitReconcileProgress(cb,"executing",done,total,it.Relative);continue};if err:=os.Rename(it.Source,it.Destination);err!=nil{result.Errors=append(result.Errors,it.Relative+": "+err.Error());done++;emitReconcileProgress(cb,"executing",done,total,it.Relative);continue};if it.Action=="duplicate"||it.Action=="superseded"{result.Quarantined++}else{result.Moved++};done++;emitReconcileProgress(cb,"executing",done,total,it.Relative)}
	for _,root:=range report.Roots{removeEmptyLibraryDirs(root)};emitReconcileProgress(cb,"done",total,total,"整理完了");return result,nil
}

var _ = strconv.Itoa
