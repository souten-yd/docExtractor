package organizer

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func nestedZIPBytes(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestInferFromArchiveFindsJapaneseTitleInsideNestedZIP(t *testing.T) {
	dir := t.TempDir()
	deep := nestedZIPBytes(t, map[string][]byte{
		"ブラックラグーン 第05巻/001.jpg": []byte("image"),
		"ブラックラグーン 第05巻/002.jpg": []byte("image2"),
	})
	middle := nestedZIPBytes(t, map[string][]byte{
		"wrapper/deep.zip": deep,
	})
	outer := nestedZIPBytes(t, map[string][]byte{
		"download/abc123.zip": middle,
	})
	filename := filepath.Join(dir, "unknown-archive.zip")
	if err := os.WriteFile(filename, outer, 0o640); err != nil {
		t.Fatal(err)
	}

	evidence := inferFromArchive(filepath.Base(filename), filename)
	if evidence.Parsed.Series != "ブラックラグーン" {
		t.Fatalf("series=%q candidates=%#v evidence=%#v", evidence.Parsed.Series, evidence.Candidates, evidence.Evidence)
	}
	if !evidence.Parsed.HasVolume || evidence.Parsed.Volume != 5 {
		t.Fatalf("volume=%d has=%v", evidence.Parsed.Volume, evidence.Parsed.HasVolume)
	}
	if evidence.Source == "outer-filename" {
		t.Fatalf("deep Japanese evidence was not used: %#v", evidence)
	}
}
