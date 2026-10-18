package organizer

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Ratings are boundary annotations only. Stars inside a title remain identity.
func splitSeriesRating(s string) (string, string) {
	r := []rune(strings.TrimSpace(s))
	start, end := 0, len(r)
	for start < end && ratingBoundary(r[start]) {
		start++
	}
	for end > start && ratingBoundary(r[end-1]) {
		end--
	}
	marks := string(r[:start]) + string(r[end:])
	if ratingCount(marks) == 0 {
		return strings.TrimSpace(s), ""
	}
	return strings.TrimSpace(string(r[start:end])), strings.TrimSpace(marks)
}

func ratingBoundary(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune("⭐🌟★☆\ufe0e\ufe0f", r)
}

func ratingCount(s string) int {
	n := 0
	for _, r := range s {
		if strings.ContainsRune("⭐🌟★☆", r) {
			n++
		}
	}
	return n
}

// Keep the strongest existing rating, never sum ratings across files/scans.
// Lexical tie breaking makes traversal order immaterial; preserve the glyphs.
func withSeriesRating(title string, sources ...string) string {
	title, best := splitSeriesRating(title)
	for _, source := range sources {
		_, marks := splitSeriesRating(source)
		if ratingCount(marks) > ratingCount(best) || (ratingCount(marks) == ratingCount(best) && marks > best) {
			best = marks
		}
	}
	if best == "" {
		return title
	}
	return best + " " + title
}

// New volumes must reuse an existing rated output folder. Only exact normalized
// titles are used here: no fuzzy inference from unrelated directory names.
func existingSeriesDisplay(root, series string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return series
	}
	best := ""
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := entry.Name()
		if canonicalKey(name) != canonicalKey(series) {
			continue
		}
		if best == "" || ratingCountOf(name) > ratingCountOf(best) || (ratingCountOf(name) == ratingCountOf(best) && name < best) {
			best = name
		}
	}
	if best == "" {
		return series
	}
	// Reuse the exact spelling (including suffix ratings and variation selectors).
	if ratingCountOf(best) >= ratingCountOf(series) {
		return filepath.Base(best)
	}
	return withSeriesRating(best, series)
}

func ratingCountOf(s string) int { _, marks := splitSeriesRating(s); return ratingCount(marks) }
