package organizer

import (
	"math"
	"strconv"
	"unicode"

	"github.com/souten-yd/docExtractor/internal/archive"
	"github.com/souten-yd/docExtractor/internal/classifier"
)

type nameEvidence struct {
	Parsed         classifier.Result
	Coverage       classifier.Coverage
	Source         string
	Evidence       []string
	Candidates     []string
	CandidateCount int
}

type scoredName struct {
	parsed   classifier.Result
	coverage classifier.Coverage
	kind     archive.NameCandidateKind
	name     string
	score    float64
}

func inferFromArchive(outerName, filename string) nameEvidence {
	outer := classifier.Parse(outerName)
	outerCoverage := classifier.ParseCoverage(outerName)
	best := nameEvidence{Parsed: outer, Coverage: outerCoverage, Source: "outer-filename", Evidence: []string{"outer filename"}}
	inspection, err := archive.InspectNames(filename)
	if err != nil || len(inspection.Candidates) == 0 {
		return best
	}

	items := make([]scoredName, 0, len(inspection.Candidates))
	groups := make(map[string][]int)
	for _, candidate := range inspection.Candidates {
		parsed := classifier.Parse(candidate.Name)
		if parsed.Series == "" || parsed.Series == "Unknown" {
			continue
		}
		score := parsed.Confidence + kindBonus(candidate.Kind)
		if containsJapanese(candidate.Name) {
			score += 0.16
		}
		idx := len(items)
		items = append(items, scoredName{parsed: parsed, coverage: classifier.ParseCoverage(candidate.Name), kind: candidate.Kind, name: candidate.Name, score: score})
		key := classifier.GroupKey(parsed.Series)
		groups[key] = append(groups[key], idx)
	}
	if len(items) == 0 {
		return best
	}

	for _, indexes := range groups {
		if len(indexes) < 2 {
			continue
		}
		bonus := math.Min(0.22, 0.07*float64(len(indexes)-1))
		for _, idx := range indexes {
			items[idx].score += bonus
		}
	}

	winner := items[0]
	for _, item := range items[1:] {
		if betterName(item, winner) {
			winner = item
		}
	}
	groupKey := classifier.GroupKey(winner.parsed.Series)
	winnerGroup := groups[groupKey]
	if winner.score < outer.Confidence+0.08 && !containsJapanese(winner.name) && len(winnerGroup) < 2 {
		return best
	}

	parsed := winner.parsed
	parsed.Confidence = math.Min(0.99, math.Max(parsed.Confidence, math.Min(0.99, winner.score)))

	coverages := make([]classifier.Coverage, 0, len(winnerGroup))
	volumes := make(map[int]struct{})
	for _, idx := range winnerGroup {
		coverages = append(coverages, items[idx].coverage)
		if items[idx].parsed.HasVolume {
			volumes[items[idx].parsed.Volume] = struct{}{}
		}
	}
	coverage := classifier.MergeCoverage(coverages)
	if coverage.Kind == classifier.CoverageUnknown {
		coverage = outerCoverage
	}
	multiVolume := coverage.VolumeStart > 0 && coverage.VolumeEnd > coverage.VolumeStart
	if len(volumes) > 1 {
		multiVolume = true
	}
	if multiVolume {
		parsed.Volume = 0
		parsed.HasVolume = false
	} else if outer.HasVolume && !parsed.HasVolume && classifier.GroupKey(outer.Series) == groupKey {
		parsed.Volume = outer.Volume
		parsed.HasVolume = true
	}

	candidateNames := make([]string, 0, 10)
	candidateCount := 0
	for _, item := range items {
		if classifier.GroupKey(item.parsed.Series) != groupKey {
			continue
		}
		candidateCount++
		candidateNames = appendUnique(candidateNames, item.name, 10)
	}
	if len(candidateNames) == 0 {
		candidateNames = append(candidateNames, winner.name)
		candidateCount = 1
	}
	evidence := []string{string(winner.kind), "archive metadata"}
	if containsJapanese(winner.name) {
		evidence = append(evidence, "Japanese title preferred")
	}
	if len(winnerGroup) >= 2 {
		evidence = append(evidence, "consensus from "+strconv.Itoa(len(winnerGroup))+" entries")
	}
	if multiVolume {
		evidence = append(evidence, "multiple volumes detected")
	}
	if coverage.ChapterStart > 0 {
		evidence = append(evidence, "chapter coverage detected")
	}
	if inspection.Truncated {
		evidence = append(evidence, "metadata scan truncated at safety limit")
	}
	return nameEvidence{Parsed: parsed, Coverage: coverage, Source: string(winner.kind), Evidence: evidence, Candidates: candidateNames, CandidateCount: candidateCount}
}

func kindBonus(kind archive.NameCandidateKind) float64 {
	switch kind {
	case archive.CandidateNestedArchive:
		return 0.20
	case archive.CandidateTopDirectory:
		return 0.16
	case archive.CandidateNamedImage:
		return 0.07
	default:
		return 0
	}
}

func betterName(a, b scoredName) bool {
	if math.Abs(a.score-b.score) > 0.0001 {
		return a.score > b.score
	}
	aj, bj := containsJapanese(a.name), containsJapanese(b.name)
	if aj != bj {
		return aj
	}
	return kindBonus(a.kind) > kindBonus(b.kind)
}

func containsJapanese(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana) || (r >= 0x3400 && r <= 0x9fff) {
			return true
		}
	}
	return false
}

func appendUnique(dst []string, s string, limit int) []string {
	for _, existing := range dst {
		if existing == s {
			return dst
		}
	}
	if len(dst) < limit {
		return append(dst, s)
	}
	return dst
}
