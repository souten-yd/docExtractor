package web

import (
	"strings"
	"testing"
)

func TestArchiveScanUIInjected(t *testing.T) {
	h := renderIndexHTML()
	for _, want := range []string{"archiveScanProgress", "api/scan?op=start", "api/scan?op=status", "ブラウザを閉じてもスキャンはサーバー側で継続"} {
		if !strings.Contains(h, want) { t.Fatalf("rendered UI missing %q", want) }
	}
}
