package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/souten-yd/docExtractor/internal/archive"
	"github.com/souten-yd/docExtractor/internal/classifier"
)

const (
	CollisionSkip      = "skip"
	CollisionOverwrite = "overwrite"
)

type Config struct {
	Root                string
	OutputRoot          string
	CollisionPolicy     string
	ConfidenceThreshold float64
	Aliases             map[string]string
}

type Organizer struct {
	mu                  sync.RWMutex
	root                string
	outputRoot          string
	collisionPolicy     string
	confidenceThreshold float64
	aliases             map[string]string
	resolved            map[string]string
}

type Plan struct {
	Name           string              `json:"name"`
	Source         string              `json:"source"`
	Destination    string              `json:"destination"`
	Series         string              `json:"series"`
	Author         string              `json:"author,omitempty"`
	Volume         int                 `json:"volume,omitempty"`
	HasVolume      bool                `json:"has_volume"`
	Coverage       classifier.Coverage `json:"coverage"`
	Confidence     float64             `json:"confidence"`
	NeedsReview    bool                `json:"needs_review"`
	Action         string              `json:"action"`
	Overwrite      bool                `json:"overwrite,omitempty"`
	Skipped        bool                `json:"skipped,omitempty"`
	Entries        int                 `json:"entries,omitempty"`
	NameSource     string              `json:"name_source,omitempty"`
	Evidence       []string            `json:"evidence,omitempty"`
	Candidates     []string            `json:"candidates,omitempty"`
	CandidateCount int                 `json:"candidate_count,omitempty"`
	Cluster        ClusterInfo         `json:"cluster,omitempty"`
	Multipart      bool                `json:"multipart,omitempty"`
	PartCount      int                 `json:"part_count,omitempty"`
	Parts          []string            `json:"parts,omitempty"`
	Error          string              `json:"error,omitempty"`
}

func normalizeCollisionPolicy(v string) string { if v == CollisionOverwrite { return CollisionOverwrite }; return CollisionSkip }

func New(cfg Config) (*Organizer, error) {
	root, err := normalizeRoot(cfg.Root)
	if err != nil { return nil, err }
	outputRoot := strings.TrimSpace(cfg.OutputRoot)
	if outputRoot == "" { outputRoot = root }
	outputRoot, err = normalizeRoot(outputRoot)
	if err != nil { return nil, fmt.Errorf("output root: %w", err) }
	if cfg.ConfidenceThreshold <= 0 { cfg.ConfidenceThreshold = 0.72 }
	o := &Organizer{root: root, outputRoot: outputRoot, collisionPolicy: normalizeCollisionPolicy(cfg.CollisionPolicy), confidenceThreshold: cfg.ConfidenceThreshold, aliases: map[string]string{}, resolved: map[string]string{}}
	o.SetAliases(cfg.Aliases)
	return o, nil
}

func (o *Organizer) Root() string { o.mu.RLock(); defer o.mu.RUnlock(); return o.root }
func (o *Organizer) OutputRoot() string { o.mu.RLock(); defer o.mu.RUnlock(); return o.outputRoot }
func (o *Organizer) CollisionPolicy() string { o.mu.RLock(); defer o.mu.RUnlock(); return o.collisionPolicy }
func (o *Organizer) SetRoot(root string) error {
	normalized, err := normalizeRoot(root); if err != nil { return err }
	o.mu.Lock(); o.root = normalized; o.resolved = map[string]string{}; o.mu.Unlock(); return nil
}
func (o *Organizer) SetOutputRoot(root string) error {
	normalized, err := normalizeRoot(root); if err != nil { return err }
	o.mu.Lock(); o.outputRoot = normalized; o.resolved = map[string]string{}; o.mu.Unlock(); return nil
}
func (o *Organizer) SetCollisionPolicy(policy string) { o.mu.Lock(); o.collisionPolicy=normalizeCollisionPolicy(policy); o.mu.Unlock() }
func (o *Organizer) SetAliases(in map[string]string) {
	clean := make(map[string]string, len(in))
	for alias, canonical := range in {
		alias = strings.TrimSpace(alias); canonical = classifier.SafeFolderName(strings.TrimSpace(canonical))
		if alias == "" || canonical == "" || canonical == "Unknown" { continue }
		clean[alias] = canonical
	}
	o.mu.Lock(); o.aliases = clean; o.resolved = map[string]string{}; o.mu.Unlock()
}
func (o *Organizer) Aliases() map[string]string { o.mu.RLock(); defer o.mu.RUnlock(); out:=make(map[string]string,len(o.aliases)); for k,v:=range o.aliases{out[k]=v}; return out }

