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
	}
	for _, tc := range tests {
		got := Parse(tc.name)
		if got.Series != tc.series || got.Author != tc.author || got.Volume != tc.volume || got.HasVolume != tc.hasVol {
			t.Fatalf("%q => %+v", tc.name, got)
		}
	}
}

func TestSafeFolderName(t *testing.T) {
	if got := SafeFolderName(`a/b:c*?`); got != "a b c" { t.Fatalf("unexpected %q", got) }
}

func TestGroupKeyIgnoresCommonPunctuation(t *testing.T) {
	if GroupKey("Super-Manga 作品") != GroupKey("super manga・作品") { t.Fatal("expected equivalent group keys") }
}
