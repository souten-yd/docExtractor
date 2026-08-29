package classifier

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Result is the best-effort interpretation of an archive filename.
type Result struct {
	Series     string   `json:"series"`
	Author     string   `json:"author,omitempty"`
	Volume     int      `json:"volume,omitempty"`
	HasVolume  bool     `json:"has_volume"`
	Confidence float64  `json:"confidence"`
	Reasons    []string `json:"reasons,omitempty"`
}

var (
	volumeJP        = regexp.MustCompile(`(?i)\s*第\s*([0-9０-９]{1,4})\s*(?:巻|卷)\b?`)
	volumeVol       = regexp.MustCompile(`(?i)(?:^|[\s_\-])(?:vol(?:ume)?\.?|v)\s*([0-9０-９]{1,4})(?:[a-z](?:[0-9]+)?)?(?:\b|$)`)
	volumeTail      = regexp.MustCompile(`(?:^|[\s_\-])([0-9０-９]{1,3})(?:\s*(?:巻|卷))?(?:\s*(?:特装版|通常版|限定版|完))?\s*$`)
	volumeRangeTail = regexp.MustCompile(`(?i)(?:\s|^)(?:第\s*)?(?:vol(?:ume)?\.?\s*|v\s*)?[0-9０-９]{1,4}\s*[-~～〜－]\s*[0-9０-９]{1,4}\s*(?:巻|卷)?(?:[a-z](?:[0-9]+)?)?\s*$`)
	editionTail     = regexp.MustCompile(`(?i)\s*(?:\[[^\]]*(?:DL|電子|Digital)[^\]]*\]|\([^\)]*(?:DL|電子|Digital)[^\)]*\)|(?:特装版|通常版|限定版|単行本|コミックス?))\s*$`)
	chapterTail     = regexp.MustCompile(`(?i)(?:[\s_\-]+)(?:(?:ch(?:apter)?\.?|chap\.?)\s*[0-9０-９]{1,5}|第\s*[0-9０-９]{1,5}\s*話)\s*$`)
	pageTail        = regexp.MustCompile(`(?i)(?:\s*[-_]?\s*)(?:p|pg|page)\s*0*[0-9０-９]{1,6}(?:\s*[\[【（(][^\]】）)]{1,80}[\]】）)])?\s*$`)
	spaces          = regexp.MustCompile(`\s+`)
	variantBracket  = regexp.MustCompile(`(?i)\s*[\[【（(]\s*(?:フルカラー|カラー|セミカラー|モノクロ|白黒|別スキャン|別scan|別炊|再スキャン|再scan|修正版|修正|fix(?:ed)?|番外編?|外伝|特典|おまけ|bonus|extra|単ページ|見開き(?:結合)?)(?:版)?\s*[\]】）)]\s*$`)
	variantPlain    = regexp.MustCompile(`(?i)(?:\s|[_\-])+\s*(?:フルカラー版?|カラー版?|セミカラー版?|モノクロ版?|白黒版?|別スキャン|別scan|別炊|再スキャン|再scan|修正版?|fix(?:ed)?|番外編?|外伝|特典|おまけ|bonus|extra|単ページ|見開き(?:結合)?)(?:\s*版)?\s*$`)
)

var metadataTags = map[string]struct{}{
	"一般コミック": {}, "コミック": {}, "漫画": {}, "マンガ": {}, "manga": {},
	"digital": {}, "電子版": {}, "電子書籍": {}, "dl版": {}, "dl": {}, "raw": {}, "scan": {},
	"complete": {}, "完結": {}, "web": {},
}

var knownExtensions = map[string]struct{}{
	".zip": {}, ".rar": {}, ".cbz": {}, ".cbr": {}, ".7z": {},
	".jpg": {}, ".jpeg": {}, ".png": {}, ".webp": {}, ".avif": {}, ".gif": {}, ".bmp": {}, ".tif": {}, ".tiff": {},
}

