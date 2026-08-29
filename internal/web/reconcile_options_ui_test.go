package web

import (
	"strings"
	"testing"
)

func TestReconcileQuarantineScanControlsAreEmbedded(t *testing.T) {
	html := renderIndexHTML()
	for _, want := range []string{"reprocessIncludeQuarantine", "manageIncludeQuarantine", "隔離フォルダも解析する", "include_quarantine"} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered UI missing %q", want)
		}
	}
}
