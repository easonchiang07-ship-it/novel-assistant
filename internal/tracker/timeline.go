package tracker

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

type TimelineEvent struct {
	ID           string    `json:"id"`
	Chapter      int       `json:"chapter"`
	Scene        string    `json:"scene"`
	Description  string    `json:"description"`
	Characters   []string  `json:"characters"`
	Consequences string    `json:"consequences"`
	CreatedAt    time.Time `json:"created_at"`
}

type TimelineTracker struct {
	mu    sync.RWMutex
	Items []*TimelineEvent `json:"items"`
	path  string
}

func NewTimelineTracker(path string) *TimelineTracker {
	return &TimelineTracker{path: path}
}

func (t *TimelineTracker) Load() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	data, err := os.ReadFile(t.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &t.Items)
}

func (t *TimelineTracker) Save() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	data, _ := json.MarshalIndent(t.Items, "", "  ")
	return os.WriteFile(t.path, data, 0644)
}

func (t *TimelineTracker) Add(e *TimelineEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e.ID = fmt.Sprintf("ev_%d", time.Now().UnixNano())
	e.CreatedAt = time.Now()
	t.Items = append(t.Items, e)
}

func (t *TimelineTracker) Delete(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var filtered []*TimelineEvent
	for _, item := range t.Items {
		if item.ID != id {
			filtered = append(filtered, item)
		}
	}
	t.Items = filtered
}

func (t *TimelineTracker) GetSorted() []*TimelineEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*TimelineEvent, len(t.Items))
	copy(result, t.Items)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Chapter < result[j].Chapter
	})
	return result
}
