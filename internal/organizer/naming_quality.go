package organizer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func seriesNameUsable(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || s == "Unknown" || !utf8.ValidString(s) { return false }
	for _, r := range s { if r == unicode.ReplacementChar { return false } }
	lower := strings.ToLower(s)
	switch lower {
	case "manga", "comic", "comics", "book", "books", "image", "images", "img", "imgs",
		"page", "pages", "scan", "scans", "screenshot", "screenshots", "preview", "previews", "sample", "samples", "raw", "source",
		"単ページ", "見開き", "見開きページ", "ページ", "画像", "画像ファイル", "表紙", "カバー", "スクリーンショット":
		return false
	}
	if volumeOrChapterOnly(s) { return false }
	meaningful := 0
	for _, r := range s { if unicode.IsLetter(r) || unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) { meaningful++ } }
	return meaningful >= 2
}

func volumeOrChapterOnly(s string) bool {
	n := strings.ToLower(strings.TrimSpace(s)); if n == "" { return true }
	for _, token := range []string{"volume", "vol", "chapter", "chap", "ch", "第", "巻", "卷", "話", "章"} { n = strings.ReplaceAll(n, token, "") }
	n = strings.Map(func(r rune) rune { if (r >= '0' && r <= '9') || (r >= '０' && r <= '９') || unicode.IsSpace(r) || strings.ContainsRune("-_.()[]【】（）#", r) { return -1 }; return r }, n)
	return strings.TrimSpace(n) == ""
}

func hasReplacementText(s string) bool {
	if !utf8.ValidString(s) { return true }
	for _, r := range s { if r == unicode.ReplacementChar { return true } }
	return false
}

func betterSeriesDisplay(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	au, bu := seriesNameUsable(a), seriesNameUsable(b)
	if au != bu { return au }
	if !au { return false }

	// When a derivative work is deliberately grouped with its parent series,
	// keep the parent title as the folder name even if the derivative has more
	// files. The full derivative title remains on each archive filename.
	ad, bd := hasSpinOffMarker(a), hasSpinOffMarker(b)
	if ad != bd { return !ad }

	ak, bk := canonicalKey(a), canonicalKey(b)
	if ak == bk { return displayPreference(a, b) }
	if bilingualEquivalent(a, b) {
		ar, br := len([]rune(a)), len([]rune(b)); if ar != br { return ar < br }; return displayPreference(a, b)
	}
	aj, bj := containsJapaneseText(a), containsJapaneseText(b)
	if aj != bj { return aj }
	return false
}

func displayPreference(a, b string) bool {
	aj, bj := containsJapaneseText(a), containsJapaneseText(b); if aj != bj { return aj }
	al, bl := containsLatin(a), containsLatin(b); if aj && al != bl { return !al }
	ar, br := len([]rune(a)), len([]rune(b)); if ar != br { return ar < br }
	return strings.ToLower(a) < strings.ToLower(b)
}
