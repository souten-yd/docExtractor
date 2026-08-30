package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const nestedVolumeRARFixture = "UmFyIRoHAQDz4YLrCwEFBwAGAQGAgIAAXYKDjDICAwvgAASHAaSDAtTl1rGAAwEUQkxBQ0tfTEFHT09OXzAxYi56aXAKAxNV3ZNqVJyTGsHGXTVjNC92AzIECdgBhwuBIrMDKTCxIqMHMoMZMHME6jkOHU9+MVjUSkTfzyx1+f1HvY9trYd9TFVGIutbjhxzvkZjtzhCM7986/ZJ4flWBEUjaTV4qA1t30lWb2kHANvKUx0yAgML3wAEhwGkgwIIcFArgAMBFEJMQUNLX0xBR09PTl8wMmIuemlwCgMTVd2TalSckxrBx1w1ZDMvdgMxwgOUlZUYQKAkVmBlJhYkVGhGFmPSiI5yUz8Yq+TSbXd92WdX7vqPex7ni1eNjEVWGmtbpn351qpY7c4QbMPfsoMk8PyUBIUzZ0V5KA11/8kWz28XADDtJjUyAgML4AAEhwGkgwKbJOG7gAMBFEJMQUNLX0xBR09PTl8wM2IuemlwCgMTVd2TalSckxrAx101YzMvdgQyBSy4AYcKgkczAziYWJCQkYkYCZWZDeFQ5bxv4xWNRKRN/PLHz8/uHvY9vtcrCxiKrDTWtyx574zWoducINnvn3UGSeH5WgSFM2dFeSgNev+SLR7WLgAdd1ZRAwUEAA=="

func TestPreviewOutputTargetsSplitsTopFolders(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(d, "bundle.zip")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, name := range []string{"作品A 第01巻/001.jpg", "作品B 第02巻/001.jpg"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("x"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	defaultDst := filepath.Join(d, "out", "bundle", "bundle.zip")
	p, err := PreviewOutputTargets(src, defaultDst)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Targets) != 2 {
		t.Fatalf("targets=%v", p.Targets)
	}
	if filepath.Base(p.Targets[0]) != "作品A 第01巻.zip" || filepath.Base(p.Targets[1]) != "作品B 第02巻.zip" {
		t.Fatalf("targets=%v", p.Targets)
	}
}

func previewZIPBytes(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entries[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestPreviewOutputTargetsExpandsNestedVolumeArchives(t *testing.T) {
	dir := t.TempDir()
	outer := previewZIPBytes(t, map[string][]byte{
		"BLACK_LAGOON_01b.zip": previewZIPBytes(t, map[string][]byte{"001.jpg": []byte("one")}),
		"BLACK_LAGOON_02b.zip": previewZIPBytes(t, map[string][]byte{"001.jpg": []byte("two")}),
		"BLACK_LAGOON_03b.zip": previewZIPBytes(t, map[string][]byte{"001.jpg": []byte("three")}),
	})
	src := filepath.Join(dir, "BLACK_LAGOON_01b-03b.zip")
	if err := os.WriteFile(src, outer, 0o640); err != nil {
		t.Fatal(err)
	}
	defaultDst := filepath.Join(dir, "out", "BLACK LAGOON ブラック・ラグーン", "BLACK_LAGOON_01b-03b.zip")
	preview, err := PreviewOutputTargets(src, defaultDst)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Nested {
		t.Fatal("nested archive should be reported")
	}
	if len(preview.Targets) != 3 {
		t.Fatalf("targets=%#v", preview.Targets)
	}
	wantBases := []string{"BLACK_LAGOON_01b.zip", "BLACK_LAGOON_02b.zip", "BLACK_LAGOON_03b.zip"}
	for i, want := range wantBases {
		if got := filepath.Base(preview.Targets[i]); got != want {
			t.Fatalf("target[%d]=%q want=%q; all=%#v", i, got, want, preview.Targets)
		}
	}
	if strings.Contains(strings.Join(preview.Targets, "\n"), "BLACK_LAGOON_01b-03b.zip") {
		t.Fatalf("outer collection must not be predicted as a published ZIP: %#v", preview.Targets)
	}

	result, err := New(Config{}).Process(context.Background(), Task{
		Source: src, Destination: defaultDst, DeleteSource: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Operation, "flatten-and-split") {
		t.Fatalf("operation=%q", result.Operation)
	}
	for _, target := range preview.Targets {
		if _, err := VerifyZIPNoNestedArchives(context.Background(), target, VerifyFull); err != nil {
			t.Fatalf("predicted target was not published as a normalized ZIP: %s: %v", target, err)
		}
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be removed after all three outputs are verified: %v", err)
	}
}

func TestRARPreviewAndExecutionPublishEachNestedVolume(t *testing.T) {
	dir := t.TempDir()
	raw, err := base64.StdEncoding.DecodeString(nestedVolumeRARFixture)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "BLACK_LAGOON_01b-03b.rar")
	if err := os.WriteFile(src, raw, 0o640); err != nil {
		t.Fatal(err)
	}
	defaultDst := filepath.Join(dir, "out", "BLACK LAGOON ブラック・ラグーン", "BLACK_LAGOON_01b-03b.zip")
	preview, err := PreviewOutputTargets(src, defaultDst)
	if err != nil {
		t.Fatal(err)
	}
	wantBases := []string{"BLACK_LAGOON_01b.zip", "BLACK_LAGOON_02b.zip", "BLACK_LAGOON_03b.zip"}
	if len(preview.Targets) != len(wantBases) {
		t.Fatalf("targets=%#v", preview.Targets)
	}
	for i, want := range wantBases {
		if got := filepath.Base(preview.Targets[i]); got != want {
			t.Fatalf("target[%d]=%q want=%q; all=%#v", i, got, want, preview.Targets)
		}
	}

	result, err := New(Config{}).Process(context.Background(), Task{
		Source: src, Destination: defaultDst, DeleteSource: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Operation, "rar-recursive-split") {
		t.Fatalf("operation=%q", result.Operation)
	}
	for _, target := range preview.Targets {
		if _, err := VerifyZIPNoNestedArchives(context.Background(), target, VerifyFull); err != nil {
			t.Fatalf("predicted RAR target was not published as a normalized ZIP: %s: %v", target, err)
		}
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("RAR source should be removed after all three outputs are verified: %v", err)
	}
}
