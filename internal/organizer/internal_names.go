package organizer

import (
	"math"
	"strings"
	"unicode"

	"github.com/souten-yd/docExtractor/internal/archive"
	"github.com/souten-yd/docExtractor/internal/classifier"
)

type nameEvidence struct {
	Parsed     classifier.Result
	Source     string
	Evidence   []string
	Candidates []string
}

type scoredName struct {
	parsed classifier.Result
	kind   archive.NameCandidateKind
	name   string
	score  float64
}

func inferFromArchive(outerName, filename string) nameEvidence {
	outer := classifier.Parse(outerName)
	best := nameEvidence{Parsed: outer, Source: "outer-filename", Evidence: []string{"outer filename"}}
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
		items = append(items, scoredName{parsed: parsed, kind: candidate.Kind, name: candidate.Name, score: score})
		groups[classifier.GroupKey(parsed.Series)] = append(groups[classifier.GroupKey(parsed.Series)], idx)
	}
	if len(items) == 0 {
		return best
	}

	// Consensus is important for archives containing one nested archive per
	// volume. Repeated normalized series names are stronger evidence than any
	// single filename, and Japanese candidates are preferred when equivalent.
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
	// Internal metadata must beat the outer filename by a useful margin unless
	// it carries Japanese title evidence or multi-entry consensus.
	outerScore := outer.Confidence
	if winner.score < outerScore+0.08 && !containsJapanese(winner.name) && len(groups[classifier.GroupKey(winner.parsed.Series)]) < 2 {
		return best
	}

	parsed := winner.parsed
	parsed.Confidence = math.Min(0.99, math.Max(parsed.Confidence, math.Min(0.99, winner.score)))
	if outer.HasVolume && !parsed.HasVolume && classifier.GroupKey(outer.Series) == classifier.GroupKey(parsed.Series) {
		parsed.Volume = outer.Volume
		parsed.HasVolume = true
	}

	candidateNames := make([]string, 0, 6)
	for _, item := range items {
		if classifier.GroupKey(item.parsed.Series) != classifier.GroupKey(winner.parsed.Series) {
			continue
		}
		candidateNames = appendUnique(candidateNames, item.name, 6)
	}
	if len(candidateNames) == 0 {
		candidateNames = append(candidateNames, winner.name)
	}
	evidence := []string{string(winner.kind), "archive metadata"}
	if containsJapanese(winner.name) {
		evidence = append(evidence, "Japanese title preferred")
	}
	if n := len(groups[classifier.GroupKey(winner.parsed.Series)]); n >= 2 {
		evidence = append(evidence, "consensus from "+itoaSmall(n)+" entries")
	}
	return nameEvidence{Parsed: parsed, Source: string(winner.kind), Evidence: evidence, Candidates: candidateNames}
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

func itoaSmall(n int) string {
	if n >= 0 && n <= 9 {
		return string(rune('0' + n))
	}
	return strings.Repeat("+", 0) + "many"
}
