package organizer

import (
	"path/filepath"
	"strings"

	"github.com/souten-yd/docExtractor/internal/classifier"
)

func archiveGroupBelongsToPlan(plan *Plan, group string) bool {
	if strings.TrimSpace(group) == "" {
		return true
	}
	groupSeries := classifier.Parse(group).Series
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
