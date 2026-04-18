package tracker

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type Foreshadowing struct {
	ID          string    `json:"id"`
	Chapter     int       `json:"chapter"`
	Description string    `json:"description"`
	PlantedIn   string    `json:"planted_in"`
	ResolvedIn  string    `json:"resolved_in"`
	Status      string    `json:"status"` // "未回收" | "已回收"
	CreatedAt   time.Time `json:"created_at"`
}

type ForeshadowTracker struct {
	mu    sync.RWMutex
	Items []*Foreshadowing `json:"items"`
	path  string
}

func NewForeshadowTracker(path string) *ForeshadowTracker {
	return &ForeshadowTracker{path: path}
}

func (t *ForeshadowTracker) Load() error {
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

func (t *ForeshadowTracker) Save() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	data, _ := json.MarshalIndent(t.Items, "", "  ")
	return os.WriteFile(t.path, data, 0644)
}

func (t *ForeshadowTracker) Add(f *Foreshadowing) {
	t.mu.Lock()
	defer t.mu.Unlock()
	f.ID = fmt.Sprintf("fs_%d", time.Now().UnixNano())
	f.Status = "未回收"
	f.CreatedAt = time.Now()
	t.Items = append(t.Items, f)
}

func (t *ForeshadowTracker) Resolve(id, resolvedIn string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, item := range t.Items {
		if item.ID == id {
			item.Status = "已回收"
			item.ResolvedIn = resolvedIn
			return true
		}
	}
	return false
}

func (t *ForeshadowTracker) Delete(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var filtered []*Foreshadowing
	for _, item := range t.Items {
		if item.ID != id {
			filtered = append(filtered, item)
		}
	}
	t.Items = filtered
}

func (t *ForeshadowTracker) GetAll() []*Foreshadowing {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*Foreshadowing, len(t.Items))
	copy(result, t.Items)
	return result
}
