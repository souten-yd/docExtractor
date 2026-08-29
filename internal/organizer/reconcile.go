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
	"strconv"
	"strings"
	"time"

	"github.com/souten-yd/docExtractor/internal/archive"
)

type ReconcileItem struct {
	Source       string    `json:"source"`
	LibraryRoot  string    `json:"library_root"`
	Relative     string    `json:"relative"`
	Series       string    `json:"series"`
	Destination  string    `json:"destination"`
	Action       string    `json:"action"`
	Confidence   float64   `json:"confidence"`
	Reason       string    `json:"reason,omitempty"`
	DuplicateOf  string    `json:"duplicate_of,omitempty"`
	Size         int64     `json:"size"`
	ModifiedAt   time.Time `json:"modified_at"`
	Volume       int       `json:"volume,omitempty"`
	HasVolume    bool      `json:"has_volume"`
	ReviewGroup  string    `json:"review_group,omitempty"`
	AutoSelected bool      `json:"auto_selected,omitempty"`
}

type ReconcileChoice struct {
	ID         string          `json:"id"`
	Series     string          `json:"series"`
	Volume     int             `json:"volume,omitempty"`
	HasVolume  bool            `json:"has_volume"`
	Reason     string          `json:"reason"`
	Candidates []ReconcileItem `json:"candidates"`
}

type ReconcileSummary struct {
	Files       int `json:"files"`
	Keep        int `json:"keep"`
	Move        int `json:"move"`
	Duplicates  int `json:"duplicates"`
	Superseded  int `json:"superseded"`
	Conflicts   int `json:"conflicts"`
	Review      int `json:"review"`
	Errors      int `json:"errors"`
}

type ReconcileReport struct {
	Root        string            `json:"root,omitempty"`
	Roots       []string          `json:"roots"`
	OutputRoot  string            `json:"output_root"`
	Items       []ReconcileItem   `json:"items"`
	Choices     []ReconcileChoice `json:"choices,omitempty"`
	Summary     ReconcileSummary  `json:"summary"`
}

type ReconcileResult struct {
	Moved       int      `json:"moved"`
	Quarantined int      `json:"quarantined"`
	Skipped     int      `json:"skipped"`
	Errors      []string `json:"errors,omitempty"`
}

type reconcileRaw struct {
	path, root, rel, name, series string
	confidence                    float64
	size                          int64
	modified                      time.Time
	volume                        int
	hasVolume                     bool
	err                           error
}

func (o *Organizer) ReconcileScan(root string) (ReconcileReport, error) {
	return o.ReconcileScanMulti([]string{root}, root)
}

func (o *Organizer) ReconcileScanMulti(roots []string, outputRoot string) (ReconcileReport, error) {
	roots, outputRoot, err := normalizeReconcileRoots(roots, outputRoot)
	if err != nil { return ReconcileReport{}, err }
	raws := make([]reconcileRaw, 0, 256)
	for _, root := range roots {
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil { return walkErr }
			if path == root { return nil }
			rel, _ := filepath.Rel(root, path)
			if d.IsDir() {
				if d.Name() == ".docExtractor-duplicates" { return filepath.SkipDir }
				return nil
			}
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if ext != ".zip" && ext != ".rar" { return nil }
			if ext == ".rar" && archive.IsSecondaryRARVolume(d.Name()) { return nil }
			st, e := os.Lstat(path)
			if e != nil { raws = append(raws, reconcileRaw{path:path, root:root, rel:rel, name:d.Name(), err:e}); return nil }
			if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() { return nil }
			n := inferFromArchive(d.Name(), path)
			series, confidence := n.Parsed.Series, n.Parsed.Confidence
			parent := filepath.Base(filepath.Dir(path))
			if filepath.Dir(path) != root && parent != "." && parent != "" {
				if score, _ := sameSeries(series, parent); score >= .90 {
					if richerSeries(parent, series) { series = parent }
					if confidence < .90 { confidence = .90 }
				} else if confidence < o.confidenceThreshold {
					series, confidence = cleanLegacyFolderName(parent), .78
				}
			}
			if canonical, ok := aliasLookup(o.Aliases(), series); ok { series, confidence = canonical, 1 }
			raws = append(raws, reconcileRaw{path:path, root:root, rel:rel, name:d.Name(), series:series, confidence:confidence, size:st.Size(), modified:st.ModTime(), volume:n.Parsed.Volume, hasVolume:n.Parsed.HasVolume})
			return nil
		})
		if err != nil { return ReconcileReport{}, err }
	}

	plans := make([]Plan, len(raws))
	for i, r := range raws {
		if r.err != nil { plans[i] = Plan{Name:r.rel, Error:r.err.Error()} } else { plans[i] = Plan{Name:r.rel, Series:r.series, Confidence:r.confidence} }
	}
	plans = clusterPlans(plans, o.Aliases())
	items := make([]ReconcileItem, len(raws))
	for i, r := range raws {
		it := ReconcileItem{Source:r.path, LibraryRoot:r.root, Relative:r.rel, Series:plans[i].Series, Confidence:r.confidence, Size:r.size, ModifiedAt:r.modified, Volume:r.volume, HasVolume:r.hasVolume}
		if r.err != nil { it.Action, it.Reason = "error", r.err.Error(); items[i] = it; continue }
		if it.Series == "" { it.Action, it.Reason = "error", "series could not be determined"; items[i] = it; continue }
		it.Destination = filepath.Join(outputRoot, it.Series, filepath.Base(r.path))
		if filepath.Clean(it.Destination) == filepath.Clean(r.path) { it.Action = "keep" } else { it.Action, it.Reason = "move", "normalize series folder" }
		items[i] = it
	}

	markExactDuplicates(outputRoot, items)
	choices := resolveSameVolumeVariants(outputRoot, items)
	markDestinationConflicts(items)
	sort.Slice(items, func(i,j int) bool {
		if items[i].Series != items[j].Series { return strings.ToLower(items[i].Series) < strings.ToLower(items[j].Series) }
		if items[i].HasVolume && items[j].HasVolume && items[i].Volume != items[j].Volume { return items[i].Volume < items[j].Volume }
		return items[i].Source < items[j].Source
	})
	report := ReconcileReport{Root:roots[0], Roots:roots, OutputRoot:outputRoot, Items:items, Choices:choices}
	for _, it := range items {
		report.Summary.Files++
		switch it.Action {
		case "keep": report.Summary.Keep++
		case "move": report.Summary.Move++
		case "duplicate": report.Summary.Duplicates++
		case "superseded": report.Summary.Superseded++
		case "conflict": report.Summary.Conflicts++
		case "review": report.Summary.Review++
		case "error": report.Summary.Errors++
		}
	}
	return report, nil
}

