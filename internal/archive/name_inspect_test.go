package archive

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectZIPNamesCollectsUsefulMetadataOnly(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "sample.zip")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, name := range []string{
		"大自然の魔法師アシュト 第06巻/001.jpg",
		"大自然の魔法師アシュト 第06巻/002.jpg",
		"内包作品 第01巻.zip",
		"images/003.jpg",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("payload-not-read-by-inspector")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := InspectNames(filename)
	if err != nil {
		t.Fatal(err)
	}
	if got.Entries != 4 {
		t.Fatalf("entries=%d", got.Entries)
	}
	var folder, nested bool
	for _, c := range got.Candidates {
		if c.Kind == CandidateTopDirectory && c.Name == "大自然の魔法師アシュト 第06巻" {
			folder = true
		}
		if c.Kind == CandidateNestedArchive && c.Name == "内包作品 第01巻" {
			nested = true
		}
		if c.Name == "images" || c.Name == "001" || c.Name == "002" || c.Name == "003" {
			t.Fatalf("generic/numeric candidate leaked: %#v", c)
		}
	}
	if !folder || !nested {
		t.Fatalf("candidates=%#v", got.Candidates)
	}
}
