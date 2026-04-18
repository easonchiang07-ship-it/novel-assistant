package tracker

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Relationship struct {
	From         string    `json:"from"`
	To           string    `json:"to"`
	Status       string    `json:"status"`
	Note         string    `json:"note"`
	TriggerEvent string    `json:"trigger_event"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type RelationshipTracker struct {
	mu    sync.RWMutex
	Items []*Relationship `json:"items"`
	path  string
}

func NewRelationshipTracker(path string) *RelationshipTracker {
	return &RelationshipTracker{path: path}
}

func (t *RelationshipTracker) Load() error {
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

func (t *RelationshipTracker) Save() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	data, _ := json.MarshalIndent(t.Items, "", "  ")
	return os.WriteFile(t.path, data, 0644)
}

func (t *RelationshipTracker) Upsert(r *Relationship) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r.UpdatedAt = time.Now()
	for i, item := range t.Items {
		if item.From == r.From && item.To == r.To {
			t.Items[i] = r
			return
		}
	}
	t.Items = append(t.Items, r)
}

func (t *RelationshipTracker) Delete(from, to string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var filtered []*Relationship
	for _, item := range t.Items {
		if !(item.From == from && item.To == to) {
			filtered = append(filtered, item)
		}
	}
	t.Items = filtered
}

func (t *RelationshipTracker) GetAll() []*Relationship {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*Relationship, len(t.Items))
	copy(result, t.Items)
	return result
}
