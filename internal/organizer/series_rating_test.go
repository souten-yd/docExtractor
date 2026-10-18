package organizer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRatingIdentityAndTitleStars(t *testing.T) {
	for _, rating := range []string{"⭐", "⭐️", "⭐⭐", "★", "☆", "🌟"} {
		for _, name := range []string{rating + " 作品タイトル", "作品タイトル " + rating} {
			if canonicalKey(name) != canonicalKey("作品タイトル") {
				t.Errorf("rating not separated: %q", name)
			}
			if got := withSeriesRating("作品タイトル", name); !strings.Contains(got, rating) {
				t.Errorf("lost glyph: %q", got)
			}
		}
	}
	for _, title := range []string{"マジカル★エクスプローラー", "煩悩☆西遊記", "落ちこぼれ[☆1]魔法使い"} {
		plain, marks := splitSeriesRating(title)
		if plain != title || marks != "" {
			t.Fatalf("modified title star: %q", title)
		}
	}
	if canonicalKey("マジカル★エクスプローラー") == canonicalKey("マジカルエクスプローラー") {
		t.Fatal("internal star removed")
	}
}

func TestClusterPreservesStrongestRatingThroughAliases(t *testing.T) {
	aliases := map[string]string{"作品タイトル": "正式タイトル"}
	for _, reverse := range []bool{false, true} {
		plans := []Plan{{Series: "⭐️⭐️ 作品タイトル"}, {Series: "作品タイトル"}, {Series: "★ 作品タイトル"}}
		if reverse {
			plans[0], plans[2] = plans[2], plans[0]
		}
		for _, p := range clusterPlans(plans, aliases) {
			if p.Series != "⭐️⭐️ 正式タイトル" {
				t.Fatalf("lost rating: %q", p.Series)
			}
		}
	}
}

func writeLibraryStub(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileSubtitleAndRatingAcrossLibraries(t *testing.T) {
	a, b, out := t.TempDir(), t.TempDir(), t.TempDir()
	short := "万年Fランク【通訳】スキル持ち底辺冒険者、"
	long := short + "異種族の最強美少女たちとパーティーを組んで才能に開花し無双する"
	paths := []string{
		filepath.Join(a, "⭐️ "+short, "[作者甲×作者乙] "+short+" 第01巻.zip"),
		filepath.Join(b, long, "[作者乙×作者甲] "+long+" 第02巻.zip"),
	}
	for i, p := range paths {
		writeLibraryStub(t, p, fmt.Sprintf("untouched payload %d", i))
	}
	o, err := New(Config{Root: a, OutputRoot: out})
	if err != nil {
		t.Fatal(err)
	}
	report, err := o.ReconcileScanMulti([]string{a, b}, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 2 || report.Items[0].Series != report.Items[1].Series || !strings.HasPrefix(report.Items[0].Series, "⭐️ ") {
		t.Fatalf("not merged/rated: %+v", report.Items)
	}
	if report.Summary.Move != 2 || report.Summary.Superseded != 0 {
		t.Fatalf("summary: %+v", report.Summary)
	}
	result, err := o.ReconcileExecuteReportProgress(report, nil, nil)
	if err != nil || len(result.Errors) != 0 || result.Moved != 2 {
		t.Fatalf("execute: %+v %v", result, err)
	}
	for _, it := range report.Items {
		data, err := os.ReadFile(it.Destination)
		if err != nil || !strings.HasPrefix(string(data), "untouched payload") {
			t.Fatalf("payload: %v", err)
		}
	}
	again, err := o.ReconcileScanMulti([]string{a, b}, out)
	if err != nil || again.Summary.Keep != 2 || again.Summary.Move != 0 {
		t.Fatalf("not idempotent: %+v %v", again, err)
	}
}

func TestReconcileRatingSurvivesDuplicateQuarantine(t *testing.T) {
	root := t.TempDir()
	name := "[作者] 評価対象シリーズ 第01巻.zip"
	writeLibraryStub(t, filepath.Join(root, "評価対象シリーズ", name), "identical")
	writeLibraryStub(t, filepath.Join(root, "評価対象シリーズ ⭐️", name), "identical")
	o, _ := New(Config{Root: root})
	report, err := o.ReconcileScan(root)
	if err != nil || report.Summary.Duplicates != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	for _, it := range report.Items {
		if !strings.Contains(it.Series, "⭐️") {
			t.Fatal("lost rating before quarantine")
		}
	}
	result, err := o.ReconcileExecuteReportProgress(report, nil, nil)
	if err != nil || result.Quarantined != 1 || len(result.Errors) > 0 {
		t.Fatalf("%+v %v", result, err)
	}
	again, err := o.ReconcileScan(root)
	if err != nil || again.Summary.Keep != 1 || !strings.Contains(again.Items[0].Series, "⭐️") {
		t.Fatalf("%+v %v", again, err)
	}
}

func TestIncomingVolumeUsesRatedFolder(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	rated := "評価対象シリーズ ⭐️"
	if err := os.Mkdir(filepath.Join(out, rated), 0755); err != nil {
		t.Fatal(err)
	}
	name := "[作者] 評価対象シリーズ 第02巻.zip"
	writeTestComicZIP(t, filepath.Join(root, name), "評価対象シリーズ 第02巻")
	o, _ := New(Config{Root: root, OutputRoot: out})
	for _, scan := range []func() ([]Plan, error){o.Scan, func() ([]Plan, error) { return o.ScanWithProgress(nil) }} {
		plans, err := scan()
		if err != nil || len(plans) != 1 {
			t.Fatalf("%v %+v", err, plans)
		}
		if filepath.Dir(plans[0].Destination) != filepath.Join(out, rated) {
			t.Fatalf("scan: %+v", plans[0])
		}
		plan, err := o.PlanName(name)
		if err != nil || plan.Destination != plans[0].Destination {
			t.Fatalf("execution drift: %+v %v", plan, err)
		}
	}
}

func TestUnrelatedRatedBucketDoesNotRateEveryWork(t *testing.T) {
	got, _ := parentSeriesEvidence("/library", "/library/⭐️ お気に入り/[作者] 別作品 第01巻.zip", "別作品", .99, .72)
	if got != "別作品" {
		t.Fatalf("bucket leaked: %q", got)
	}
}

func TestReconcileDoesNotQuarantineDifferentSubtitleWork(t *testing.T) {
	root := t.TempDir()
	for i, title := range []string{"作品タイトル本編", "作品タイトル本編 別の物語"} {
		writeLibraryStub(t, filepath.Join(root, title, "[同一作者] "+title+" 第01巻.zip"), fmt.Sprintf("different %d", i))
	}
	o, _ := New(Config{Root: root})
	report, err := o.ReconcileScan(root)
	if err != nil || report.Summary.Duplicates != 0 || report.Summary.Superseded != 0 || report.Summary.Review != 0 {
		t.Fatalf("work identity collapsed: %+v %v", report, err)
	}
}

func TestNumberedSequelNeverCompetesForSameVolume(t *testing.T) {
	a := ReconcileItem{Series: "メカニカル バディ ユニバース1.0", WorkSeries: "メカニカル バディ ユニバース", Author: "同一作者"}
	b := a
	b.WorkSeries = "メカニカル バディ ユニバース1.0"
	if sameReconcileWork(a, b) {
		t.Fatal("numbered sequel treated as spelling alias")
	}
}
