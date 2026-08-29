package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/souten-yd/docExtractor/internal/diagnostics"
	"github.com/souten-yd/docExtractor/internal/jobs"
	"github.com/souten-yd/docExtractor/internal/organizer"
)

func TestQTSProxyPrefixAndDirectRoutes(t *testing.T) {
	root := t.TempDir()
	org, err := organizer.New(organizer.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	dm, err := diagnostics.New(diagnostics.Config{RootDir: filepath.Join(root, ".diag")})
	if err != nil {
		t.Fatal(err)
	}
	jm, err := jobs.New(1, 2, func(context.Context, string, jobs.Task, func(jobs.Update)) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer jm.Close()

	h := (&Server{Organizer: org, Jobs: jm, Diagnostics: dm, Version: "test"}).Handler()
	for _, target := range []string{"/", "/docExtractor", "/docExtractor/", "/api/status", "/docExtractor/api/status"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s => status %d", target, res.Code)
		}
	}
}
