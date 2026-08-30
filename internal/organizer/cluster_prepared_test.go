package organizer

import "testing"

func TestSameSeriesPreparedMatchesLegacyDecision(t *testing.T) {
	pairs := [][2]string{
		{"終わりのセラフ", "終わりのセラフ"},
		{"DEAD Tube -デッドチューブ", "DEAD Tube デッドチューブ"},
		{"転生したら剣でした", "転生したら剣でした。"},
		{"作品名", "作品名 外伝"},
		{"Magic User", "Magic User マジックユーザー"},
		{"abcdefghijklmnop", "abcdefghijklmnoq"},
		{"短い題名", "別の題名"},
	}
	for _, p := range pairs {
		want, _ := sameSeries(p[0], p[1])
		got, _ := sameSeriesPrepared(prepareSeries(p[0]), prepareSeries(p[1]))
		if (want >= 0.90) != (got >= 0.90) {
			t.Fatalf("decision mismatch for %q / %q: legacy=%f prepared=%f", p[0], p[1], want, got)
		}
	}
}

func BenchmarkSameSeriesLegacy(b *testing.B) {
	a, c := "転生したらスライムだった件", "転生したらスライムだつた件"
	b.ResetTimer()
	for i := 0; i < b.N; i++ { _, _ = sameSeries(a, c) }
}

func BenchmarkSameSeriesPrepared(b *testing.B) {
	a, c := prepareSeries("転生したらスライムだった件"), prepareSeries("転生したらスライムだつた件")
	b.ResetTimer()
	for i := 0; i < b.N; i++ { _, _ = sameSeriesPrepared(a, c) }
}
