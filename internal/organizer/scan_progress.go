package organizer

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/souten-yd/docExtractor/internal/archive"
)

type ScanProgress struct {
	Phase     string
	Completed int
	Total     int
	Current   string
	Message   string
}

func (o *Organizer) ScanWithProgress(cb func(ScanProgress)) ([]Plan, error) {
	root := o.Root()
	entries, err := os.ReadDir(root)
	if err != nil { return nil, err }
	total := 0
	for _, entry := range entries {
		if entry.IsDir() { continue }
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".zip" && ext != ".rar" { continue }
		if ext == ".rar" && archive.IsSecondaryRARVolume(entry.Name()) { continue }
		total++
	}
	if cb != nil { cb(ScanProgress{Phase:"inspecting", Total:total, Message:"アーカイブを解析しています"}) }
	plans := make([]Plan, 0, total)
	completed := 0
	for _, entry := range entries {
		if entry.IsDir() { continue }
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".zip" && ext != ".rar" { continue }
		if ext == ".rar" && archive.IsSecondaryRARVolume(entry.Name()) { continue }
		if cb != nil { cb(ScanProgress{Phase:"inspecting", Completed:completed, Total:total, Current:entry.Name(), Message:"内部構造・日本語名を再帰確認中"}) }
		plan, planErr := o.planNameAt(root, entry.Name(), false)
		if planErr != nil { plans = append(plans, Plan{Name:entry.Name(), Source:filepath.Join(root,entry.Name()), NeedsReview:true, Error:planErr.Error()}) } else { plans = append(plans, plan) }
		completed++
		if cb != nil { cb(ScanProgress{Phase:"inspecting", Completed:completed, Total:total, Current:entry.Name(), Message:"解析済み"}) }
	}
	if cb != nil { cb(ScanProgress{Phase:"clustering", Completed:completed, Total:total, Message:"シリーズ表記を統合しています"}) }
	plans = clusterPlans(plans, o.Aliases())
	resolved := make(map[string]string, len(plans))
	for _, p := range plans { if p.Error == "" && p.Series != "" { resolved[p.Name] = p.Series } }
	o.mu.Lock(); o.resolved = resolved; o.mu.Unlock()
	for i := range plans { if plans[i].Error == "" { if err := o.applyDestination(&plans[i]); err != nil { plans[i].Error = err.Error(); plans[i].NeedsReview = true } } }
	sort.Slice(plans, func(i,j int) bool { return plans[i].Name < plans[j].Name })
	if cb != nil { cb(ScanProgress{Phase:"done", Completed:total, Total:total, Message:"解析完了"}) }
	return plans, nil
}
