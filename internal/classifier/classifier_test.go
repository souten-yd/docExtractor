package classifier

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name   string
		series string
		author string
		volume int
		hasVol bool
	}{
		{"[山田太郎] Super Manga 第01巻.zip", "Super Manga", "山田太郎", 1, true},
		{"【作者】作品名 第１２巻 特装版.rar", "作品名", "作者", 12, true},
		{"作品名 Vol.12.zip", "作品名", "", 12, true},
		{"作品名 v3.rar", "作品名", "", 3, true},
		{"作品名 07.zip", "作品名", "", 7, true},
		{"作品名.zip", "作品名", "", 0, false},
		{"[一般コミック][山田太郎] 作品名 第03巻.zip", "作品名", "山田太郎", 3, true},
		{"[Digital]【作者】作品名 第04巻.rar", "作品名", "作者", 4, true},
		{"[Y.A×楠本弘樹] 八男って、それはないでしょう！ 第08巻", "八男って、それはないでしょう！", "Y.A×楠本弘樹", 8, true},
		{"愛の流星カウーパ_ch06", "愛の流星カウーパ", "", 0, false},
		{"終わりのセラフ 20 - p007 [aKraa]", "終わりのセラフ", "", 20, true},
		{"ブラックラグーン 単行本", "ブラックラグーン", "", 0, false},
		{"ブラックラグーン コミックス", "ブラックラグーン", "", 0, false},
		{"[盧恩＆雪笠(Friendly Land)×早秋] 塔の管理をしてみよう 第02巻.zip", "塔の管理をしてみよう", "盧恩＆雪笠(Friendly Land)×早秋", 2, true},
		{"転生したら剣でした 転生したら剣でした 第17巻.zip", "転生したら剣でした", "", 17, true},
		{"怪獣自衛隊 1巻 別スキャン.zip", "怪獣自衛隊", "", 1, true},
		{"Kurasu Teni de ore Dake Haburareta v06s.zip", "Kurasu Teni de ore Dake Haburareta", "", 6, true},
		{"転生したら第七王子だったので、気ままに魔術を極めます セミカラー版 第02巻.zip", "転生したら第七王子だったので、気ままに魔術を極めます", "", 2, true},
		{"作品名 第03巻 [カラー版].zip", "作品名", "", 3, true},
		{"作品名 番外編.zip", "作品名", "", 0, false},
		{"作品名 外伝.zip", "作品名", "", 0, false},
		{"作品名 第04巻 fix.zip", "作品名", "", 4, true},
		{"異世界転移した俺は、Ｈのたびにガチャを引く！ 第01-02巻s.zip", "異世界転移した俺は、Ｈのたびにガチャを引く！", "", 0, false},
		// A named spin-off remains distinct; only a standalone trailing variant
		// marker is folded into the main series identity.
		{"転生したらスライムだった件 異聞 ～魔国暮らしのトリニティ～ 第01巻.zip", "転生したらスライムだった件 異聞 ～魔国暮らしのトリニティ～", "", 1, true},
	}
	for _, tc := range tests {
		got := Parse(tc.name)
		if got.Series != tc.series || got.Author != tc.author || got.Volume != tc.volume || got.HasVolume != tc.hasVol {
			t.Fatalf("%q => %+v, want series=%q author=%q volume=%d has=%v", tc.name, got, tc.series, tc.author, tc.volume, tc.hasVol)
		}
	}
}

func TestVariantGroupKeysUnifyWithMainSeries(t *testing.T) {
	main := GroupKey("終わりのセラフ")
	for _, name := range []string{
		"終わりのセラフ カラー版",
		"終わりのセラフ セミカラー版",
		"終わりのセラフ 別スキャン",
		"終わりのセラフ 別炊",
		"終わりのセラフ 修正版",
		"終わりのセラフ 番外編",
		"終わりのセラフ 外伝",
		"終わりのセラフ 特典",
	} {
		if got := GroupKey(name); got != main {
			t.Fatalf("GroupKey(%q)=%q want %q", name, got, main)
		}
	}
}

func TestSafeFolderName(t *testing.T) {
	if got := SafeFolderName(`a/b:c*?`); got != "a b c" { t.Fatalf("unexpected %q", got) }
}

func TestGroupKeyIgnoresCommonPunctuation(t *testing.T) {
	if GroupKey("Super-Manga 作品") != GroupKey("super manga・作品") { t.Fatal("expected equivalent group keys") }
}
