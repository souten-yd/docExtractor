package web

import (
	"path/filepath"
	"testing"
)

func TestValidateAsyncManageIncludesOutputLibrary(t *testing.T) {
	base := t.TempDir()
	input := filepath.Join(base, "Temp")
	output := filepath.Join(base, "Manga")
	s := &Server{BrowseRoot: base}
	req := asyncReconcileRequest{Mode: "manage", Roots: []string{input}, OutputRoot: output}
	if err := s.validateAsyncRequest(&req); err != nil {
		t.Fatal(err)
	}
	if len(req.Roots) != 2 || req.Roots[0] != input || req.Roots[1] != output {
		t.Fatalf("manage roots=%v; output library must be included", req.Roots)
	}
}
