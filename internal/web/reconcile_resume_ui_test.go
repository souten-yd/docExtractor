package web

import (
	"strings"
	"testing"
)

func TestReconcileResumeUIInstalled(t *testing.T) {
	h := renderIndexHTML()
	for _, want := range []string{"直前の解析結果を再適用", "サーバー処理を停止", "overall_progress", "実行中の状態を反映"} {
		if !strings.Contains(h, want) { t.Fatalf("missing resume UI marker %q", want) }
	}
}

func TestOverallReconcileProgressDoesNotFinishAtInspection(t *testing.T) {
	if got := overallReconcileProgress("scan", "inspecting", 100, 100); got >= 0.5 {
		t.Fatalf("inspection completion must not mean job completion: %v", got)
	}
	if got := overallReconcileProgress("scan", "clustering", 100, 100); got >= 0.8 {
		t.Fatalf("legacy clustering entry must not look complete: %v", got)
	}
	if got := overallReconcileProgress("scan", "done", 100, 100); got != 1 {
		t.Fatalf("done progress=%v", got)
	}
}
