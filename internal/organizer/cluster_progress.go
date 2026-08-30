package organizer

import (
	"sort"
	"strings"
)

// clusterPlansProgress preserves clusterPlans semantics while comparing each
// distinct series spelling only once. Existing libraries often contain many
// volumes with the same series string, so comparing every file pair was O(n²)
// in the number of archives and could spend many minutes after inspection had
// already shown 100%. Progress is reported in actual distinct-name comparisons.
func clusterPlansProgress(plans []Plan, persisted map[string]string, cb ReconcileProgressFunc) []Plan {
	if len(plans) < 2 && len(persisted) == 0 {
		emitReconcileProgress(cb, "clustering", 1, 1, "シリーズ統合対象なし")
		return plans
	}

	for i := range plans {
		if plans[i].Series == "" || plans[i].Error != "" || !seriesNameUsable(plans[i].Series) {
			continue
		}
		if canonical, ok := aliasLookup(persisted, plans[i].Series); ok {
			plans[i].Cluster = ClusterInfo{Canonical: canonical, Aliases: []string{plans[i].Series}, Reason: "saved alias", Score: 1}
			plans[i].Series = canonical
		}
	}

	// Keep first-seen order deterministic while collapsing repeated volumes.
	unique := make([]string, 0)
	uid := make(map[string]int)
	members := make([][]int, 0)
	for i := range plans {
		if plans[i].Series == "" || plans[i].Error != "" || !seriesNameUsable(plans[i].Series) {
			continue
		}
		key := plans[i].Series
		u, ok := uid[key]
		if !ok {
			u = len(unique)
			uid[key] = u
			unique = append(unique, key)
			members = append(members, nil)
		}
		members[u] = append(members[u], i)
	}

	if len(unique) == 0 {
		emitReconcileProgress(cb, "clustering", 1, 1, "有効なシリーズ名なし")
		return plans
	}

	parent := make([]int, len(unique))
	for i := range parent { parent[i] = i }
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x { parent[x] = find(parent[x]) }
		return parent[x]
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb { parent[rb] = ra }
	}

	total := len(unique) * (len(unique)-1) / 2
	if total == 0 { total = 1 }
	done := 0
	step := total / 500
	if step < 100 { step = 100 }
	reasons := make([]string, len(unique))
	scores := make([]float64, len(unique))
	for i := 0; i < len(unique); i++ {
		for j := i + 1; j < len(unique); j++ {
			score, reason := sameSeries(unique[i], unique[j])
			if score >= 0.90 {
				union(i, j)
				if reasons[i] == "" { reasons[i], scores[i] = reason, score }
				if reasons[j] == "" { reasons[j], scores[j] = reason, score }
			}
			done++
			if done == total || done%step == 0 {
				emitReconcileProgress(cb, "clustering", done, total, "シリーズ名比較中")
			}
		}
	}
	if len(unique) == 1 {
		emitReconcileProgress(cb, "clustering", 1, 1, "シリーズ名比較完了")
	}

	groups := map[int][]int{}
	for u := range unique {
		r := find(u)
		groups[r] = append(groups[r], members[u]...)
	}
	for _, idxs := range groups {
		if len(idxs) < 2 { continue }
		canonical := chooseCanonical(plans, idxs, persisted)
		aliases := make([]string, 0)
		seen := map[string]struct{}{}
		for _, idx := range idxs {
			if _, ok := seen[plans[idx].Series]; !ok {
				seen[plans[idx].Series] = struct{}{}
				aliases = append(aliases, plans[idx].Series)
			}
		}
		sort.Slice(aliases, func(i, j int) bool { return strings.ToLower(aliases[i]) < strings.ToLower(aliases[j]) })
		for _, idx := range idxs {
			old := plans[idx].Series
			plans[idx].Series = canonical
			plans[idx].Destination = replaceSeriesDir(plans[idx].Destination, old, canonical)
			plans[idx].Cluster.Canonical = canonical
			plans[idx].Cluster.Aliases = aliases
			if plans[idx].Cluster.Reason == "" {
				u := uid[old]
				if reasons[u] != "" {
					plans[idx].Cluster.Reason = reasons[u]
					plans[idx].Cluster.Score = scores[u]
				} else {
					plans[idx].Cluster.Reason = "series cluster"
					plans[idx].Cluster.Score = 0.95
				}
			}
		}
	}
	return plans
}