func normalizeReconcileRoots(roots []string, outputRoot string) ([]string, string, error) {
	if len(roots) == 0 { return nil, "", errors.New("at least one library root is required") }
	seen := map[string]struct{}{}
	out := make([]string,0,len(roots))
	for _, root := range roots {
		n, err := normalizeRoot(strings.TrimSpace(root)); if err != nil { return nil,"",err }
		if _, ok := seen[n]; ok { continue }; seen[n]=struct{}{}; out=append(out,n)
	}
	if len(out)==0 { return nil,"",errors.New("at least one library root is required") }
	if strings.TrimSpace(outputRoot)=="" { outputRoot=out[0] }
	nOut, err := normalizeRoot(outputRoot); if err != nil { return nil,"",fmt.Errorf("output root: %w",err) }
	return out,nOut,nil
}

func cleanLegacyFolderName(s string) string {
	s = strings.TrimSpace(s)
	for _, suffix := range []string{" 単行本", "【単行本】", "[単行本]", " コミックス", "【コミックス】", "[コミックス]"} {
		s = strings.TrimSpace(strings.TrimSuffix(s, suffix))
	}
	return s
}

func markExactDuplicates(root string, items []ReconcileItem) {
	groups := map[int64][]int{}
	for i,it := range items { if it.Action!="error" && it.Size>=0 { groups[it.Size]=append(groups[it.Size],i) } }
	for _, idxs := range groups {
		if len(idxs)<2 { continue }
		hashGroups:=map[string][]int{}
		for _,idx:=range idxs { h,err:=hashFile(items[idx].Source); if err!=nil { continue }; hashGroups[h]=append(hashGroups[h],idx) }
		for _,dups:=range hashGroups {
			if len(dups)<2 { continue }
			keeper:=dups[0]
			for _,idx:=range dups[1:] { if items[idx].ModifiedAt.After(items[keeper].ModifiedAt) { keeper=idx } }
			for _,idx:=range dups {
				if idx==keeper { continue }
				items[idx].Action="duplicate"; items[idx].DuplicateOf=items[keeper].Source
				items[idx].Destination=uniqueQuarantinePath(root,items[idx].Series,filepath.Base(items[idx].Source))
				items[idx].Reason="SHA-256 exact duplicate; newer copy kept"
			}
		}
	}
}

func resolveSameVolumeVariants(root string, items []ReconcileItem) []ReconcileChoice {
	groups:=map[string][]int{}
	for i,it:=range items {
		if it.Action=="error" || it.Action=="duplicate" || !it.HasVolume || it.Series=="" { continue }
		key:=canonicalKey(it.Series)+"#"+strconv.Itoa(it.Volume)
		groups[key]=append(groups[key],i)
	}
	choices:=make([]ReconcileChoice,0)
	seq:=0
	for _,idxs:=range groups {
		if len(idxs)<2 { continue }
		// A single strictly-newest mtime is safe to select automatically. Equal latest timestamps require user choice.
		sort.SliceStable(idxs,func(i,j int)bool{return items[idxs[i]].ModifiedAt.After(items[idxs[j]].ModifiedAt)})
		winner:=idxs[0]
		if items[winner].ModifiedAt.After(items[idxs[1]].ModifiedAt) {
			items[winner].AutoSelected=true
			items[winner].Reason="newest modified time among same series/volume"
			for _,idx:=range idxs[1:] {
				items[idx].Action="superseded"
				items[idx].DuplicateOf=items[winner].Source
				items[idx].Destination=uniqueQuarantinePath(root,items[idx].Series,filepath.Base(items[idx].Source))
				items[idx].Reason="older variant of same series/volume; newer file selected"
			}
			continue
		}
		seq++; id:=fmt.Sprintf("volume-choice-%d",seq)
		choice:=ReconcileChoice{ID:id,Series:items[winner].Series,Volume:items[winner].Volume,HasVolume:true,Reason:"same series/volume has no unique newest file"}
		for _,idx:=range idxs { items[idx].Action="review";items[idx].ReviewGroup=id;items[idx].Reason="user selection required";choice.Candidates=append(choice.Candidates,items[idx]) }
		choices=append(choices,choice)
	}
	return choices
}

