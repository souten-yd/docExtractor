package classifier

import (
	"regexp"
	"strconv"
	"strings"
)

type CoverageKind string

const (
	CoverageUnknown CoverageKind = "unknown"
	CoverageVolume  CoverageKind = "volume"
	CoverageChapter CoverageKind = "chapter"
	CoverageMixed   CoverageKind = "mixed"
)

type Coverage struct {
	Kind         CoverageKind `json:"kind"`
	VolumeStart  int          `json:"volume_start,omitempty"`
	VolumeEnd    int          `json:"volume_end,omitempty"`
	ChapterStart int          `json:"chapter_start,omitempty"`
	ChapterEnd   int          `json:"chapter_end,omitempty"`
	Label        string       `json:"label,omitempty"`
}

var (
	jpVolumeRange = regexp.MustCompile(`(?i)第?\s*([0-9０-９]{1,4})\s*(?:巻|卷)?\s*[-~〜～－ー–—]\s*第?\s*([0-9０-９]{1,4})\s*(?:巻|卷)`)
	volRange      = regexp.MustCompile(`(?i)(?:vol(?:ume)?\.?|v)?\s*([0-9０-９]{1,4})\s*[-~〜～－ー–—]\s*(?:vol(?:ume)?\.?|v)?\s*([0-9０-９]{1,4})(?:\s*(?:巻|卷)|\b)`)
	jpChapterRange = regexp.MustCompile(`(?i)第?\s*([0-9０-９]{1,5})\s*(?:話|章)\s*[-~〜～－ー–—]\s*第?\s*([0-9０-９]{1,5})\s*(?:話|章)`)
	chRange       = regexp.MustCompile(`(?i)(?:ch(?:apter)?\.?|c)\s*([0-9０-９]{1,5})\s*[-~〜～－ー–—]\s*(?:ch(?:apter)?\.?|c)?\s*([0-9０-９]{1,5})`)
	jpChapterOne  = regexp.MustCompile(`(?i)第\s*([0-9０-９]{1,5})\s*(?:話|章)`)
	chOne         = regexp.MustCompile(`(?i)(?:^|[\s_\-])(?:ch(?:apter)?\.?|c)\s*([0-9０-９]{1,5})(?:\b|$)`)
)

func ParseCoverage(filename string) Coverage {
	s := strings.TrimSpace(filename)
	var c Coverage
	if a, b, ok := matchRange(jpVolumeRange, s); ok {
		c.Kind, c.VolumeStart, c.VolumeEnd = CoverageVolume, a, b
		c.Label = rangeLabel("巻", a, b)
	}
	if a, b, ok := matchRange(volRange, s); ok && c.Kind == CoverageUnknown {
		c.Kind, c.VolumeStart, c.VolumeEnd = CoverageVolume, a, b
		c.Label = rangeLabel("巻", a, b)
	}
	if a, b, ok := matchRange(jpChapterRange, s); ok {
		if c.Kind == CoverageVolume { c.Kind = CoverageMixed } else { c.Kind = CoverageChapter }
		c.ChapterStart, c.ChapterEnd = a, b
	}
	if a, b, ok := matchRange(chRange, s); ok && c.ChapterStart == 0 {
		if c.Kind == CoverageVolume { c.Kind = CoverageMixed } else { c.Kind = CoverageChapter }
		c.ChapterStart, c.ChapterEnd = a, b
	}
	if c.Kind == CoverageUnknown {
		p := Parse(filename)
		if p.HasVolume {
			c.Kind, c.VolumeStart, c.VolumeEnd = CoverageVolume, p.Volume, p.Volume
			c.Label = singleLabel("巻", p.Volume)
		}
	}
	if c.ChapterStart == 0 {
		if n, ok := matchOne(jpChapterOne, s); ok {
			if c.Kind == CoverageVolume { c.Kind = CoverageMixed } else if c.Kind == CoverageUnknown { c.Kind = CoverageChapter }
			c.ChapterStart, c.ChapterEnd = n, n
		} else if n, ok := matchOne(chOne, s); ok {
			if c.Kind == CoverageVolume { c.Kind = CoverageMixed } else if c.Kind == CoverageUnknown { c.Kind = CoverageChapter }
			c.ChapterStart, c.ChapterEnd = n, n
		}
	}
	if c.Kind == CoverageChapter {
		c.Label = rangeLabel("話", c.ChapterStart, c.ChapterEnd)
	} else if c.Kind == CoverageMixed {
		parts := []string{}
		if c.VolumeStart > 0 { parts = append(parts, rangeLabel("巻", c.VolumeStart, c.VolumeEnd)) }
		if c.ChapterStart > 0 { parts = append(parts, rangeLabel("話", c.ChapterStart, c.ChapterEnd)) }
		c.Label = strings.Join(parts, " / ")
	}
	return c
}

func MergeCoverage(items []Coverage) Coverage {
	out := Coverage{}
	for _, c := range items {
		if c.VolumeStart > 0 {
			if out.VolumeStart == 0 || c.VolumeStart < out.VolumeStart { out.VolumeStart = c.VolumeStart }
			if c.VolumeEnd > out.VolumeEnd { out.VolumeEnd = c.VolumeEnd }
		}
		if c.ChapterStart > 0 {
			if out.ChapterStart == 0 || c.ChapterStart < out.ChapterStart { out.ChapterStart = c.ChapterStart }
			if c.ChapterEnd > out.ChapterEnd { out.ChapterEnd = c.ChapterEnd }
		}
	}
	hasV, hasC := out.VolumeStart > 0, out.ChapterStart > 0
	switch { case hasV && hasC: out.Kind = CoverageMixed; case hasV: out.Kind = CoverageVolume; case hasC: out.Kind = CoverageChapter; default: out.Kind = CoverageUnknown }
	if hasV && hasC { out.Label = rangeLabel("巻", out.VolumeStart, out.VolumeEnd)+" / "+rangeLabel("話", out.ChapterStart, out.ChapterEnd) } else if hasV { out.Label = rangeLabel("巻", out.VolumeStart, out.VolumeEnd) } else if hasC { out.Label = rangeLabel("話", out.ChapterStart, out.ChapterEnd) }
	return out
}

func matchRange(re *regexp.Regexp, s string) (int, int, bool) {
	m := re.FindStringSubmatch(s); if len(m) != 3 { return 0,0,false }
	a, oka := parseCoverageInt(m[1]); b, okb := parseCoverageInt(m[2]); if !oka || !okb { return 0,0,false }
	if b < a { a,b = b,a }; return a,b,true
}
func matchOne(re *regexp.Regexp, s string) (int,bool) { m:=re.FindStringSubmatch(s); if len(m)!=2{return 0,false}; return parseCoverageInt(m[1]) }
func parseCoverageInt(s string)(int,bool){ s=strings.Map(func(r rune) rune{ if r>='０'&&r<='９'{return '0'+(r-'０')};return r},s); n,e:=strconv.Atoi(s);return n,e==nil&&n>=0 }
func rangeLabel(unit string,a,b int) string { if a==0{return ""}; if b==0||a==b{return singleLabel(unit,a)}; return unit+" "+strconv.Itoa(a)+"–"+strconv.Itoa(b) }
func singleLabel(unit string,n int) string { return unit+" "+strconv.Itoa(n) }
