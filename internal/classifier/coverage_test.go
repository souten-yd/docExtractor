package classifier

import "testing"

func TestParseCoverageRanges(t *testing.T) {
	tests := []struct {
		name  string
		kind  CoverageKind
		vs,ve int
		cs,ce int
		label string
	}{
		{"作品名 第01巻-第30巻.zip", CoverageVolume, 1, 30, 0, 0, "巻 1–30"},
		{"作品名 001-120巻.rar", CoverageVolume, 1, 120, 0, 0, "巻 1–120"},
		{"作品名 ch.001-250.zip", CoverageChapter, 0, 0, 1, 250, "話 1–250"},
		{"作品名 第12話-第80話.rar", CoverageChapter, 0, 0, 12, 80, "話 12–80"},
	}
	for _, tc := range tests {
		got := ParseCoverage(tc.name)
		if got.Kind != tc.kind || got.VolumeStart != tc.vs || got.VolumeEnd != tc.ve || got.ChapterStart != tc.cs || got.ChapterEnd != tc.ce || got.Label != tc.label {
			t.Fatalf("%q => %+v", tc.name, got)
		}
	}
}

func TestMergeCoverageAcrossManyVolumes(t *testing.T) {
	items := make([]Coverage, 0, 150)
	for i := 1; i <= 150; i++ {
		items = append(items, Coverage{Kind: CoverageVolume, VolumeStart: i, VolumeEnd: i})
	}
	got := MergeCoverage(items)
	if got.Kind != CoverageVolume || got.VolumeStart != 1 || got.VolumeEnd != 150 || got.Label != "巻 1–150" {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeChapterCoverageAcrossManyEntries(t *testing.T) {
	items := make([]Coverage, 0, 400)
	for i := 1; i <= 400; i++ {
		items = append(items, Coverage{Kind: CoverageChapter, ChapterStart: i, ChapterEnd: i})
	}
	got := MergeCoverage(items)
	if got.Kind != CoverageChapter || got.ChapterStart != 1 || got.ChapterEnd != 400 || got.Label != "話 1–400" {
		t.Fatalf("got %+v", got)
	}
}