func markDestinationConflicts(items []ReconcileItem) {
	for i:=range items {
		it:=&items[i]
		if it.Action!="move" && it.Action!="keep" { continue }
		if filepath.Clean(it.Destination)==filepath.Clean(it.Source) { continue }
		if st,e:=os.Lstat(it.Destination); e==nil && st.Mode().IsRegular() { it.Action="conflict";it.Reason="destination already exists but is not a verified duplicate" }
	}
}

func hashFile(path string)(string,error){f,err:=os.Open(path);if err!=nil{return "",err};defer f.Close();h:=sha256.New();if _,err:=io.CopyBuffer(h,f,make([]byte,4*1024*1024));err!=nil{return "",err};return hex.EncodeToString(h.Sum(nil)),nil}
func uniqueQuarantinePath(root,series,name string)string{base:=filepath.Join(root,".docExtractor-duplicates",series,name);if _,err:=os.Lstat(base);errors.Is(err,os.ErrNotExist){return base};ext:=filepath.Ext(name);stem:=strings.TrimSuffix(name,ext);for n:=2;n<10000;n++{p:=filepath.Join(root,".docExtractor-duplicates",series,fmt.Sprintf("%s (%d)%s",stem,n,ext));if _,err:=os.Lstat(p);errors.Is(err,os.ErrNotExist){return p}};return base+".duplicate"}

func (o *Organizer) ReconcileExecute(root string)(ReconcileResult,error){return o.ReconcileExecuteMulti([]string{root},root,nil)}

func (o *Organizer) ReconcileExecuteMulti(roots []string, outputRoot string, selections map[string]string)(ReconcileResult,error){
	report,err:=o.ReconcileScanMulti(roots,outputRoot);if err!=nil{return ReconcileResult{},err}
	if len(report.Choices)>0 {
		for _,choice:=range report.Choices { selected:=filepath.Clean(selections[choice.ID]);if selections==nil||selected=="."||selected=="" { return ReconcileResult{},fmt.Errorf("selection required for %s volume %d",choice.Series,choice.Volume) }; found:=false;for i:=range report.Items { it:=&report.Items[i];if it.ReviewGroup!=choice.ID{continue};if filepath.Clean(it.Source)==selected {found=true;if filepath.Clean(it.Source)==filepath.Clean(filepath.Join(report.OutputRoot,it.Series,filepath.Base(it.Source))){it.Action="keep"}else{it.Action="move"};it.Destination=filepath.Join(report.OutputRoot,it.Series,filepath.Base(it.Source));it.Reason="selected by user"}else{it.Action="superseded";it.Destination=uniqueQuarantinePath(report.OutputRoot,it.Series,filepath.Base(it.Source));it.DuplicateOf=selected;it.Reason="not selected for same series/volume"}};if !found{return ReconcileResult{},fmt.Errorf("invalid selection for %s volume %d",choice.Series,choice.Volume)}}
	}
	result:=ReconcileResult{}
	for _,it:=range report.Items {
		if it.Action!="move"&&it.Action!="duplicate"&&it.Action!="superseded"{result.Skipped++;continue}
		if err:=os.MkdirAll(filepath.Dir(it.Destination),0o750);err!=nil{result.Errors=append(result.Errors,it.Relative+": "+err.Error());continue}
		if _,err:=os.Lstat(it.Destination);err==nil{result.Skipped++;continue}
		if err:=os.Rename(it.Source,it.Destination);err!=nil{result.Errors=append(result.Errors,it.Relative+": "+err.Error());continue}
		if it.Action=="duplicate"||it.Action=="superseded"{result.Quarantined++}else{result.Moved++}
	}
	for _,root:=range report.Roots{removeEmptyLibraryDirs(root)}
	return result,nil
}

func removeEmptyLibraryDirs(root string){var dirs []string;_ = filepath.WalkDir(root,func(path string,d os.DirEntry,err error)error{if err!=nil{return nil};if d.IsDir()&&path!=root&&!strings.Contains(path,string(os.PathSeparator)+".docExtractor-duplicates"){dirs=append(dirs,path)};return nil});sort.Slice(dirs,func(i,j int)bool{return len(dirs[i])>len(dirs[j])});for _,d:=range dirs{entries,err:=os.ReadDir(d);if err==nil&&len(entries)==0{_ = os.Remove(d)}}}
