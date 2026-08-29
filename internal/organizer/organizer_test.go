package organizer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanRejectsSourceSymlink(t *testing.T) {
	root := t.TempDir()
	realFile := filepath.Join(root, "real.zip")
	if err := os.WriteFile(realFile, []byte("not-a-real-zip"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realFile, filepath.Join(root, "link.zip")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	o, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.PlanName("link.zip"); err == nil {
		t.Fatal("expected source symlink to be rejected")
	}
}

func TestPlanRejectsDestinationSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Series 第01巻.zip"), []byte("not-a-real-zip"), 0o640); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "Series")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	o, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.PlanName("Series 第01巻.zip"); err == nil {
		t.Fatal("expected destination symlink to be rejected")
	}
}

func TestNewRequiresExistingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := New(Config{Root: missing}); err == nil {
		t.Fatal("expected missing root to be rejected")
	}
}
