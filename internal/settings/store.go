package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Settings struct {
	Root string `json:"root"`
}

type Store struct {
	mu      sync.RWMutex
	path    string
	current Settings
}

func Open(path string, defaults Settings) (*Store, error) {
	s := &Store{path: filepath.Clean(path), current: defaults}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var loaded Settings
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, err
	}
	if strings.TrimSpace(loaded.Root) != "" {
		s.current.Root = filepath.Clean(strings.TrimSpace(loaded.Root))
	}
	return s, nil
}

// New creates a store without reading the existing file. It is used as a
// safe fallback when a damaged settings file should not prevent the Web UI
// from starting.
func New(path string, current Settings) *Store {
	return &Store{path: filepath.Clean(path), current: current}
}

func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *Store) Save(next Settings) error {
	next.Root = filepath.Clean(strings.TrimSpace(next.Root))
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	if dir, err := os.Open(filepath.Dir(s.path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}

	s.mu.Lock()
	s.current = next
	s.mu.Unlock()
	ok = true
	return nil
}
