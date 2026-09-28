// Package store keeps the account in accounts.json inside the data directory
// (see DataDir). The file holds session cookies in plain text.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"lastedlive/internal/models"
)

type Store struct {
	dir  string
	file string
	mu   sync.RWMutex
	m    map[string]*models.Account
}

// DataDir picks the data directory:
//   - $LASTEDLIVE_DATA if set
//   - a data folder next to the executable, if it already has accounts (portable use)
//   - otherwise the user config dir (%APPDATA%\LastedLiveFree on Windows)
func DataDir() string {
	if d := os.Getenv("LASTEDLIVE_DATA"); d != "" {
		return d
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "data")
		if _, err := os.Stat(filepath.Join(p, "accounts.json")); err == nil {
			return p
		}
	}
	if cfg, err := os.UserConfigDir(); err == nil {
		return filepath.Join(cfg, "LastedLiveFree")
	}
	return "data"
}

func New() (*Store, error) {
	dir := DataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, file: filepath.Join(dir, "accounts.json"), m: map[string]*models.Account{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Dir is the data directory; scenes, logs and caches live there too.
func (s *Store) Dir() string { return s.dir }

func (s *Store) load() error {
	raw, err := os.ReadFile(s.file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var list []*models.Account
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	// The free version runs a single account. A file copied over from the full
	// version may hold more; only the first one is used.
	if len(list) > 1 {
		list = list[:1]
	}
	for _, a := range list {
		if a.Cookies == nil {
			a.Cookies = map[string]string{}
		}
		s.m[a.ID] = a
	}
	return nil
}

func (s *Store) save() error {
	list := make([]*models.Account, 0, len(s.m))
	for _, a := range s.m {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.file + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.file)
}

// All returns every account, sorted by id. The pointers are shared, so
// callers shouldn't modify them without going through Upsert.
func (s *Store) All() []*models.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]*models.Account, 0, len(s.m))
	for _, a := range s.m {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

func (s *Store) Get(id string) *models.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m[id]
}

func (s *Store) Upsert(a *models.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.Cookies == nil {
		a.Cookies = map[string]string{}
	}
	s.m[a.ID] = a
	return s.save()
}

func (s *Store) Delete(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[id]; !ok {
		return false, nil
	}
	delete(s.m, id)
	return true, s.save()
}
