package web

import (
	"strings"
	"testing"
)

func TestTabbedModesAndInlineBrowserSlotsAreEmbedded(t *testing.T) {
	html := renderIndexHTML()
	for _, want := range []string{
		"アーカイブ処理",
		"既存ファイル再整理",
		"統合ファイル管理",
		"archiveRootBrowserSlot",
		"archiveOutputBrowserSlot",
		"reprocessBrowserSlot",
		"reconcileRootBrowserSlot",
		"reconcileOutputBrowserSlot",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered UI missing %q", want)
		}
	}
}

func TestManageItemsUseTheirOwnTimeFormatter(t *testing.T) {
	html := renderIndexHTML()
	if !strings.Contains(html, "function fmtManageTime") || !strings.Contains(html, "fmtManageTime(x.modified_at)") {
		t.Fatal("manage result renderer is missing its scoped time formatter")
	}
	if strings.Contains(html, "fmtTime?fmtTime") {
		t.Fatal("manage result renderer still references the formatter from another script scope")
	}
}

func TestManageUIExplainsOutputLibraryComparison(t *testing.T) {
	if html := renderIndexHTML(); !strings.Contains(html, "既存ファイルも比較対象として自動解析します") {
		t.Fatal("manage UI does not explain that the existing output library is scanned")
	}
}
