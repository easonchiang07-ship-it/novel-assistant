package reviewrules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Settings struct {
	DefaultChecks      []string                   `json:"default_checks"`
	DefaultStyles      []string                   `json:"default_styles"`
	ReviewBias         string                     `json:"review_bias"`
	RewriteBias        string                     `json:"rewrite_bias"`
	RetrievalSources   []string                   `json:"retrieval_sources"`
	RetrievalTopK      int                        `json:"retrieval_top_k"`
	RetrievalThreshold float64                    `json:"retrieval_threshold"`
	Presets            map[string]RetrievalPreset `json:"presets,omitempty"`
}

type RetrievalPreset struct {
	Sources   []string `json:"sources"`
	TopK      int      `json:"top_k"`
	Threshold float64  `json:"threshold"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	item Settings
}

func Defaults() Settings {
	return Settings{
		DefaultChecks:      []string{"behavior"},
		ReviewBias:         "balanced",
		RewriteBias:        "faithful",
		RetrievalSources:   []string{"character", "world", "style"},
		RetrievalTopK:      4,
		RetrievalThreshold: 0,
		Presets: map[string]RetrievalPreset{
			"behavior": {Sources: []string{"character", "world"}, TopK: 4, Threshold: 0},
			"dialogue": {Sources: []string{"character"}, TopK: 3, Threshold: 0},
			"world":    {Sources: []string{"world"}, TopK: 4, Threshold: 0},
			"rewrite":  {Sources: []string{"character", "world", "style"}, TopK: 5, Threshold: 0},
		},
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

	allowed := map[string]struct{}{
		"character": {},
		"world":     {},
		"style":     {},
	}
	filtered := make([]string, 0, len(item.RetrievalSources))
	for _, source := range uniqueNonEmpty(item.RetrievalSources) {
		if _, ok := allowed[source]; ok {
			filtered = append(filtered, source)
		}
	}
	if len(filtered) == 0 {
		item.RetrievalSources = append([]string(nil), def.RetrievalSources...)
	} else {
		item.RetrievalSources = filtered
	}
	if item.RetrievalTopK < 1 || item.RetrievalTopK > 20 {
		item.RetrievalTopK = def.RetrievalTopK
	}
	if item.RetrievalThreshold < 0 || item.RetrievalThreshold > 1 {
		item.RetrievalThreshold = def.RetrievalThreshold
	}

	validPresetKeys := map[string]struct{}{
		"behavior": {},
		"dialogue": {},
		"world":    {},
		"rewrite":  {},
	}
	if item.Presets == nil {
		item.Presets = make(map[string]RetrievalPreset)
	}
	for key, preset := range item.Presets {
		if _, ok := validPresetKeys[key]; !ok {
			delete(item.Presets, key)
			continue
		}
		preset.Sources = normalizePresetSources(preset.Sources, def.RetrievalSources, allowed)
		if preset.TopK < 1 || preset.TopK > 20 {
			preset.TopK = def.RetrievalTopK
		}
		if preset.Threshold < 0 || preset.Threshold > 1 {
			preset.Threshold = def.RetrievalThreshold
		}
		item.Presets[key] = preset
	}
	for key, preset := range def.Presets {
		if _, ok := item.Presets[key]; ok {
			continue
		}
		preset.Sources = append([]string(nil), preset.Sources...)
		item.Presets[key] = preset
	}
	return item
}

func clone(item Settings) Settings {
	item.DefaultChecks = append([]string(nil), item.DefaultChecks...)
	item.DefaultStyles = append([]string(nil), item.DefaultStyles...)
	item.RetrievalSources = append([]string(nil), item.RetrievalSources...)
	if item.Presets != nil {
		cloned := make(map[string]RetrievalPreset, len(item.Presets))
		for key, preset := range item.Presets {
			preset.Sources = append([]string(nil), preset.Sources...)
			cloned[key] = preset
		}
		item.Presets = cloned
	}
	return item
}

// PresetFor returns the retrieval preset for a task, falling back to global settings.
func (s *Store) PresetFor(task string) RetrievalPreset {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if preset, ok := s.item.Presets[task]; ok {
		preset.Sources = append([]string(nil), preset.Sources...)
		return preset
	}
	return RetrievalPreset{
		Sources:   append([]string(nil), s.item.RetrievalSources...),
		TopK:      s.item.RetrievalTopK,
		Threshold: s.item.RetrievalThreshold,
	}
}

func normalizePresetSources(sources, fallback []string, allowed map[string]struct{}) []string {
	filtered := make([]string, 0, len(sources))
	for _, source := range uniqueNonEmpty(sources) {
		if _, ok := allowed[source]; ok {
			filtered = append(filtered, source)
		}
	}
	if len(filtered) == 0 {
		return append([]string(nil), fallback...)
	}
	return filtered
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
