package reviewhistory

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

type Entry struct {
	ID               string                     `json:"id"`
	Kind             string                     `json:"kind"` // review | rewrite
	ChapterTitle     string                     `json:"chapter_title"`
	ChapterFile      string                     `json:"chapter_file"`
	SceneTitle       string                     `json:"scene_title,omitempty"` // empty = full chapter
	ChapterVersion   int                        `json:"chapter_version"`
	KindVersion      int                        `json:"kind_version"`
	InputContent     string                     `json:"input_content,omitempty"`
	Checks           []string                   `json:"checks,omitempty"`
	Styles           []string                   `json:"styles,omitempty"`
	RewriteMode      string                     `json:"rewrite_mode,omitempty"`
	Sources          []string                   `json:"sources,omitempty"`
	RetrievalConfigs map[string]RetrievalConfig `json:"retrieval_configs,omitempty"`
	Result           string                     `json:"result"`
	CreatedAt        time.Time                  `json:"created_at"`
}

type RetrievalConfig struct {
	Sources   []string `json:"sources"`
	TopK      int      `json:"top_k"`
	Threshold float64  `json:"threshold"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	Items []*Entry `json:"items"`
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.Items)
}

func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := json.MarshalIndent(s.Items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *Store) Add(entry *Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.ID = fmt.Sprintf("history_%d", time.Now().UnixNano())
	entry.CreatedAt = time.Now()
	chapterKey := historyChapterKey(entry)
	entry.ChapterVersion = 1
	entry.KindVersion = 1
	for _, item := range s.Items {
		if historyChapterKey(item) != chapterKey {
			continue
		}
		if item.ChapterVersion >= entry.ChapterVersion {
			entry.ChapterVersion = item.ChapterVersion + 1
		}
		if item.Kind == entry.Kind && item.KindVersion >= entry.KindVersion {
			entry.KindVersion = item.KindVersion + 1
		}
	}
	s.Items = append(s.Items, entry)
}

func (s *Store) Recent(limit int) []*Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]*Entry, len(s.Items))
	copy(items, s.Items)
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, item := range s.Items {
		if item.ID != id {
			continue
		}
		s.Items = append(s.Items[:i], s.Items[i+1:]...)
		return true
	}
	return false
}

func (s *Store) Select(ids []string, fallbackLimit int) []*Entry {
	if len(ids) == 0 {
		return s.Recent(fallbackLimit)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	lookup := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		lookup[id] = struct{}{}
	}

	items := make([]*Entry, 0, len(ids))
	for _, item := range s.Items {
		if _, ok := lookup[item.ID]; ok {
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items
}

func (s *Store) Find(id string) *Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, item := range s.Items {
		if item.ID == id {
			cp := *item
			return &cp
		}
	}
	return nil
}

func historyChapterKey(entry *Entry) string {
	if entry == nil {
		return ""
	}
	var base string
	if name := entry.ChapterFile; name != "" {
		base = "file:" + name
	} else {
		base = "title:" + entry.ChapterTitle
	}
	if scene := entry.SceneTitle; scene != "" {
		return base + "/scene:" + scene
	}
	return base
}
