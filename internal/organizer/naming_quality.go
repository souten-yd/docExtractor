package organizer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// seriesNameUsable rejects strings that are useful as volume/page metadata but
// unsafe as a library folder name. This is intentionally conservative: a bad
// candidate is ignored and the archive/file name can still be used as evidence.
func seriesNameUsable(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || s == "Unknown" || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r == unicode.ReplacementChar {
			return false
		}
	}
	lower := strings.ToLower(s)
	switch lower {
	case "manga", "comic", "comics", "book", "books", "image", "images", "img", "imgs",
		"page", "pages", "scan", "scans", "screenshot", "screenshots", "preview", "previews", "sample", "samples", "raw", "source",
		"単ページ", "見開き", "見開きページ", "ページ", "画像", "画像ファイル", "表紙", "カバー", "スクリーンショット":
		return false
	}
	if volumeOrChapterOnly(s) {
		return false
	}
	meaningful := 0
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			meaningful++
		}
	}
	return meaningful >= 2
}

func volumeOrChapterOnly(s string) bool {
	n := strings.ToLower(strings.TrimSpace(s))
	if n == "" {
		return true
	}
	// Remove the vocabulary allowed in a pure volume/chapter label. If no
	// letters remain afterwards, names such as "第13巻", "Vol. 8" and
	// "ch06" must not become a series folder.
	for _, token := range []string{"volume", "vol", "chapter", "chap", "ch", "第", "巻", "卷", "話", "章"} {
		n = strings.ReplaceAll(n, token, "")
	}
	n = strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= '０' && r <= '９') || unicode.IsSpace(r) || strings.ContainsRune("-_.()[]【】（）#", r) {
			return -1
		}
		return r
	}, n)
	return strings.TrimSpace(n) == ""
}

func hasReplacementText(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for _, r := range s {
		if r == unicode.ReplacementChar {
			return true
		}
	}
	return false
}

// betterSeriesDisplay is used only after two strings have already been judged
// to describe the same series. It prefers a stable, concise display name rather
// than stuffing aliases into the folder name.
func betterSeriesDisplay(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	au, bu := seriesNameUsable(a), seriesNameUsable(b)
	if au != bu {
		return au
	}
	if !au {
		return false
	}

	ak, bk := canonicalKey(a), canonicalKey(b)
	// Punctuation/case-only variants: keep the shorter readable spelling.
	if ak == bk {
		return displayPreference(a, b)
	}
	// A bilingual superset is useful as evidence/alias, but the folder should
	// use the concise title when the shorter title is literally embedded.
	if bilingualEquivalent(a, b) {
		ar, br := len([]rune(a)), len([]rune(b))
		if ar != br {
			return ar < br
		}
		return displayPreference(a, b)
	}

	// For fuzzy typo clusters do not blindly choose the shortest spelling.
	// Japanese is preferred for a Japanese library, otherwise keep the current
	// canonical and let frequency/persisted aliases decide.
	aj, bj := containsJapaneseText(a), containsJapaneseText(b)
	if aj != bj {
		return aj
	}
	return false
}

func displayPreference(a, b string) bool {
	aj, bj := containsJapaneseText(a), containsJapaneseText(b)
	if aj != bj {
		return aj
	}
	// If both contain Japanese, prefer the Japanese-only title over a verbose
	// Japanese+Latin alias such as "ブラックラグーン Black Lagoon".
	al, bl := containsLatin(a), containsLatin(b)
	if aj && al != bl {
		return !al
	}
	ar, br := len([]rune(a)), len([]rune(b))
	if ar != br {
		return ar < br
	}
	return strings.ToLower(a) < strings.ToLower(b)
}
