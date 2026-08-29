package web

import (
	"strings"
	"testing"
)

func TestReprocessNavigationUIContract(t *testing.T) {
	for _, want := range []string{"reprocessPageInput", "reprocessPageGo", "隔離ファイル管理", "解析完了後に「再整理を実行」が有効"} {
		if !strings.Contains(reprocessNavigationUIScript, want) {
			t.Fatalf("navigation UI missing %q", want)
		}
	}
}
