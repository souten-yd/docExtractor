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
