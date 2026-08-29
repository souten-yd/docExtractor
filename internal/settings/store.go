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
	Root          string            `json:"root"`
	SeriesAliases map[string]string `json:"series_aliases,omitempty"`
}

type Store struct {
	mu      sync.RWMutex
	path    string
	current Settings
}

func Open(path string, defaults Settings) (*Store, error) {
	defaults.SeriesAliases = cloneAliases(defaults.SeriesAliases)
	s := &Store{path: filepath.Clean(path), current: defaults}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) { return s, nil }
	if err != nil { return nil, err }
	var loaded Settings
	if err := json.Unmarshal(data, &loaded); err != nil { return nil, err }
	if strings.TrimSpace(loaded.Root) != "" { s.current.Root = filepath.Clean(strings.TrimSpace(loaded.Root)) }
	if loaded.SeriesAliases != nil { s.current.SeriesAliases = cloneAliases(loaded.SeriesAliases) }
	return s, nil
}

func New(path string, current Settings) *Store {
	current.SeriesAliases = cloneAliases(current.SeriesAliases)
	return &Store{path: filepath.Clean(path), current: current}
}

func (s *Store) Get() Settings {
	s.mu.RLock(); defer s.mu.RUnlock(); out:=s.current; out.SeriesAliases=cloneAliases(s.current.SeriesAliases); return out
}

func (s *Store) Save(next Settings) error {
	next.Root = filepath.Clean(strings.TrimSpace(next.Root)); next.SeriesAliases=cloneAliases(next.SeriesAliases)
	data, err := json.MarshalIndent(next, "", "  "); if err != nil { return err }; data=append(data,'\n')
	if err := os.MkdirAll(filepath.Dir(s.path),0o750);err!=nil{return err}
	tmp:=s.path+".tmp"; f,err:=os.OpenFile(tmp,os.O_CREATE|os.O_TRUNC|os.O_WRONLY,0o600);if err!=nil{return err}
	ok:=false; defer func(){_ = f.Close();if !ok{_ = os.Remove(tmp)}}()
	if _,err:=f.Write(data);err!=nil{return err};if err:=f.Sync();err!=nil{return err};if err:=f.Close();err!=nil{return err};if err:=os.Rename(tmp,s.path);err!=nil{return err}
	if dir,err:=os.Open(filepath.Dir(s.path));err==nil{_ = dir.Sync();_ = dir.Close()}
	s.mu.Lock();s.current=next;s.mu.Unlock();ok=true;return nil
}

func cloneAliases(in map[string]string) map[string]string {
	if len(in)==0{return map[string]string{}}
	out:=make(map[string]string,len(in));for k,v:=range in{k=strings.TrimSpace(k);v=strings.TrimSpace(v);if k!=""&&v!=""{out[k]=v}};return out
}
