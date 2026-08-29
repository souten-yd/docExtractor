package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/souten-yd/docExtractor/internal/diagnostics"
	"github.com/souten-yd/docExtractor/internal/jobs"
	"github.com/souten-yd/docExtractor/internal/organizer"
	"github.com/souten-yd/docExtractor/internal/updater"
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
	um, err := updater.New(updater.Config{CurrentVersion: "v0.2.0", DataDir: filepath.Join(root, ".updates")})
	if err != nil {
		t.Fatal(err)
	}

	h := (&Server{Organizer: org, Jobs: jm, Diagnostics: dm, Updater: um, Version: "v0.2.0"}).Handler()
	for _, target := range []string{"/", "/docExtractor", "/docExtractor/", "/api/status", "/docExtractor/api/status", "/api/update", "/docExtractor/api/update"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s => status %d", target, res.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/docExtractor/", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if !strings.Contains(res.Body.String(), "アプリ更新") {
		t.Fatal("embedded Web UI does not contain update controls")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/update/install", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("update without confirmation header => status %d", res.Code)
	}
}
