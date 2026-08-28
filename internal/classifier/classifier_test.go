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
	}
	for _, tc := range tests {
		got := Parse(tc.name)
		if got.Series != tc.series || got.Author != tc.author || got.Volume != tc.volume || got.HasVolume != tc.hasVol {
			t.Fatalf("%q => %+v", tc.name, got)
		}
	}
}

func TestSafeFolderName(t *testing.T) {
	if got := SafeFolderName(`a/b:c*?`); got != "a b c" {
		t.Fatalf("unexpected %q", got)
	}
}
