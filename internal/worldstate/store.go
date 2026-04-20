package worldstate

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"
)

type Change struct {
	Entity      string `json:"entity"`
	ChangeType  string `json:"change_type"`
	Description string `json:"description"`
}

type Snapshot struct {
	ChapterFile  string    `json:"chapter_file"`
	ChapterIndex int       `json:"chapter_index"`
	GeneratedAt  time.Time `json:"generated_at"`
	Changes      []Change  `json:"changes"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	Items []*Snapshot `json:"items"`
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
	if err := json.Unmarshal(data, &s.Items); err != nil {
		return err
	}
	s.sortLocked()
	return nil
}

func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, _ := json.MarshalIndent(s.Items, "", "  ")
	return os.WriteFile(s.path, data, 0644)
}

func (s *Store) Upsert(snapshot *Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := cloneSnapshot(snapshot)
	copied.GeneratedAt = time.Now()
	replaced := false
	for i, item := range s.Items {
		if item.ChapterFile == copied.ChapterFile {
			s.Items[i] = copied
			replaced = true
			break
		}
	}
	if !replaced {
		s.Items = append(s.Items, copied)
	}
	s.sortLocked()
}

func (s *Store) GetLatestBefore(chapterIndex int) *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *Snapshot
	for _, item := range s.Items {
		if item == nil || item.ChapterIndex >= chapterIndex {
			continue
		}
		if latest == nil || item.ChapterIndex > latest.ChapterIndex || (item.ChapterIndex == latest.ChapterIndex && item.GeneratedAt.After(latest.GeneratedAt)) {
			latest = item
		}
	}
	return cloneSnapshot(latest)
}

func (s *Store) GetByChapterFile(chapterFile string) *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.Items {
		if item != nil && item.ChapterFile == chapterFile {
			return cloneSnapshot(item)
		}
	}
	return nil
}

func (s *Store) GetAll() []*Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]*Snapshot, 0, len(s.Items))
	for _, item := range s.Items {
		items = append(items, cloneSnapshot(item))
	}
	return items
}

func (s *Store) sortLocked() {
	sort.Slice(s.Items, func(i, j int) bool {
		left := s.Items[i]
		right := s.Items[j]
		if left == nil || right == nil {
			return left != nil
		}
		if left.ChapterIndex != right.ChapterIndex {
			return left.ChapterIndex < right.ChapterIndex
		}
		if left.ChapterFile != right.ChapterFile {
			return left.ChapterFile < right.ChapterFile
		}
		return left.GeneratedAt.Before(right.GeneratedAt)
	})
}

func cloneSnapshot(item *Snapshot) *Snapshot {
	if item == nil {
		return nil
	}
	changes := make([]Change, len(item.Changes))
	copy(changes, item.Changes)
	return &Snapshot{
		ChapterFile:  item.ChapterFile,
		ChapterIndex: item.ChapterIndex,
		GeneratedAt:  item.GeneratedAt,
		Changes:      changes,
	}
}
