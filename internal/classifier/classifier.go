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
	authorPrefix = regexp.MustCompile(`^\s*[\[【]([^\]】]+)[\]】]\s*`)
	volumeJP     = regexp.MustCompile(`(?i)\s*第\s*([0-9０-９]{1,4})\s*(?:巻|卷)\b?`)
	volumeVol    = regexp.MustCompile(`(?i)(?:^|[\s_\-])(?:vol(?:ume)?\.?|v)\s*([0-9０-９]{1,4})(?:\b|$)`)
	volumeTail   = regexp.MustCompile(`(?:^|[\s_\-])([0-9０-９]{1,3})(?:\s*(?:巻|卷))?(?:\s*(?:特装版|通常版|限定版|完))?\s*$`)
	editionTail  = regexp.MustCompile(`(?i)\s*(?:\[[^\]]*(?:DL|電子|Digital)[^\]]*\]|\([^\)]*(?:DL|電子|Digital)[^\)]*\)|(?:特装版|通常版|限定版))\s*$`)
	spaces       = regexp.MustCompile(`\s+`)
)

func Parse(filename string) Result {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	base = strings.TrimSpace(base)
	result := Result{Confidence: 0.45}

	if m := authorPrefix.FindStringSubmatchIndex(base); m != nil {
		result.Author = strings.TrimSpace(base[m[2]:m[3]])
		base = strings.TrimSpace(base[m[1]:])
		result.Confidence += 0.12
		result.Reasons = append(result.Reasons, "author-prefix")
	}

	// Remove only well-known edition suffixes. Other bracket text may be part of the title.
	for {
		cleaned := editionTail.ReplaceAllString(base, "")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == base {
			break
		}
		base = cleaned
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
	}

	series := base
	if volumeStart >= 0 {
		series = base[:volumeStart]
	}
	series = cleanupSeries(series)
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
	return strings.ToLower(spaces.ReplaceAllString(strings.TrimSpace(s), " "))
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