func Parse(filename string) Result {
	base := filepath.Base(filename)
	if ext := strings.ToLower(filepath.Ext(base)); ext != "" {
		if _, ok := knownExtensions[ext]; ok {
			base = strings.TrimSuffix(base, filepath.Ext(base))
		}
	}
	base = strings.TrimSpace(base)
	result := Result{Confidence: 0.45}

	// Leading bracket blocks are metadata/creator annotations, not title text.
	// Parse each bracket style independently so an author such as
	// [盧恩＆雪笠(Friendly Land)×早秋] is not broken by parentheses inside [].
	var authorCandidates []string
	for {
		tag, rest, ok := consumeLeadingBracket(base)
		if !ok {
			break
		}
		base = rest
		if isMetadataTag(tag) {
			result.Reasons = append(result.Reasons, "metadata-prefix")
			continue
		}
		authorCandidates = append(authorCandidates, tag)
	}
	if len(authorCandidates) > 0 {
		result.Author = authorCandidates[len(authorCandidates)-1]
		result.Confidence += 0.12
		result.Reasons = append(result.Reasons, "author-prefix")
	}

	for {
		cleaned := strings.TrimSpace(editionTail.ReplaceAllString(base, ""))
		cleaned = stripSeriesVariantTail(cleaned)
		if cleaned == base {
			break
		}
		base = cleaned
		result.Reasons = append(result.Reasons, "edition-or-variant-suffix")
	}

	// Image/chapter metadata inside old archives must not leak into the series
	// folder name. Keep the original string for ParseCoverage callers; Parse
	// itself only removes trailing evidence noise before finding the volume.
	for {
		cleaned := strings.TrimSpace(pageTail.ReplaceAllString(base, ""))
		cleaned = strings.TrimSpace(chapterTail.ReplaceAllString(cleaned, ""))
		if cleaned == base {
			break
		}
		base = cleaned
		result.Reasons = append(result.Reasons, "trailing-page-or-chapter-metadata")
	}

	var volumeStart = -1
	if loc := volumeJP.FindStringSubmatchIndex(base); loc != nil {
		result.Volume, result.HasVolume = parseDigits(base[loc[2]:loc[3]])
		volumeStart = loc[0]
		if result.HasVolume {
			result.Confidence += 0.30
			result.Reasons = append(result.Reasons, "explicit-japanese-volume")
		}
	} else if loc := volumeVol.FindStringSubmatchIndex(base); loc != nil {
		result.Volume, result.HasVolume = parseDigits(base[loc[2]:loc[3]])
		volumeStart = loc[0]
		if result.HasVolume {
			result.Confidence += 0.27
			result.Reasons = append(result.Reasons, "explicit-vol-volume")
		}
	} else if loc := volumeTail.FindStringSubmatchIndex(base); loc != nil {
		result.Volume, result.HasVolume = parseDigits(base[loc[2]:loc[3]])
		volumeStart = loc[0]
		if result.HasVolume {
			result.Confidence += 0.16
			result.Reasons = append(result.Reasons, "numeric-tail-volume")
		}
	} else if loc := volumeRangeTail.FindStringIndex(base); loc != nil {
		// A range is useful coverage evidence but is not one concrete volume.
		volumeStart = loc[0]
		result.Reasons = append(result.Reasons, "volume-range")
	}

	series := base
	if volumeStart >= 0 {
		series = base[:volumeStart]
	}
	series = cleanupSeries(series)
	series = stripSeriesVariantTail(series)
	series = collapseRepeatedSeries(series)
	if series != "" {
		result.Series = SafeFolderName(series)
		result.Confidence += 0.10
		result.Reasons = append(result.Reasons, "non-empty-series")
	} else {
		result.Series = SafeFolderName(base)
		result.Confidence -= 0.20
	}
	if result.Confidence > 0.99 {
		result.Confidence = 0.99
	}
	if result.Confidence < 0 {
		result.Confidence = 0
	}
	return result
}

func consumeLeadingBracket(s string) (tag, rest string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", s, false
	}
	var close rune
	switch []rune(s)[0] {
	case '[':
		close = ']'
	case '【':
		close = '】'
	case '（':
		close = '）'
	case '(':
		close = ')'
	default:
		return "", s, false
	}
	runes := []rune(s)
	for i := 1; i < len(runes); i++ {
		if runes[i] == close {
			tag = strings.TrimSpace(string(runes[1:i]))
			if tag == "" {
				return "", s, false
			}
			return tag, strings.TrimSpace(string(runes[i+1:])), true
		}
	}
	return "", s, false
}

func stripSeriesVariantTail(s string) string {
	for {
		before := strings.TrimSpace(s)
		after := strings.TrimSpace(variantBracket.ReplaceAllString(before, ""))
		after = strings.TrimSpace(variantPlain.ReplaceAllString(after, ""))
		if after == before {
			return after
		}
		s = after
	}
}

func collapseRepeatedSeries(s string) string {
	s = strings.TrimSpace(s)
	// Collapse only an exact duplicated half. This avoids shortening legitimate
	// repeated words inside a title while fixing "転生したら剣でした 転生したら剣でした".
	for i, r := range s {
		if !unicode.IsSpace(r) {
			continue
		}
		left := strings.TrimSpace(s[:i])
		right := strings.TrimSpace(s[i:])
		if left != "" && left == right {
			return left
		}
	}
	return s
}

func isMetadataTag(s string) bool {
	n := strings.ToLower(strings.TrimSpace(s))
	if _, ok := metadataTags[n]; ok {
		return true
	}
	for tag := range metadataTags {
		if strings.Contains(n, tag) && (strings.Contains(n, "版") || strings.Contains(n, "comic") || strings.Contains(n, "コミック")) {
			return true
		}
	}
	return false
}

func cleanupSeries(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "-–—_・. ")
	s = spaces.ReplaceAllString(s, " ")
	return s
}

func SafeFolderName(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return ' '
		default:
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}
	}, s)
	s = spaces.ReplaceAllString(strings.TrimSpace(s), " ")
	s = strings.Trim(s, ". ")
	if len([]rune(s)) > 180 {
		rs := []rune(s)
		s = strings.TrimSpace(string(rs[:180]))
	}
	if s == "" {
		return "Unknown"
	}
	return s
}

func GroupKey(s string) string {
	n := stripSeriesVariantTail(strings.TrimSpace(s))
	n = collapseRepeatedSeries(n)
	n = strings.ToLower(n)
	n = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '_', '-', '‐', '‑', '–', '—', '・', '.', '．':
			return -1
		default:
			return r
		}
	}, n)
	return n
}

func parseDigits(s string) (int, bool) {
	s = strings.Map(func(r rune) rune {
		if r >= '０' && r <= '９' {
			return '0' + (r - '０')
		}
		return r
	}, s)
	n, err := strconv.Atoi(s)
	return n, err == nil
}
