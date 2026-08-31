package organizer

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/souten-yd/docExtractor/internal/classifier"
)

var archiveCoverageRangeTail = regexp.MustCompile(`(?i)(?:[\s_\-]+)(?:第\s*)?(?:(?:vol(?:ume)?\.?|v|ch(?:apter)?\.?|chap\.?|c)\s*)?[0-9０-９]{1,5}(?:\s*(?:巻|卷|話|章))?\s*[-~〜～－ー–—]\s*(?:第\s*)?(?:(?:vol(?:ume)?\.?|v|ch(?:apter)?\.?|chap\.?|c)\s*)?[0-9０-９]{1,5}\s*(?:巻|卷|話|章)?\s*$`)

func archiveGroupBelongsToPlan(plan *Plan, group string) bool {
	if strings.TrimSpace(group) == "" {
		return true
	}
	// A high-confidence archive whose output groups all describe one contained
	// work should use the scan-selected canonical series. This covers one or many
	// volumes written as transliterations, chapter ranges, or spelling variants.
	// Mixed-series archives remain split by their independently parsed titles.
	if !plan.NeedsReview && seriesNameUsable(plan.Series) && archiveGroupsDescribeOneWork(plan.PreviewGroups) {
		return true
	}
	groupSeries := archiveGroupSeries(group)
	if !seriesNameUsable(groupSeries) {
		return false
	}
	candidates := append([]string{plan.Series}, plan.Cluster.Aliases...)
	for _, candidate := range candidates {
		if score, _ := sameSeries(candidate, groupSeries); score >= .86 {
			return true
		}
	}
	return false
}

func archiveGroupsDescribeOneWork(groups []string) bool {
	if len(groups) == 0 {
		return false
	}
	if len(groups) == 1 {
		return true
	}
	first := archiveGroupSeries(groups[0])
	if !seriesNameUsable(first) {
		return false
	}
	for _, group := range groups[1:] {
		series := archiveGroupSeries(group)
		if !seriesNameUsable(series) {
			return false
		}
		if score, _ := sameSeries(first, series); score < .86 {
			return false
		}
	}
	return true
}

func archiveGroupSeries(group string) string {
	// Parse already removes single volume/chapter suffixes and volume ranges.
	// Normalize the remaining common range forms locally so archive output
	// routing does not change existing-library reorganization semantics.
	group = archiveCoverageRangeTail.ReplaceAllString(strings.TrimSpace(group), "")
	return classifier.Parse(group).Series
}

func markArchiveOutputConflicts(plans []Plan) {
	owners := map[string][]int{}
	for i := range plans {
		if plans[i].Error != "" {
			continue
		}
		for _, target := range plans[i].PredictedOutputs {
			key := strings.ToLower(filepath.Clean(target))
			owners[key] = append(owners[key], i)
		}
	}
	for target, idxs := range owners {
		if len(idxs) < 2 {
			continue
		}
		for _, idx := range idxs {
			plans[idx].Evidence = appendUnique(plans[idx].Evidence, "overlapping volume output", 20)
			msg := "same-volume overlap detected; exact duplicates and older variants will be quarantined: " + filepath.Base(target)
			if plans[idx].PreviewWarning == "" {
				plans[idx].PreviewWarning = msg
			} else {
				plans[idx].PreviewWarning += "; " + msg
			}
		}
	}
}
