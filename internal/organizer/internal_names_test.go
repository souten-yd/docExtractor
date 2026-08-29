package organizer

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeTestZIP(t *testing.T, filename string, names []string) {
	t.Helper()
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestInternalJapaneseFolderBeatsOuterRomanizedName(t *testing.T) {
	root := t.TempDir()
	name := "Daishizen_no_mahoushi_ashuto_01-06.zip"
	writeTestZIP(t, filepath.Join(root, name), []string{
		"[一般コミック] 大自然の魔法師アシュト 第06巻/001.jpg",
		"[一般コミック] 大自然の魔法師アシュト 第06巻/002.jpg",
	})
	o, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := o.PlanName(name)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Series != "大自然の魔法師アシュト" {
		t.Fatalf("series=%q", plan.Series)
	}
	if plan.NameSource != "top-directory" {
		t.Fatalf("name source=%q", plan.NameSource)
	}
	if !plan.HasVolume || plan.Volume != 6 {
		t.Fatalf("volume=%d has=%v", plan.Volume, plan.HasVolume)
	}
	if plan.Confidence < 0.90 {
		t.Fatalf("confidence=%v", plan.Confidence)
	}
}

func TestNestedArchivesProvideConsensusSeries(t *testing.T) {
	root := t.TempDir()
	name := "BLACK_LAGOON_00-08v2.zip"
	writeTestZIP(t, filepath.Join(root, name), []string{
		"[広江礼威] BLACK LAGOON 第01巻.zip",
		"[広江礼威] BLACK LAGOON 第02巻.zip",
		"[広江礼威] BLACK LAGOON 第03巻.zip",
	})
	o, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := o.PlanName(name)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Series != "BLACK LAGOON" {
		t.Fatalf("series=%q", plan.Series)
	}
	if plan.NameSource != "nested-archive" {
		t.Fatalf("source=%q", plan.NameSource)
	}
	if len(plan.Candidates) < 3 {
		t.Fatalf("candidates=%v", plan.Candidates)
	}
	if plan.Confidence < 0.90 {
		t.Fatalf("confidence=%v", plan.Confidence)
	}
}

func TestNumericImagesDoNotOverrideOuterName(t *testing.T) {
	root := t.TempDir()
	name := "作品名 第03巻.zip"
	writeTestZIP(t, filepath.Join(root, name), []string{"001.jpg", "002.jpg", "003.png"})
	o, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := o.PlanName(name)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Series != "作品名" || plan.NameSource != "outer-filename" {
		t.Fatalf("series=%q source=%q", plan.Series, plan.NameSource)
	}
}
