package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type ReconcileProgress struct {
	Phase     string `json:"phase"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Message   string `json:"message,omitempty"`
}

type ReconcileProgressFunc func(ReconcileProgress)

func emitReconcileProgress(cb ReconcileProgressFunc, phase string, completed, total int, msg string) {
	if cb != nil {
		cb(ReconcileProgress{Phase: phase, Completed: completed, Total: total, Message: msg})
	}
}

func (o *Organizer) ReconcileScanMultiProgress(roots []string, outputRoot string, cb ReconcileProgressFunc) (ReconcileReport, error) {
	return o.ReconcileScanMultiProgressWithOptions(roots, outputRoot, ReconcileScanOptions{}, cb)
}

// ReconcileScanMultiProgressWithOptions intentionally treats existing libraries
// as already-normalized archives. It never opens ZIP/RAR members for naming.
// The filesystem archive filename is the primary identity source; parent folders
// are only supporting evidence. File bodies are read later only when SHA-256 is
// required for a same-size duplicate candidate.
func (o *Organizer) ReconcileScanMultiProgressWithOptions(roots []string, outputRoot string, opts ReconcileScanOptions, cb ReconcileProgressFunc) (ReconcileReport, error) {
	roots, outputRoot, err := normalizeReconcileRoots(roots, outputRoot)
	if err != nil {
		return ReconcileReport{}, err
	}

	total := 0
	emitReconcileProgress(cb, "counting", 0, 0, "対象ファイル数を確認中")
	for _, root := range roots {
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				if path == root {
					return walkErr
				}
				return nil
			}
			if path == root {
				return nil
			}
			if d.IsDir() {
				if skipReconcileDir(d, opts) {
					return filepath.SkipDir
				}
				return nil
			}
			if isReconcileArchiveName(d.Name()) {
				total++
			}
			return nil
		})
		if err != nil {
			return ReconcileReport{}, err
		}
	}

	msg := "ファイル名からシリーズ・巻番号を解析中（アーカイブ内部は参照しません）"
	if opts.IncludeQuarantine {
		msg = "通常ライブラリ＋隔離フォルダのファイル名を解析中（アーカイブ内部は参照しません）"
	}
	emitReconcileProgress(cb, "inspecting", 0, total, msg)

	raws := make([]reconcileRaw, 0, total)
	done := 0
	for _, root := range roots {
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				if path == root {
					return walkErr
				}
				return nil
			}
			if path == root {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			if d.IsDir() {
				if skipReconcileDir(d, opts) {
					return filepath.SkipDir
				}
				return nil
			}
			if !isReconcileArchiveName(d.Name()) {
				return nil
			}

			st, statErr := os.Lstat(path)
			if statErr != nil {
				raws = append(raws, reconcileRaw{path: path, root: root, rel: rel, name: d.Name(), err: statErr})
				done++
				emitReconcileProgress(cb, "inspecting", done, total, d.Name())
				return nil
			}
			if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
				done++
				emitReconcileProgress(cb, "inspecting", done, total, d.Name())
				return nil
			}

			n := inferExistingArchiveName(d.Name())
			workSeries := n.Parsed.Series
			edition := reconcileEdition(d.Name())
			series, confidence := parentSeriesEvidence(root, path, n.Parsed.Series, n.Parsed.Confidence, o.confidenceThreshold)
			if canonical, ok := aliasLookup(o.Aliases(), series); ok && seriesNameUsable(canonical) {
				series, confidence = canonical, 1
			}
			if !seriesNameUsable(series) {
				series, confidence = "", 0
			}
			if !seriesNameUsable(workSeries) {
				workSeries = series
			}
			raws = append(raws, reconcileRaw{
				path: path, root: root, rel: rel, name: d.Name(), series: series, workSeries: workSeries, edition: edition,
				confidence: confidence, size: st.Size(), modified: st.ModTime(),
				volume: n.Parsed.Volume, hasVolume: n.Parsed.HasVolume,
			})
			done++
			emitReconcileProgress(cb, "inspecting", done, total, d.Name())
			return nil
		})
		if err != nil {
			return ReconcileReport{}, err
		}
	}

	emitReconcileProgress(cb, "clustering", done, total, "ファイル名由来のシリーズ表記を統合中")
	plans := make([]Plan, len(raws))
	for i, r := range raws {
		if r.err != nil {
			plans[i] = Plan{Name: r.rel, Error: r.err.Error()}
		} else {
			plans[i] = Plan{Name: r.rel, Series: r.series, Confidence: r.confidence}
		}
	}
	plans = clusterPlans(plans, o.Aliases())

	items := make([]ReconcileItem, len(raws))
	for i, r := range raws {
		it := ReconcileItem{
			Source: r.path, LibraryRoot: r.root, Relative: r.rel, Series: plans[i].Series, WorkSeries: r.workSeries, Edition: r.edition,
			Confidence: r.confidence, Size: r.size, ModifiedAt: r.modified,
			Volume: r.volume, HasVolume: r.hasVolume,
		}
		if r.err != nil {
			it.Action, it.Reason = "error", r.err.Error()
			items[i] = it
			continue
		}
		if !seriesNameUsable(it.Series) {
			it.Action, it.Reason = "error", "series could not be determined safely from filename"
			items[i] = it
			continue
		}
		it.Destination = filepath.Join(outputRoot, it.Series, filepath.Base(r.path))
		if filepath.Clean(it.Destination) == filepath.Clean(r.path) {
			it.Action = "keep"
		} else {
			it.Action, it.Reason = "move", "normalize series folder from archive filename"
			if inQuarantine(r.root, r.path) {
				it.Reason = "restore/reclassify from quarantine using archive filename"
			}
		}
		items[i] = it
	}

	emitReconcileProgress(cb, "duplicates", 0, total, "同サイズ候補だけSHA-256を確認中")
	markExactDuplicatesProgress(outputRoot, items, cb, total)
	emitReconcileProgress(cb, "variants", total, total, "同一シリーズ・同一巻の更新日時を比較中")
	choices := resolveSameVolumeVariantsProgress(outputRoot, items)
	markDestinationConflicts(items)
	restoreConflictsWhoseDestinationWillBeVacated(items)

	sort.Slice(items, func(i, j int) bool {
		if items[i].Series != items[j].Series {
			return strings.ToLower(items[i].Series) < strings.ToLower(items[j].Series)
		}
		if items[i].HasVolume && items[j].HasVolume && items[i].Volume != items[j].Volume {
			return items[i].Volume < items[j].Volume
		}
		return items[i].Source < items[j].Source
	})

	report := ReconcileReport{Root: roots[0], Roots: roots, OutputRoot: outputRoot, Items: items, Choices: choices}
	for _, it := range items {
		report.Summary.Files++
		switch it.Action {
		case "keep":
			report.Summary.Keep++
		case "move":
			report.Summary.Move++
		case "duplicate":
			report.Summary.Duplicates++
		case "superseded":
			report.Summary.Superseded++
		case "conflict":
			report.Summary.Conflicts++
		case "review":
			report.Summary.Review++
		case "error":
			report.Summary.Errors++
		}
	}
	emitReconcileProgress(cb, "done", total, total, "解析完了（アーカイブ内部の読み取りなし）")
	return report, nil
}

func isReconcileArchiveName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".zip" {
		return true
	}
	return ext == ".rar" && !isSecondaryRARName(name)
}

func isSecondaryRARName(name string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name))), ".part2") || isSecondaryRARVolumeByParser(name)
}

func isSecondaryRARVolumeByParser(name string) bool {
	// Keep multipart detection centralized without opening the archive body.
	return archiveSecondaryRAR(name)
}

func archiveSecondaryRAR(name string) bool {
	// This tiny wrapper exists so the reconcile path's intent remains explicit.
	return isSecondaryRARVolumeName(name)
}

func markExactDuplicatesProgress(root string, items []ReconcileItem, cb ReconcileProgressFunc, total int) {
	type candidateKey struct {
		size int64
		work string
	}
	groups := map[candidateKey][]int{}
	for i, it := range items {
		if it.Action != "error" && it.Size >= 0 {
			key := candidateKey{size: it.Size, work: reconcileWorkKey(it)}
			groups[key] = append(groups[key], i)
		}
	}
	hashTotal := 0
	for _, idxs := range groups {
		if len(idxs) > 1 {
			hashTotal += len(idxs)
		}
	}
	if hashTotal == 0 {
		emitReconcileProgress(cb, "duplicates", total, total, "同サイズの重複候補なし")
		return
	}

	hashed := 0
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}
		hashGroups := map[string][]int{}
		for _, idx := range idxs {
			h, err := hashFile(items[idx].Source)
			hashed++
			emitReconcileProgress(cb, "duplicates", hashed, hashTotal, filepath.Base(items[idx].Source))
			if err != nil {
				continue
			}
			hashGroups[h] = append(hashGroups[h], idx)
		}
		for _, dups := range hashGroups {
			if len(dups) < 2 {
				continue
			}
			keeper := dups[0]
			for _, idx := range dups[1:] {
				keeperQ := inQuarantine(items[keeper].LibraryRoot, items[keeper].Source)
				idxQ := inQuarantine(items[idx].LibraryRoot, items[idx].Source)
				if (keeperQ && !idxQ) || (keeperQ == idxQ && items[idx].ModifiedAt.After(items[keeper].ModifiedAt)) {
					keeper = idx
				}
			}
			for _, idx := range dups {
				if idx == keeper {
					continue
				}
				items[idx].DuplicateOf = items[keeper].Source
				if inQuarantine(items[idx].LibraryRoot, items[idx].Source) {
					items[idx].Action = "keep"
					items[idx].Destination = items[idx].Source
					items[idx].Reason = "SHA-256 exact duplicate already quarantined"
					continue
				}
				items[idx].Action = "duplicate"
				items[idx].Destination = uniqueQuarantinePath(root, items[idx].Series, filepath.Base(items[idx].Source))
				items[idx].Reason = "SHA-256 exact duplicate; active copy preferred, then newer copy"
			}
		}
	}
}

func resolveSameVolumeVariantsProgress(root string, items []ReconcileItem) []ReconcileChoice {
	groups := map[string][]int{}
	for i, it := range items {
		if it.Action == "error" || it.Action == "duplicate" || !it.HasVolume || it.Series == "" {
			continue
		}
		key := reconcileWorkKey(it) + "#" + strconv.Itoa(it.Volume)
		groups[key] = append(groups[key], i)
	}
	choices := make([]ReconcileChoice, 0)
	seq := 0
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}
		sort.SliceStable(idxs, func(i, j int) bool { return items[idxs[i]].ModifiedAt.After(items[idxs[j]].ModifiedAt) })
		winner := idxs[0]
		if items[winner].ModifiedAt.After(items[idxs[1]].ModifiedAt) {
			items[winner].AutoSelected = true
			items[winner].Reason = "newest modified time among same series/volume"
			for _, idx := range idxs[1:] {
				items[idx].DuplicateOf = items[winner].Source
				if inQuarantine(items[idx].LibraryRoot, items[idx].Source) {
					items[idx].Action = "keep"
					items[idx].Destination = items[idx].Source
					items[idx].Reason = "older same-volume variant already quarantined"
					continue
				}
				items[idx].Action = "superseded"
				items[idx].Destination = uniqueQuarantinePath(root, items[idx].Series, filepath.Base(items[idx].Source))
				items[idx].Reason = "older variant of same series/volume; newer file selected"
			}
			continue
		}

		seq++
		id := fmt.Sprintf("volume-choice-%d", seq)
		choice := ReconcileChoice{ID: id, Series: reconcileWorkSeries(items[winner]), Volume: items[winner].Volume, HasVolume: true, Reason: "same series/volume has no unique newest file"}
		for _, idx := range idxs {
			items[idx].Action = "review"
			items[idx].ReviewGroup = id
			items[idx].Reason = "user selection required"
			choice.Candidates = append(choice.Candidates, items[idx])
		}
		choices = append(choices, choice)
	}
	return choices
}

func reconcileWorkSeries(it ReconcileItem) string {
	if seriesNameUsable(it.WorkSeries) {
		return it.WorkSeries
	}
	return it.Series
}

func reconcileWorkKey(it ReconcileItem) string {
	edition := it.Edition
	if edition == "" {
		edition = "standard"
	}
	return canonicalKey(reconcileWorkSeries(it)) + "#edition:" + edition
}

func reconcileEdition(filename string) string {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	compact := strings.NewReplacer(" ", "", "_", "", "-", "").Replace(name)
	switch {
	case strings.Contains(name, "セミカラー") || strings.Contains(compact, "semicolor") || strings.Contains(compact, "semicolour"):
		return "semi-color"
	case strings.Contains(name, "フルカラー") || strings.Contains(compact, "fullcolor") || strings.Contains(compact, "fullcolour"):
		return "full-color"
	case strings.Contains(name, "モノクロ") || strings.Contains(name, "白黒") || strings.Contains(compact, "monochrome") || strings.Contains(compact, "blackandwhite"):
		return "monochrome"
	case strings.Contains(name, "カラー") || strings.Contains(compact, "color") || strings.Contains(compact, "colour"):
		return "color"
	default:
		return "standard"
	}
}

func restoreConflictsWhoseDestinationWillBeVacated(items []ReconcileItem) {
	vacated := map[string]struct{}{}
	for _, it := range items {
		if (it.Action == "duplicate" || it.Action == "superseded") && filepath.Clean(it.Source) != filepath.Clean(it.Destination) {
			vacated[filepath.Clean(it.Source)] = struct{}{}
		}
	}
	for i := range items {
		it := &items[i]
		if it.Action != "conflict" {
			continue
		}
		if _, ok := vacated[filepath.Clean(it.Destination)]; ok {
			it.Action = "move"
			it.Reason = "destination will be vacated by older/duplicate copy before restore"
		}
	}
}

func (o *Organizer) ReconcileExecuteReportProgress(report ReconcileReport, selections map[string]string, cb ReconcileProgressFunc) (ReconcileResult, error) {
	if len(report.Roots) == 0 {
		return ReconcileResult{}, errors.New("reconcile report has no roots")
	}
	for _, choice := range report.Choices {
		selected := filepath.Clean(selections[choice.ID])
		if selections == nil || selected == "." || selected == "" {
			return ReconcileResult{}, fmt.Errorf("selection required for %s volume %d", choice.Series, choice.Volume)
		}
		found := false
		for i := range report.Items {
			it := &report.Items[i]
			if it.ReviewGroup != choice.ID {
				continue
			}
			if filepath.Clean(it.Source) == selected {
				found = true
				it.Destination = filepath.Join(report.OutputRoot, it.Series, filepath.Base(it.Source))
				if filepath.Clean(it.Source) == filepath.Clean(it.Destination) {
					it.Action = "keep"
				} else {
					it.Action = "move"
				}
				it.Reason = "selected by user"
			} else if inQuarantine(it.LibraryRoot, it.Source) {
				it.Action = "keep"
				it.Destination = it.Source
				it.DuplicateOf = selected
				it.Reason = "not selected; already quarantined"
			} else {
				it.Action = "superseded"
				it.Destination = uniqueQuarantinePath(report.OutputRoot, it.Series, filepath.Base(it.Source))
				it.DuplicateOf = selected
				it.Reason = "not selected for same series/volume"
			}
		}
		if !found {
			return ReconcileResult{}, fmt.Errorf("invalid selection for %s volume %d", choice.Series, choice.Volume)
		}
	}

	total := 0
	for _, it := range report.Items {
		if it.Action == "move" || it.Action == "duplicate" || it.Action == "superseded" {
			total++
		}
	}
	emitReconcileProgress(cb, "executing", 0, total, "整理を実行中")
	result := ReconcileResult{}
	done := 0

	executePhase := func(actions map[string]bool) {
		for _, it := range report.Items {
			if !actions[it.Action] {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(it.Destination), 0o750); err != nil {
				result.Errors = append(result.Errors, it.Relative+": "+err.Error())
				done++
				emitReconcileProgress(cb, "executing", done, total, it.Relative)
				continue
			}
			if filepath.Clean(it.Source) == filepath.Clean(it.Destination) {
				result.Skipped++
				done++
				emitReconcileProgress(cb, "executing", done, total, it.Relative)
				continue
			}
			if _, err := os.Lstat(it.Destination); err == nil {
				result.Skipped++
				done++
				emitReconcileProgress(cb, "executing", done, total, it.Relative)
				continue
			}
			if err := os.Rename(it.Source, it.Destination); err != nil {
				result.Errors = append(result.Errors, it.Relative+": "+err.Error())
				done++
				emitReconcileProgress(cb, "executing", done, total, it.Relative)
				continue
			}
			if it.Action == "duplicate" || it.Action == "superseded" {
				result.Quarantined++
			} else {
				result.Moved++
			}
			done++
			emitReconcileProgress(cb, "executing", done, total, it.Relative)
		}
	}

	// Vacate old/duplicate destinations first, then restore/move the selected copy.
	executePhase(map[string]bool{"duplicate": true, "superseded": true})
	executePhase(map[string]bool{"move": true})
	for _, it := range report.Items {
		if it.Action != "move" && it.Action != "duplicate" && it.Action != "superseded" {
			result.Skipped++
		}
	}
	for _, root := range report.Roots {
		removeEmptyLibraryDirs(root)
	}
	emitReconcileProgress(cb, "done", total, total, "整理完了")
	return result, nil
}
