package classifier

import "testing"

func TestParseExistingFilenameKeepsV0211VolumeBehavior(t *testing.T) {
	cases := []struct {
		name      string
		series    string
		volume    int
		hasVolume bool
	}{
		{"[作者] 作品名 第05巻.zip", "作品名", 5, true},
		// Deliberately preserve v0.2.11 behavior for ambiguous legacy suffixes.
		{"Kurasu Teni de ore Dake Haburareta v06s.zip", "Kurasu Teni de ore Dake Haburareta v06s", 0, false},
		{"異世界転移した俺は、Ｈのたびにガチャを引く！ 第01-02巻s.zip", "異世界転移した俺は、Ｈのたびにガチャを引く！ 第01-02巻s", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseExistingFilename(tc.name)
			if got.Series != tc.series || got.Volume != tc.volume || got.HasVolume != tc.hasVolume {
				t.Fatalf("got series=%q volume=%d has=%v", got.Series, got.Volume, got.HasVolume)
			}
		})
	}
}

func TestParseExistingFilenameUnifiesSimpleVariantsWithMainSeries(t *testing.T) {
	cases := []string{
		"作品名 カラー版 第01巻.zip",
		"作品名 セミカラー 第01巻.zip",
		"作品名 第01巻 別スキャン.zip",
		"作品名 第01巻 別炊.zip",
		"作品名 第01巻 fix.zip",
		"作品名 番外編.zip",
		"作品名 外伝 第02巻.zip",
		"作品名 特典.zip",
		"作品名 おまけ.zip",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			got := ParseExistingFilename(name)
			if got.Series != "作品名" {
				t.Fatalf("series=%q", got.Series)
			}
		})
	}
}

func TestParseExistingFilenameHandlesNestedAuthorParentheses(t *testing.T) {
	got := ParseExistingFilename("[盧恩＆雪笠(Friendly Land)×早秋] 塔の管理をしてみよう 第02巻.zip")
	if got.Series != "塔の管理をしてみよう" || !got.HasVolume || got.Volume != 2 {
		t.Fatalf("unexpected parse: %+v", got)
	}
}
