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

func TestInputOutputModeFollowsChangedRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := New(path, Settings{Root: "/share/Download/Temp", OutputMode: OutputModeInput})
	if err := s.Save(Settings{Root: "/share/Download/Manga", OutputRoot: "/share/Download/Temp", OutputMode: OutputModeInput}); err != nil {
		t.Fatal(err)
	}
	got := s.Get()
	if got.OutputRoot != got.Root || got.OutputRoot != "/share/Download/Manga" {
		t.Fatalf("input output mode did not follow root: %+v", got)
	}
}

func TestLegacySeparateOutputRemainsCustom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := []byte("{\n  \"root\": \"/share/Incoming\",\n  \"output_root\": \"/share/Manga\"\n}\n")
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path, Settings{})
	if err != nil {
		t.Fatal(err)
	}
	got := s.Get()
	if got.OutputMode != OutputModeCustom || got.OutputRoot != "/share/Manga" {
		t.Fatalf("legacy custom output was not preserved: %+v", got)
	}
}

func TestLegacyDefaultOutputMigratesToInputMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := []byte("{\n  \"root\": \"/share/Manga\",\n  \"output_root\": \"/share/Download/Temp\"\n}\n")
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path, Settings{Root: "/share/Download/Temp"})
	if err != nil {
		t.Fatal(err)
	}
	got := s.Get()
	if got.OutputMode != OutputModeInput || got.OutputRoot != "/share/Manga" {
		t.Fatalf("legacy default output did not follow the selected input: %+v", got)
	}
}
