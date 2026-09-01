package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/souten-yd/docExtractor/internal/diagnostics"
	"github.com/souten-yd/docExtractor/internal/jobs"
	"github.com/souten-yd/docExtractor/internal/organizer"
	appsettings "github.com/souten-yd/docExtractor/internal/settings"
)

func TestSettingsPersistAndFolderBrowser(t *testing.T) {
	browseRoot := t.TempDir()
	initial := filepath.Join(browseRoot, "Incoming")
	next := filepath.Join(browseRoot, "Manga")
	child := filepath.Join(next, "Child")
	for _, dir := range []string{initial, next, child} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	dataDir := t.TempDir()
	storePath := filepath.Join(dataDir, "settings.json")
	store, err := appsettings.Open(storePath, appsettings.Settings{Root: initial})
	if err != nil {
		t.Fatal(err)
	}
	org, err := organizer.New(organizer.Config{Root: initial})
	if err != nil {
		t.Fatal(err)
	}
	dm, err := diagnostics.New(diagnostics.Config{RootDir: filepath.Join(dataDir, "diag")})
	if err != nil {
		t.Fatal(err)
	}
	jm, err := jobs.New(1, 2, func(context.Context, string, jobs.Task, func(jobs.Update)) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer jm.Close()
	h := (&Server{Organizer: org, Jobs: jm, Diagnostics: dm, Settings: store, BrowseRoot: browseRoot, Version: "test"}).Handler()

	body, _ := json.Marshal(map[string]string{"root": next})
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("settings update status=%d body=%s", res.Code, res.Body.String())
	}
	if got := org.Root(); got != next {
		t.Fatalf("organizer root=%q want=%q", got, next)
	}
	if got := org.OutputRoot(); got != next {
		t.Fatalf("organizer output root=%q want input root=%q", got, next)
	}
	reloaded, err := appsettings.Open(storePath, appsettings.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get().Root; got != next {
		t.Fatalf("persisted root=%q want=%q", got, next)
	}
	if got := reloaded.Get(); got.OutputMode != appsettings.OutputModeInput || got.OutputRoot != next {
		t.Fatalf("persisted input output mode did not follow root: %+v", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/directories?path="+next, nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("directory browser status=%d body=%s", res.Code, res.Body.String())
	}
	var listing directoryResponse
	if err := json.NewDecoder(res.Body).Decode(&listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].Path != child {
		t.Fatalf("unexpected listing: %+v", listing)
	}
}

func TestSettingsCustomOutputAndScanCacheInvalidation(t *testing.T) {
	browseRoot := t.TempDir()
	input := filepath.Join(browseRoot, "Incoming")
	output := filepath.Join(browseRoot, "Manga")
	for _, dir := range []string{input, output} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	store := appsettings.New(filepath.Join(t.TempDir(), "settings.json"), appsettings.Settings{Root: input})
	org, err := organizer.New(organizer.Config{Root: input})
	if err != nil {
		t.Fatal(err)
	}
	jm, err := jobs.New(1, 2, func(context.Context, string, jobs.Task, func(jobs.Update)) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer jm.Close()
	s := &Server{Organizer: org, Jobs: jm, Settings: store, BrowseRoot: browseRoot}
	defer archiveScanStates.Delete(s)
	st := s.archiveScanState()
	st.Phase = "done"
	st.Plans = []organizer.Plan{{Name: "old-from-temp.zip"}}
	h := s.Handler()

	body, _ := json.Marshal(map[string]string{"root": input, "output_mode": appsettings.OutputModeCustom, "output_root": output})
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("settings update status=%d body=%s", res.Code, res.Body.String())
	}
	if org.OutputRoot() != output {
		t.Fatalf("custom output=%q want=%q", org.OutputRoot(), output)
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	if st.Phase != "idle" || len(st.Plans) != 0 {
		t.Fatalf("old scan result was retained: phase=%q plans=%+v", st.Phase, st.Plans)
	}
}

func TestSettingsRejectOutsideBrowseRoot(t *testing.T) {
	browseRoot := t.TempDir()
	initial := filepath.Join(browseRoot, "Incoming")
	if err := os.MkdirAll(initial, 0o750); err != nil {
		t.Fatal(err)
	}
	store := appsettings.New(filepath.Join(t.TempDir(), "settings.json"), appsettings.Settings{Root: initial})
	org, err := organizer.New(organizer.Config{Root: initial})
	if err != nil {
		t.Fatal(err)
	}
	jm, err := jobs.New(1, 2, func(context.Context, string, jobs.Task, func(jobs.Update)) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer jm.Close()
	h := (&Server{Organizer: org, Jobs: jm, Settings: store, BrowseRoot: browseRoot}).Handler()

	outside := t.TempDir()
	body, _ := json.Marshal(map[string]string{"root": outside})
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("outside root should be rejected, status=%d body=%s", res.Code, res.Body.String())
	}
	if got := org.Root(); got != initial {
		t.Fatalf("root changed after rejected setting: %q", got)
	}
}
