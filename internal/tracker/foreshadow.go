package tracker

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type Foreshadowing struct {
	ID              string    `json:"id"`
	Chapter         int       `json:"chapter"`
	Description     string    `json:"description"`
	PlantedIn       string    `json:"planted_in"`
	ResolvedIn      string    `json:"resolved_in"`
	Status          string    `json:"status"` // "未回收" | "已回收"
	LastSeenChapter int       `json:"last_seen_chapter,omitempty"`
	Confidence      string    `json:"confidence,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// PendingHook is an LLM-suggested hook awaiting user confirmation.
type PendingHook struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Context     string `json:"context"`
	Confidence  string `json:"confidence"`
}

type foreshadowStore struct {
	Items   []*Foreshadowing `json:"items"`
	Pending []*PendingHook   `json:"pending,omitempty"`
}

type ForeshadowTracker struct {
	mu      sync.RWMutex
	Items   []*Foreshadowing `json:"items"`
	Pending []*PendingHook   `json:"pending,omitempty"`
	path    string
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
	// Try new wrapper format first; fall back to legacy array format.
	var store foreshadowStore
	if json.Unmarshal(data, &store) == nil && (store.Items != nil || store.Pending != nil) {
		t.Items = store.Items
		t.Pending = store.Pending
		return nil
	}
	return json.Unmarshal(data, &t.Items)
}

func (t *ForeshadowTracker) Save() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	data, _ := json.MarshalIndent(foreshadowStore{Items: t.Items, Pending: t.Pending}, "", "  ")
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

// AddPending appends LLM-suggested hooks awaiting user confirmation.
func (t *ForeshadowTracker) AddPending(hooks []PendingHook) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range hooks {
		h := hooks[i]
		h.ID = fmt.Sprintf("ph_%d_%d", time.Now().UnixNano(), i)
		t.Pending = append(t.Pending, &h)
	}
}

// GetPending returns a copy of all pending hooks.
func (t *ForeshadowTracker) GetPending() []PendingHook {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]PendingHook, len(t.Pending))
	for i, p := range t.Pending {
		out[i] = *p
	}
	return out
}

// ConfirmPending moves a pending hook into the confirmed Items list.
// Returns false if the id is not found.
func (t *ForeshadowTracker) ConfirmPending(id string, chapter int, plantedIn string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	var remaining []*PendingHook
	found := false
	for _, p := range t.Pending {
		if p.ID == id {
			found = true
			f := &Foreshadowing{
				ID:          fmt.Sprintf("fs_%d", time.Now().UnixNano()),
				Chapter:     chapter,
				Description: p.Description,
				PlantedIn:   plantedIn,
				Status:      "未回收",
				Confidence:  p.Confidence,
				CreatedAt:   time.Now(),
			}
			t.Items = append(t.Items, f)
		} else {
			remaining = append(remaining, p)
		}
	}
	t.Pending = remaining
	return found
}

// DismissPending removes a pending hook without adding it to Items.
func (t *ForeshadowTracker) DismissPending(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	var remaining []*PendingHook
	found := false
	for _, p := range t.Pending {
		if p.ID == id {
			found = true
		} else {
			remaining = append(remaining, p)
		}
	}
	t.Pending = remaining
	return found
}

// StaleForeshadows returns unresolved hooks whose LastSeenChapter is more
// than threshold chapters before currentChapter.
// Hooks with LastSeenChapter == 0 use their planted Chapter as baseline.
func (t *ForeshadowTracker) StaleForeshadows(currentChapter, threshold int) []*Foreshadowing {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var out []*Foreshadowing
	for _, item := range t.Items {
		if item.Status == "已回收" {
			continue
		}
		baseline := item.LastSeenChapter
		if baseline == 0 {
			baseline = item.Chapter
		}
		if currentChapter-baseline >= threshold {
			out = append(out, item)
		}
	}
	return out
}
