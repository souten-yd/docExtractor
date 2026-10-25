package web

import (
	"bytes"
	"context"
	"image/png"
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
	for _, prefix := range []string{"", "/docExtractor"} {
		for name, size := range map[string]int{"apple-touch-icon.png": 180, "favicon.png": 32} {
			target := prefix + "/" + name + "?v=1"
			res := httptest.NewRecorder()
			h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, target, nil))
			if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "image/png" {
				t.Fatalf("%s: status=%d type=%s", target, res.Code, res.Header().Get("Content-Type"))
			}
			cfg, err := png.DecodeConfig(bytes.NewReader(res.Body.Bytes()))
			if err != nil || cfg.Width != size || cfg.Height != size {
				t.Fatalf("%s: invalid PNG dimensions: %+v, %v", target, cfg, err)
			}
			if res.Header().Get("Cache-Control") != "public, max-age=86400" {
				t.Fatalf("%s: missing cache policy", target)
			}
			res = httptest.NewRecorder()
			h.ServeHTTP(res, httptest.NewRequest(http.MethodHead, target, nil))
			if res.Code != http.StatusOK || res.Body.Len() != 0 {
				t.Fatalf("HEAD %s: status=%d body=%d", target, res.Code, res.Body.Len())
			}
		}
	}
	for _, target := range []string{"/", "/docExtractor", "/docExtractor/"} {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, target, nil))
		for _, marker := range []string{`rel="apple-touch-icon"`, `href="/docExtractor/apple-touch-icon.png?v=1"`, `href="/docExtractor/favicon.png?v=1"`, `name="apple-mobile-web-app-title" content="docExtractor"`} {
			if !strings.Contains(res.Body.String(), marker) {
				t.Fatalf("%s: missing icon metadata %s", target, marker)
			}
		}
	}
	for _, target := range []string{"/static/index.html", "/docExtractor/static/index.html", "/missing-icon.png"} {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, target, nil))
		if res.Code != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d", target, res.Code)
		}
	}
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
