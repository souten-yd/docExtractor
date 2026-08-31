package organizer

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func copyReconcileTestFile(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileKeepsParentAndDerivativeVolumesIndependent(t *testing.T) {
	root := t.TempDir()
	folder := "転生したらスライムだった件"
	main := filepath.Join(root, folder, "[伏瀬×川上泰樹] 転生したらスライムだった件 第11巻.zip")
	derivative := filepath.Join(root, folder, "[伏瀬×戸野タエ] 転生したらスライムだった件 異聞 ～魔国暮らしのトリニティ～ 第11巻.zip")
	writeTestComicZIP(t, main, "main")
	writeTestComicZIP(t, derivative, "derivative")
	now := time.Now()
	if err := os.Chtimes(main, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(derivative, now, now); err != nil {
		t.Fatal(err)
	}
	o, err := New(Config{Root: root, OutputRoot: root, ConfidenceThreshold: .72})
	if err != nil {
		t.Fatal(err)
	}
	report, err := o.ReconcileScan(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Superseded != 0 || report.Summary.Review != 0 || report.Summary.Keep != 2 {
		t.Fatalf("parent and derivative conflicted: summary=%#v items=%#v", report.Summary, report.Items)
	}
	if report.Items[0].WorkSeries == report.Items[1].WorkSeries {
		t.Fatalf("work identities collapsed: %#v", report.Items)
	}
}

func TestReconcileExactBytesAcrossDifferentWorksAreNotDuplicates(t *testing.T) {
	root := t.TempDir()
	folder := "長い作品タイトル"
	main := filepath.Join(root, folder, "長い作品タイトル 第01巻.zip")
	derivative := filepath.Join(root, folder, "長い作品タイトル 外伝 竜の軌跡 第01巻.zip")
	writeTestComicZIP(t, main, "same bytes")
	copyReconcileTestFile(t, main, derivative)
	o, err := New(Config{Root: root, OutputRoot: root, ConfidenceThreshold: .72})
	if err != nil {
		t.Fatal(err)
	}
	report, err := o.ReconcileScan(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Duplicates != 0 || report.Summary.Superseded != 0 || report.Summary.Keep != 2 {
		t.Fatalf("different works were deduplicated: summary=%#v items=%#v", report.Summary, report.Items)
	}
}

func TestReconcileKeepsColorEditionsInSameFolderButSeparate(t *testing.T) {
	root := t.TempDir()
	folder := "青の祓魔師"
	standard := filepath.Join(root, folder, "[加藤和恵] 青の祓魔師 第07巻.zip")
	color := filepath.Join(root, folder, "[加藤和恵] 青の祓魔師 カラー版 第07巻.zip")
	semiColor := filepath.Join(root, folder, "[加藤和恵] 青の祓魔師 セミカラー版 第07巻.zip")
	writeTestComicZIP(t, standard, "standard")
	writeTestComicZIP(t, color, "color")
	writeTestComicZIP(t, semiColor, "semi-color")
	now := time.Now()
	for i, name := range []string{standard, color, semiColor} {
		stamp := now.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(name, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	o, err := New(Config{Root: root, OutputRoot: root, ConfidenceThreshold: .72})
	if err != nil {
		t.Fatal(err)
	}
	report, err := o.ReconcileScan(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Superseded != 0 || report.Summary.Duplicates != 0 || report.Summary.Keep != 3 {
		t.Fatalf("color editions conflicted: summary=%#v items=%#v", report.Summary, report.Items)
	}
	want := map[string]bool{"standard": false, "color": false, "semi-color": false}
	for _, item := range report.Items {
		if filepath.Base(filepath.Dir(item.Destination)) != folder {
			t.Fatalf("edition escaped common folder: %#v", item)
		}
		want[item.Edition] = true
	}
	for edition, found := range want {
		if !found {
			t.Fatalf("edition %q not identified: %#v", edition, report.Items)
		}
	}
}

func TestReconcileRestoresWronglyQuarantinedDerivativeCollision(t *testing.T) {
	root := t.TempDir()
	folder := "陰の実力者になりたくて！"
	mainName := "[坂野杏梨×逢沢大介] 陰の実力者になりたくて！ 第01巻.zip"
	main := filepath.Join(root, ".docExtractor-duplicates", folder, mainName)
	derivative := filepath.Join(root, folder, "[kanco×逢沢大介] 陰の実力者になりたくて！マスターオブガーデン～七陰列伝～ 第01巻.zip")
	writeTestComicZIP(t, main, "main")
	writeTestComicZIP(t, derivative, "derivative")
	o, err := New(Config{Root: root, OutputRoot: root, ConfidenceThreshold: .72})
	if err != nil {
		t.Fatal(err)
	}
	report, err := o.ReconcileScanMultiProgressWithOptions([]string{root}, root, ReconcileScanOptions{IncludeQuarantine: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Move != 1 || report.Summary.Superseded != 0 || report.Summary.Review != 0 {
		t.Fatalf("quarantined parent was not safely restorable: summary=%#v items=%#v", report.Summary, report.Items)
	}
	want := filepath.Join(root, folder, mainName)
	result, err := o.ReconcileExecuteReportProgress(report, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Moved != 1 {
		t.Fatalf("moved=%d result=%#v", result.Moved, result)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("restored parent missing: %v", err)
	}
	if _, err := os.Stat(derivative); err != nil {
		t.Fatalf("derivative was disturbed: %v", err)
	}
}

func TestReconcileRestoresWronglyQuarantinedColorEdition(t *testing.T) {
	root := t.TempDir()
	folder := "第七王子"
	standardName := "第七王子 第01巻.zip"
	standard := filepath.Join(root, ".docExtractor-duplicates", folder, standardName)
	color := filepath.Join(root, folder, "第七王子 フルカラー版 第01巻.zip")
	writeTestComicZIP(t, standard, "standard")
	writeTestComicZIP(t, color, "full-color")
	o, err := New(Config{Root: root, OutputRoot: root, ConfidenceThreshold: .72})
	if err != nil {
		t.Fatal(err)
	}
	report, err := o.ReconcileScanMultiProgressWithOptions([]string{root}, root, ReconcileScanOptions{IncludeQuarantine: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Move != 1 || report.Summary.Superseded != 0 || report.Summary.Duplicates != 0 {
		t.Fatalf("quarantined standard edition was not safely restorable: summary=%#v items=%#v", report.Summary, report.Items)
	}
	result, err := o.ReconcileExecuteReportProgress(report, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Moved != 1 {
		t.Fatalf("moved=%d result=%#v", result.Moved, result)
	}
	if _, err := os.Stat(filepath.Join(root, folder, standardName)); err != nil {
		t.Fatalf("restored standard edition missing: %v", err)
	}
	if _, err := os.Stat(color); err != nil {
		t.Fatalf("color edition was disturbed: %v", err)
	}
}
