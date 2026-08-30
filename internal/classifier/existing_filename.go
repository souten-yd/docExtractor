package classifier

import (
	"path/filepath"
	"regexp"
	"strings"
)

// ParseExistingFilename parses archives that have already been unpacked,
// repacked and named by a previous organizer. Keep this deliberately close to
// the v0.2.11 filename behavior so changes made for raw/nested archive
// inspection cannot change an existing library's series identity.
//
// The only intentional extension is that simple edition/side-material suffixes
// are removed from the series identity, so color/alternate scans/extra chapters
// are kept with the main series.
var (
	existingVolumeVol  = regexp.MustCompile(`(?i)(?:^|[\s_\-])(?:vol(?:ume)?\.?|v)\s*([0-9０-９]{1,4})(?:\b|$)`)
	existingVolumeTail = regexp.MustCompile(`(?:^|[\s_\-])([0-9０-９]{1,3})(?:\s*(?:巻|卷))?(?:\s*(?:特装版|通常版|限定版|完))?\s*$`)
	existingVariantBracket = regexp.MustCompile(`(?i)\s*[\[【（(]\s*(?:フルカラー|カラー|セミカラー|モノクロ|白黒|別スキャン|別scan|別炊|再スキャン|再scan|修正版?|fix(?:ed)?|番外編?|外伝|特典|おまけ|bonus|extra|単ページ|見開き(?:結合)?)(?:版)?\s*[\]】）)]\s*$`)
	existingVariantPlain = regexp.MustCompile(`(?i)(?:\s|[_\-])+\s*(?:フルカラー版?|カラー版?|セミカラー版?|モノクロ版?|白黒版?|別スキャン|別scan|別炊|再スキャン|再scan|修正版?|fix(?:ed)?|番外編?|外伝|特典|おまけ|bonus|extra|単ページ|見開き(?:結合)?)(?:\s*版)?\s*$`)
)

func ParseExistingFilename(filename string) Result {
	base := filepath.Base(filename)
	if ext := strings.ToLower(filepath.Ext(base)); ext != "" {
		if _, ok := knownExtensions[ext]; ok {
			base = strings.TrimSuffix(base, filepath.Ext(base))
		}
	}
	base = strings.TrimSpace(base)
	result := Result{Confidence: 0.45}

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
		cleaned = stripExistingVariantTail(cleaned)
		if cleaned == base {
			break
		}
		base = cleaned
	}

	// Retain the v0.2.11 handling for page/chapter metadata.
	for {
		cleaned := strings.TrimSpace(pageTail.ReplaceAllString(base, ""))
		cleaned = strings.TrimSpace(chapterTail.ReplaceAllString(cleaned, ""))
		if cleaned == base {
			break
		}
		base = cleaned
		result.Reasons = append(result.Reasons, "trailing-page-or-chapter-metadata")
	}

	volumeStart := -1
	if loc := volumeJP.FindStringSubmatchIndex(base); loc != nil {
		result.Volume, result.HasVolume = parseDigits(base[loc[2]:loc[3]])
		volumeStart = loc[0]
		if result.HasVolume {
			result.Confidence += 0.30
			result.Reasons = append(result.Reasons, "explicit-japanese-volume")
		}
	} else if loc := existingVolumeVol.FindStringSubmatchIndex(base); loc != nil {
		result.Volume, result.HasVolume = parseDigits(base[loc[2]:loc[3]])
		volumeStart = loc[0]
		if result.HasVolume {
			result.Confidence += 0.27
			result.Reasons = append(result.Reasons, "explicit-vol-volume")
		}
	} else if loc := existingVolumeTail.FindStringSubmatchIndex(base); loc != nil {
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
	series = stripExistingVariantTail(series)
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

func stripExistingVariantTail(s string) string {
	for {
		before := strings.TrimSpace(s)
		after := strings.TrimSpace(existingVariantBracket.ReplaceAllString(before, ""))
		after = strings.TrimSpace(existingVariantPlain.ReplaceAllString(after, ""))
		if after == before {
			return after
		}
		s = after
	}
}
