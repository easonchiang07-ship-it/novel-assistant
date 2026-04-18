package reviewrules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Settings struct {
	DefaultChecks []string `json:"default_checks"`
	DefaultStyles []string `json:"default_styles"`
	ReviewBias    string   `json:"review_bias"`
	RewriteBias   string   `json:"rewrite_bias"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	item Settings
}

func Defaults() Settings {
	return Settings{
		DefaultChecks: []string{"behavior"},
		ReviewBias:    "balanced",
		RewriteBias:   "faithful",
	}
}

func New(path string) *Store {
	return &Store{path: path, item: Defaults()}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		s.item = Defaults()
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &s.item); err != nil {
		return err
	}
	s.item = normalize(s.item)
	return nil
}

func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.item, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.item)
}

func (s *Store) Update(item Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.item = normalize(item)
}

func normalize(item Settings) Settings {
	def := Defaults()
	item.DefaultChecks = uniqueNonEmpty(item.DefaultChecks)
	item.DefaultStyles = uniqueNonEmpty(item.DefaultStyles)
	if len(item.DefaultChecks) == 0 {
		item.DefaultChecks = def.DefaultChecks
	}

	switch item.ReviewBias {
	case "strict", "coaching", "conservative", "balanced":
	default:
		item.ReviewBias = def.ReviewBias
	}
	switch item.RewriteBias {
	case "faithful", "expressive", "structural":
	default:
		item.RewriteBias = def.RewriteBias
	}
	return item
}

func clone(item Settings) Settings {
	item.DefaultChecks = append([]string(nil), item.DefaultChecks...)
	item.DefaultStyles = append([]string(nil), item.DefaultStyles...)
	return item
}

func uniqueNonEmpty(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
