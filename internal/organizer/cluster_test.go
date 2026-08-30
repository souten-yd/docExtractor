package organizer

import "testing"

func TestSameSeriesNormalizesPunctuation(t *testing.T) {
	score, _ := sameSeries("BLACK_LAGOON", "Black Lagoon")
	if score < 0.99 { t.Fatalf("score=%v", score) }
}

func TestSameSeriesBilingualAliasLatinFirst(t *testing.T) {
	score, reason := sameSeries("BLACK LAGOON", "BLACK LAGOON ブラック・ラグーン")
	if score < 0.97 { t.Fatalf("score=%v reason=%s", score, reason) }
}

func TestSameSeriesBilingualAliasJapaneseFirst(t *testing.T) {
	score, reason := sameSeries("ブラックラグーン", "ブラックラグーン Black Lagoon")
	if score < 0.97 { t.Fatalf("score=%v reason=%s", score, reason) }
}

func TestSameSeriesMinorVariation(t *testing.T) {
	score, _ := sameSeries("灰宮先輩は怖くてかわいい", "灰宮先輩は怖くてかわいぃ")
	if score < 0.90 { t.Fatalf("score=%v", score) }
}

func TestSpinOffDoesNotAutoMerge(t *testing.T) {
	for _, b := range []string{"BLACK LAGOON 外伝", "BLACK LAGOON 異聞", "BLACK LAGOON 前日譚"} {
		score, _ := sameSeries("BLACK LAGOON", b)
		if score != 0 { t.Fatalf("spin-off should not merge: %q score=%v", b, score) }
	}
}

func TestClusterChoosesConciseJapaneseCanonical(t *testing.T) {
	plans := []Plan{
		{Series: "BLACK LAGOON", Destination: "/share/x/BLACK LAGOON/a.zip"},
		{Series: "BLACK LAGOON ブラック・ラグーン", Destination: "/share/x/BLACK LAGOON ブラック・ラグーン/b.zip"},
		{Series: "ブラック・ラグーン", Destination: "/share/x/ブラック・ラグーン/c.zip"},
	}
	got := clusterPlans(plans, nil)
	for _, p := range got {
		if p.Series != "ブラック・ラグーン" { t.Fatalf("unexpected canonical: %q", p.Series) }
	}
}

func TestClusterMergesSameAuthorPrefixSubtitleVariants(t *testing.T) {
	cases := [][2]string{
		{"万年Fランク【通訳】スキル持ち底辺冒険者、", "万年Fランク【通訳】スキル持ち底辺冒険者、異種族の最強美少女たちとパーティーを組んで才能に開花し無双する"},
		{"異世界から帰還したら地球もかなりファンタジーでした。", "異世界から帰還したら地球もかなりファンタジーでした。あと、負けヒロインどもこっち見んな。"},
		{"マジカル★エクスプローラー", "マジカル★エクスプローラー エロゲの友人キャラに転生したけど、ゲーム知識使って自由に生きる"},
		{"てのひら開拓村で異世界建国記", "てのひら開拓村で異世界建国記 ～増えてく嫁たちとのんびり無人島ライフ～"},
		{"貴族家三男の成り上がりライフ", "貴族家三男の成り上がりライフ 生まれてすぐに人外認定された少年は異世界を満喫する"},
		{"追放最凶クズ（？）賢者の辺境子育てスローライフ", "追放最凶クズ（？）賢者の辺境子育てスローライフ クズだと勘違いされがちな最強の善人は魔王の娘を超絶いい子に育て上げる"},
		{"乙女ゲームの破滅フラグしかない悪役令嬢に転生してしまった…", "乙女ゲームの破滅フラグしかない悪役令嬢に転生してしまった…カタリナからの手紙"},
	}
	for _, tc := range cases {
		plans := []Plan{
			{Series:tc[0],Author:"同一作者",Destination:"/share/x/"+tc[0]+"/a.zip"},
			{Series:tc[1],Author:"同一作者",Destination:"/share/x/"+tc[1]+"/b.zip"},
		}
		got:=clusterPlans(plans,nil)
		if got[0].Series!=got[1].Series { t.Fatalf("did not merge %q and %q: %q / %q",tc[0],tc[1],got[0].Series,got[1].Series) }
	}
}

func TestClusterDoesNotMergePrefixWithoutAuthorEvidence(t *testing.T) {
	plans:=[]Plan{{Series:"作品タイトル",Destination:"/share/x/作品タイトル/a.zip"},{Series:"作品タイトル 長い別作品名",Destination:"/share/x/作品タイトル 長い別作品名/b.zip"}}
	got:=clusterPlans(plans,nil)
	if got[0].Series==got[1].Series { t.Fatalf("prefix-only titles should not merge without author evidence") }
}

func TestClusterDoesNotMergeNamedSpinOffEvenWithSameAuthor(t *testing.T) {
	plans:=[]Plan{{Series:"転生したらスライムだった件",Author:"作者",Destination:"/share/x/a/a.zip"},{Series:"転生したらスライムだった件 異聞 ～魔国暮らしのトリニティ～",Author:"作者",Destination:"/share/x/b/b.zip"}}
	got:=clusterPlans(plans,nil)
	if got[0].Series==got[1].Series { t.Fatalf("named spin-off should remain separate") }
}

func TestPersistedAliasWins(t *testing.T) {
	plans := []Plan{{Series: "Black_Lagoon", Destination: "/share/x/Black_Lagoon/a.zip"}}
	got := clusterPlans(plans, map[string]string{"Black Lagoon": "ブラック・ラグーン"})
	if got[0].Series != "ブラック・ラグーン" { t.Fatalf("got %q", got[0].Series) }
}

func TestBrokenAndVolumeOnlySeriesAreRejected(t *testing.T) {
	for _, s := range []string{"第13巻", "Vol. 8", "ch06", "���作品名", "A"} {
		if seriesNameUsable(s) { t.Fatalf("should reject %q", s) }
	}
}
