package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorePersistsSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, err := Open(path, Settings{Root: "/share/Download/Temp"})
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{Root: "/share/Manga"}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	if got := s.Get(); got.Root != want.Root {
		t.Fatalf("Get root=%q want=%q", got.Root, want.Root)
	}
	s2, err := Open(path, Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Get(); got.Root != want.Root {
		t.Fatalf("reloaded root=%q want=%q", got.Root, want.Root)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("settings permissions too broad: mode=%v", st.Mode())
	}
}