func (o *Organizer) Scan() ([]Plan, error) {
	root := o.Root()
	entries, err := os.ReadDir(root); if err != nil { return nil, err }
	plans := make([]Plan, 0)
	for _, entry := range entries {
		if entry.IsDir() { continue }
		ext := strings.ToLower(filepath.Ext(entry.Name())); if ext != ".zip" && ext != ".rar" { continue }
		if ext == ".rar" && archive.IsSecondaryRARVolume(entry.Name()) { continue }
		plan, err := o.planNameAt(root, entry.Name(), false)
		if err != nil { plans = append(plans, Plan{Name: entry.Name(), Source: filepath.Join(root, entry.Name()), NeedsReview: true, Error: err.Error()}); continue }
		plans = append(plans, plan)
	}
	plans = clusterPlans(plans, o.Aliases())
	resolved := make(map[string]string, len(plans))
	for _, p := range plans { if p.Error == "" && p.Series != "" { resolved[p.Name] = p.Series } }
	o.mu.Lock(); o.resolved = resolved; o.mu.Unlock()
	// Clustering may change the canonical series, so recompute destination paths.
	for i := range plans { if plans[i].Error=="" { o.applyDestination(&plans[i]) } }
	sort.Slice(plans, func(i,j int)bool{return plans[i].Name<plans[j].Name})
	return plans,nil
}

func (o *Organizer) PlanName(name string) (Plan,error) { return o.planNameAt(o.Root(),name,true) }

func (o *Organizer) applyDestination(plan *Plan) {
	outputRoot:=o.OutputRoot(); policy:=o.CollisionPolicy()
	seriesDir:=filepath.Join(outputRoot,plan.Series)
	if err:=rejectSymlinkComponents(outputRoot,seriesDir);err!=nil{plan.Error=err.Error();plan.NeedsReview=true;return}
	outName:=plan.Name
	if strings.EqualFold(filepath.Ext(plan.Name),".rar"){outName=archive.MultipartOutputStem(plan.Name)+".zip"}
	plan.Destination=filepath.Join(seriesDir,outName)
	plan.Overwrite=policy==CollisionOverwrite
	plan.Skipped=false
	if _,err:=os.Lstat(plan.Destination);err==nil{
		if policy==CollisionSkip { plan.Action="skip-existing"; plan.Skipped=true; plan.NeedsReview=false } else { plan.Evidence=append(plan.Evidence,"existing output will be replaced") }
	}else if !errors.Is(err,os.ErrNotExist){plan.Error=err.Error();plan.NeedsReview=true}
}

