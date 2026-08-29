package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVersionComparison(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"0.2.0", "0.1.1", true},
		{"v0.1.1", "0.1.1", false},
		{"1.0.0", "0.99.99", true},
		{"0.1.0", "0.2.0", false},
		{"bad", "0.1.0", false},
	}
	for _, tc := range cases {
		if got := isNewer(tc.latest, tc.current); got != tc.want {
			t.Fatalf("isNewer(%q,%q)=%v want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestCheckLatestWithoutDownload(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"tag_name":"v0.2.0","html_url":"https://github.com/souten-yd/docExtractor/releases/tag/v0.2.0","published_at":"2026-08-29T00:00:00Z","assets":[{"name":"docExtractor_0.2.0_x86_64.qpkg","browser_download_url":"https://github.com/souten-yd/docExtractor/releases/download/v0.2.0/docExtractor_0.2.0_x86_64.qpkg","size":123,"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`
		return response(http.StatusOK, body), nil
	})}
	m, err := New(Config{CurrentVersion: "v0.1.1", DataDir: t.TempDir(), ReleaseAPI: "https://api.github.com/mock", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	st, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.UpdateAvailable || st.LatestVersion != "0.2.0" || st.AssetSize != 123 {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestVerifiedDownloadInstallerAndRestartRecognition(t *testing.T) {
	pkg := []byte("#!/bin/sh\necho test-qpkg\n")
	sum := sha256.Sum256(pkg)
	digest := hex.EncodeToString(sum[:])
	assetName := "docExtractor_0.2.0_x86_64.qpkg"
	assetURL := "https://github.com/souten-yd/docExtractor/releases/download/v0.2.0/" + assetName
	releaseBody := fmt.Sprintf(`{"tag_name":"v0.2.0","html_url":"https://github.com/souten-yd/docExtractor/releases/tag/v0.2.0","published_at":"2026-08-29T00:00:00Z","assets":[{"name":%q,"browser_download_url":%q,"size":%d,"digest":"sha256:%s"}]}`, assetName, assetURL, len(pkg), digest)

	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Host == "api.github.com":
			return response(http.StatusOK, releaseBody), nil
		case r.URL.Host == "github.com":
			resp := response(http.StatusOK, string(pkg))
			resp.ContentLength = int64(len(pkg))
			return resp, nil
		default:
			return nil, fmt.Errorf("unexpected host %s", r.URL.Host)
		}
	})}

	dir := t.TempDir()
	installed := make(chan string, 1)
	m, err := New(Config{
		CurrentVersion: "v0.1.1", DataDir: dir, ReleaseAPI: "https://api.github.com/mock", HTTPClient: client,
		Installer: func(packagePath, logPath string) error {
			got, err := os.ReadFile(packagePath)
			if err != nil {
				return err
			}
			if !bytes.Equal(got, pkg) {
				return fmt.Errorf("package content mismatch")
			}
			installed <- packagePath
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-installed:
	case <-time.After(2 * time.Second):
		t.Fatal("installer was not invoked")
	}
	st := m.Status()
	if st.State != StateInstalling || st.DownloadedSHA256 != digest || st.TargetVersion != "0.2.0" {
		t.Fatalf("unexpected installing status: %+v", st)
	}

	restarted, err := New(Config{CurrentVersion: "v0.2.0", DataDir: dir, HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.Status(); got.State != StateSucceeded || got.UpdateAvailable {
		t.Fatalf("restart did not recognize completed update: %+v", got)
	}
}

func TestDigestMismatchRefusesInstall(t *testing.T) {
	pkg := "not-the-expected-package"
	assetName := "docExtractor_0.2.0_x86_64.qpkg"
	releaseBody := fmt.Sprintf(`{"tag_name":"v0.2.0","html_url":"https://github.com/souten-yd/docExtractor/releases/tag/v0.2.0","published_at":"2026-08-29T00:00:00Z","assets":[{"name":%q,"browser_download_url":"https://github.com/download","size":%d,"digest":"sha256:%s"}]}`, assetName, len(pkg), strings.Repeat("a", 64))
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.github.com" {
			return response(http.StatusOK, releaseBody), nil
		}
		resp := response(http.StatusOK, pkg)
		resp.ContentLength = int64(len(pkg))
		return resp, nil
	})}
	called := make(chan struct{}, 1)
	m, err := New(Config{
		CurrentVersion: "v0.1.1", DataDir: t.TempDir(), ReleaseAPI: "https://api.github.com/mock", HTTPClient: client,
		Installer: func(_, _ string) error { called <- struct{}{}; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.Status().State == StateFailed {
			select {
			case <-called:
				t.Fatal("installer must not run after digest mismatch")
			default:
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("updater did not fail: %+v", m.Status())
}

func response(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
