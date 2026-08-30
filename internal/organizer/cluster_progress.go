package organizer

import (
	"sort"
	"strings"
)

type preparedSeries struct {
	raw     string
	key     string
	runes   []rune
	spinOff bool
	jp      bool
	latin   bool
}

func prepareSeries(s string) preparedSeries {
	key := canonicalKey(s)
	return preparedSeries{
		raw: s,
		key: key,
		runes: []rune(key),
		spinOff: hasSpinOffMarker(s),
		jp: containsJapaneseText(s),
		latin: containsLatin(s),
	}
}

func bilingualEquivalentPrepared(a, b preparedSeries) bool {
	if a.spinOff || b.spinOff { return false }
	short, long := a, b
	if len(short.runes) > len(long.runes) { short, long = long, short }
	if len(short.runes) < 4 || short.key == long.key { return false }
	var rest string
	switch {
	case strings.HasPrefix(long.key, short.key): rest = strings.TrimPrefix(long.key, short.key)
	case strings.HasSuffix(long.key, short.key): rest = strings.TrimSuffix(long.key, short.key)
	default: return false
	}
	if rest == "" { return false }
	if short.jp && !short.latin { return containsLatin(rest) }
	if short.latin && !short.jp { return containsJapaneseText(rest) }
	return false
}

func sameSeriesPrepared(a, b preparedSeries) (float64, string) {
	if a.key == "" || b.key == "" { return 0, "" }
	if a.key == b.key { return 1, "normalized exact match" }
	if bilingualEquivalentPrepared(a, b) { return 0.98, "bilingual title alias" }
	if a.spinOff || b.spinOff { return 0, "" }
	la, lb := len(a.runes), len(b.runes)
	// Cheap length bound before Levenshtein. If the length difference alone is
	// already larger than the permitted edit budget, similarity cannot reach
	// the acceptance threshold, so skip the allocation/DP work entirely.
	minThreshold := 0.94
	if la >= 10 && lb >= 10 { minThreshold = 0.90 }
	maxLen := la
	if lb > maxLen { maxLen = lb }
	diff := la - lb
	if diff < 0 { diff = -diff }
	if maxLen == 0 || 1-float64(diff)/float64(maxLen) < minThreshold { return 0, "" }
	s := levenshteinSimilarity(a.runes, b.runes)
	if la >= 10 && lb >= 10 && s >= 0.90 { return s, "minor filename variation" }
	if la >= 8 && lb >= 8 && s >= 0.94 { return s, "minor filename variation" }
	return 0, ""
}

func buildAliasIndex(persisted map[string]string) map[string]string {
	if len(persisted) == 0 { return nil }
	out := make(map[string]string, len(persisted))
	for alias, canonical := range persisted {
		key := canonicalKey(alias)
		if key != "" { out[key] = canonical }
	}
	return out
}

func chooseCanonicalPrepared(plans []Plan, idxs []int, aliasIndex map[string]string, planKeys []string) string {
	for _, idx := range idxs {
		if c, ok := aliasIndex[planKeys[idx]]; ok && seriesNameUsable(c) { return c }
	}
	counts := make(map[string]int, len(idxs))
	for _, idx := range idxs {
		if seriesNameUsable(plans[idx].Series) { counts[planKeys[idx]]++ }
	}
	best, bestCount := "", -1
	for _, idx := range idxs {
		cand := plans[idx].Series
		if !seriesNameUsable(cand) { continue }
		count := counts[planKeys[idx]]
		if best == "" || count > bestCount || (count == bestCount && betterSeriesDisplay(cand, best)) {
			best, bestCount = cand, count
		}
	}
	return best
}

// clusterPlansProgress preserves clusterPlans semantics while comparing each
// distinct series spelling only once. Existing libraries often contain many
// volumes with the same series string, so comparing every file pair was O(n²)
// in the number of archives and could spend many minutes after inspection had
// already shown 100%. Normalized keys/runes/script flags are cached once per
// distinct series so the hot comparison loop does not repeatedly allocate them.
func clusterPlansProgress(plans []Plan, persisted map[string]string, cb ReconcileProgressFunc) []Plan {
	if len(plans) < 2 && len(persisted) == 0 {
		emitReconcileProgress(cb, "clustering", 1, 1, "シリーズ統合対象なし")
		return plans
	}

	aliasIndex := buildAliasIndex(persisted)
	for i := range plans {
		if plans[i].Series == "" || plans[i].Error != "" || !seriesNameUsable(plans[i].Series) { continue }
		if canonical, ok := aliasIndex[canonicalKey(plans[i].Series)]; ok {
			plans[i].Cluster = ClusterInfo{Canonical: canonical, Aliases: []string{plans[i].Series}, Reason: "saved alias", Score: 1}
			plans[i].Series = canonical
		}
	}

	unique := make([]preparedSeries, 0)
	uid := make(map[string]int)
	members := make([][]int, 0)
	planKeys := make([]string, len(plans))
	for i := range plans {
		if plans[i].Series == "" || plans[i].Error != "" || !seriesNameUsable(plans[i].Series) { continue }
		key := plans[i].Series
		u, ok := uid[key]
		if !ok {
			u = len(unique)
			uid[key] = u
			unique = append(unique, prepareSeries(key))
			members = append(members, nil)
		}
		planKeys[i] = unique[u].key
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
			score, reason := sameSeriesPrepared(unique[i], unique[j])
			if score >= 0.90 {
				union(i, j)
				if reasons[i] == "" { reasons[i], scores[i] = reason, score }
				if reasons[j] == "" { reasons[j], scores[j] = reason, score }
			}
			done++
			if done == total || done%step == 0 { emitReconcileProgress(cb, "clustering", done, total, "シリーズ名比較中") }
		}
	}
	if len(unique) == 1 { emitReconcileProgress(cb, "clustering", 1, 1, "シリーズ名比較完了") }

	groups := make(map[int][]int, len(unique))
	for u := range unique {
		r := find(u)
		groups[r] = append(groups[r], members[u]...)
	}
	for _, idxs := range groups {
		if len(idxs) < 2 { continue }
		canonical := chooseCanonicalPrepared(plans, idxs, aliasIndex, planKeys)
		aliases := make([]string, 0)
		seen := make(map[string]struct{}, len(idxs))
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
