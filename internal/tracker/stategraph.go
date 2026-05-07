package tracker

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
)

// CharacterState captures a character's status at a point in the story.
type CharacterState struct {
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
	Note   string `json:"note,omitempty"`
}

// RelationshipEdge is a directed relationship between two characters.
type RelationshipEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

// NarrativeState is a point-in-time snapshot of story state at a given chapter.
type NarrativeState struct {
	Chapter       int                       `json:"chapter"`
	Characters    map[string]CharacterState `json:"characters"`
	Relationships []RelationshipEdge        `json:"relationships"`
	ActiveFS      []string                  `json:"active_foreshadows"`
	Events        []TimelineEvent           `json:"events"`
}

// StateDelta records what changed at a given chapter.
// Deletion fields are global tombstones — they are applied across all chapters in QueryAt.
type StateDelta struct {
	Events               []TimelineEvent           `json:"events,omitempty"`
	AddedFS              []string                  `json:"added_foreshadows,omitempty"`
	ResolvedFS           []string                  `json:"resolved_foreshadows,omitempty"`
	Relationships        []RelationshipEdge        `json:"relationships,omitempty"`
	Characters           map[string]CharacterState `json:"characters,omitempty"`
	DeletedEventIDs      []string                  `json:"deleted_events,omitempty"`
	DeletedFS            []string                  `json:"deleted_foreshadows,omitempty"`
	DeletedRelationships [][2]string               `json:"deleted_relationships,omitempty"`
}

// StateGraph is the narrative state graph interface.
type StateGraph interface {
	Apply(chapter int, delta StateDelta) error
	QueryAt(chapter int) NarrativeState
	Save() error
	Load() error
}

type chapterDelta struct {
	Chapter int        `json:"chapter"`
	Delta   StateDelta `json:"delta"`
}

type stateGraphStore struct {
	Deltas []chapterDelta `json:"deltas"`
}

// JSONStateGraph persists narrative state as a sequential delta log.
type JSONStateGraph struct {
	mu     sync.RWMutex
	deltas []chapterDelta
	path   string
}

func NewJSONStateGraph(path string) *JSONStateGraph {
	return &JSONStateGraph{path: path}
}

func (g *JSONStateGraph) Apply(chapter int, delta StateDelta) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.deltas = append(g.deltas, chapterDelta{Chapter: chapter, Delta: delta})
	return nil
}

// QueryAt replays all deltas up to and including chapter, returning the accumulated state.
// Tombstones are chapter-aware: a delete recorded at chapter N is only applied for queries
// at chapter N or later, preserving the historical snapshot for earlier chapters.
func (g *JSONStateGraph) QueryAt(chapter int) NarrativeState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	sorted := make([]chapterDelta, len(g.deltas))
	copy(sorted, g.deltas)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Chapter < sorted[j].Chapter
	})

	// Pass 1: collect tombstones from chapters <= query chapter.
	deletedEvents := make(map[string]bool)
	deletedFS := make(map[string]bool)
	deletedRels := make(map[[2]string]bool)
	for _, cd := range sorted {
		if cd.Chapter > chapter {
			break
		}
		for _, id := range cd.Delta.DeletedEventIDs {
			deletedEvents[id] = true
		}
		for _, id := range cd.Delta.DeletedFS {
			deletedFS[id] = true
		}
		for _, pair := range cd.Delta.DeletedRelationships {
			deletedRels[pair] = true
		}
	}

	// Pass 2: accumulate state up to chapter, filtered by tombstones.
	state := NarrativeState{
		Chapter:    chapter,
		Characters: make(map[string]CharacterState),
	}
	fsActive := make(map[string]bool)
	relMap := make(map[[2]string]RelationshipEdge)

	for _, cd := range sorted {
		if cd.Chapter > chapter {
			break
		}
		for _, e := range cd.Delta.Events {
			if !deletedEvents[e.ID] {
				state.Events = append(state.Events, e)
			}
		}
		for _, id := range cd.Delta.AddedFS {
			if !deletedFS[id] {
				fsActive[id] = true
			}
		}
		for _, id := range cd.Delta.ResolvedFS {
			fsActive[id] = false
		}
		for _, rel := range cd.Delta.Relationships {
			key := [2]string{rel.From, rel.To}
			if !deletedRels[key] {
				relMap[key] = rel
			}
		}
		for name, cs := range cd.Delta.Characters {
			state.Characters[name] = cs
		}
	}

	for id, active := range fsActive {
		if active {
			state.ActiveFS = append(state.ActiveFS, id)
		}
	}
	sort.Strings(state.ActiveFS)

	for _, rel := range relMap {
		state.Relationships = append(state.Relationships, rel)
	}
	sort.Slice(state.Relationships, func(i, j int) bool {
		if state.Relationships[i].From != state.Relationships[j].From {
			return state.Relationships[i].From < state.Relationships[j].From
		}
		return state.Relationships[i].To < state.Relationships[j].To
	})

	return state
}

func (g *JSONStateGraph) Save() error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	data, _ := json.MarshalIndent(stateGraphStore{Deltas: g.deltas}, "", "  ")
	return os.WriteFile(g.path, data, 0644)
}

func (g *JSONStateGraph) Load() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	data, err := os.ReadFile(g.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var store stateGraphStore
	if err := json.Unmarshal(data, &store); err != nil {
		return err
	}
	g.deltas = store.Deltas
	return nil
}