func (o *Organizer) planNameAt(root,name string,useResolved bool)(Plan,error){
	if filepath.Base(name)!=name||name=="."||name==".."{return Plan{},errors.New("name must be a single file in the configured root")}
	ext:=strings.ToLower(filepath.Ext(name)); if ext!=".zip"&&ext!=".rar"{return Plan{},fmt.Errorf("unsupported extension: %s",ext)}
	source:=filepath.Join(root,name); if !allowedAt(root,source){return Plan{},errors.New("source escapes configured root")}
	st,err:=os.Lstat(source); if err!=nil{return Plan{},err}; if st.Mode()&os.ModeSymlink!=0{return Plan{},errors.New("symbolic-link archives are not allowed")}; if !st.Mode().IsRegular(){return Plan{},errors.New("source is not a regular archive")}

	multipart:=false; partCount:=0; partNames:=[]string(nil)
	if ext==".rar" {
		set,isMulti,merr:=archive.DiscoverMultipartRAR(source); if merr!=nil{return Plan{},merr}
		if isMulti {
			multipart=true
			if set.Primary=="" { return Plan{},errors.New("multipart RAR is missing part1") }
			if filepath.Clean(source)!=filepath.Clean(set.Primary) { return Plan{},errors.New("secondary multipart RAR volume cannot be executed directly") }
			if len(set.Missing)>0 { return Plan{},fmt.Errorf("multipart RAR is missing part(s): %v",set.Missing) }
			partCount=len(set.Parts); partNames=make([]string,0,len(set.Parts)); for _,p:=range set.Parts { partNames=append(partNames,filepath.Base(p)) }
		}
	}

	naming:=inferFromArchive(name,source); parsed:=naming.Parsed
	if canonical,ok:=aliasLookup(o.Aliases(),parsed.Series);ok{parsed.Series=canonical; naming.Evidence=append(naming.Evidence,"saved alias")}
	if useResolved { o.mu.RLock(); canonical:=o.resolved[name]; o.mu.RUnlock(); if canonical!=""{parsed.Series=canonical; naming.Evidence=append(naming.Evidence,"scan cluster") } }
	action:="rename-zip"; entries:=0
	if ext==".rar"{info,err:=archive.InspectRAR(source); if err!=nil{return Plan{},fmt.Errorf("RAR inspection failed: %w",err)}; entries=info.RegularFiles; if info.SingleNestedZIP{action="unwrap-nested-zip"}else{action="rar-to-zip"}}
	plan:=Plan{Name:name,Source:source,Series:parsed.Series,Author:parsed.Author,Volume:parsed.Volume,HasVolume:parsed.HasVolume,Coverage:naming.Coverage,Confidence:parsed.Confidence,NeedsReview:parsed.Confidence<o.confidenceThreshold,Action:action,Entries:entries,NameSource:naming.Source,Evidence:naming.Evidence,Candidates:naming.Candidates,CandidateCount:naming.CandidateCount,Multipart:multipart,PartCount:partCount,Parts:partNames}
	if multipart { plan.Evidence=append(plan.Evidence,fmt.Sprintf("multipart RAR: %d parts",partCount)) }
	o.applyDestination(&plan)
	return plan,nil
}

func normalizeRoot(root string)(string,error){root=strings.TrimSpace(root);if root==""{return "",errors.New("root is required")};abs,err:=filepath.Abs(root);if err!=nil{return "",err};abs=filepath.Clean(abs);st,err:=os.Stat(abs);if err!=nil{return "",fmt.Errorf("root is not accessible: %w",err)};if !st.IsDir(){return "",errors.New("root is not a directory")};return abs,nil}
func allowedAt(root,filename string)bool{rel,err:=filepath.Rel(root,filepath.Clean(filename));if err!=nil{return false};return rel!=".."&&!strings.HasPrefix(rel,".."+string(os.PathSeparator))}
func rejectSymlinkComponents(root,target string)error{rel,err:=filepath.Rel(root,filepath.Clean(target));if err!=nil{return err};if rel==".."||strings.HasPrefix(rel,".."+string(os.PathSeparator)){return errors.New("destination escapes configured root")};current:=root;for _,part:=range strings.Split(rel,string(os.PathSeparator)){if part==""||part=="."{continue};current=filepath.Join(current,part);st,err:=os.Lstat(current);if errors.Is(err,os.ErrNotExist){return nil};if err!=nil{return err};if st.Mode()&os.ModeSymlink!=0{return fmt.Errorf("destination path contains symbolic link: %s",part)};if !st.IsDir(){return fmt.Errorf("destination path component is not a directory: %s",part)}};return nil}
