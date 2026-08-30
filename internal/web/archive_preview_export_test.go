package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/souten-yd/docExtractor/internal/organizer"
)

func TestArchivePreviewExportUsesCachedPlanWithoutOpeningSource(t *testing.T) {
	root := t.TempDir()
	org, err := organizer.New(organizer.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Organizer: org}
	defer archiveScanStates.Delete(s)

	missingSource := filepath.Join(root, "already-removed.zip")
	target := filepath.Join(root, "BLACK LAGOON", "BLACK_LAGOON_01b.zip")
	st := s.archiveScanState()
	st.Phase = "done"
	st.Plans = []organizer.Plan{{
		Name:             "BLACK_LAGOON_01b-03b.zip",
		Source:           missingSource,
		Destination:      filepath.Join(root, "BLACK LAGOON", "BLACK_LAGOON_01b-03b.zip"),
		Series:           "BLACK LAGOON",
		Action:           "rename-zip",
		PredictedOutputs: []string{target},
		PreviewNested:    true,
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/archive/preview/export", nil)
	res := httptest.NewRecorder()
	s.archivePreviewExport(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, filepath.ToSlash(target)) {
		t.Fatalf("cached target missing from report: %s", body)
	}
	if strings.Contains(body, "preview_error:") {
		t.Fatalf("export tried to inspect the missing source: %s", body)
	}
}
