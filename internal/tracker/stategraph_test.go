package tracker_test

import (
	"os"
	"path/filepath"
	"testing"

	"novel-assistant/internal/tracker"
)

func TestStateGraphQueryAtEmpty(t *testing.T) {
	g := tracker.NewJSONStateGraph("")
	state := g.QueryAt(5)
	if state.Chapter != 5 {
		t.Errorf("expected chapter 5, got %d", state.Chapter)
	}
	if len(state.Events) != 0 || len(state.ActiveFS) != 0 || len(state.Relationships) != 0 {
		t.Errorf("expected empty state, got %+v", state)
	}
}

func TestStateGraphQueryAtAccumulatesEvents(t *testing.T) {
	g := tracker.NewJSONStateGraph("")
	g.Apply(1, tracker.StateDelta{Events: []tracker.TimelineEvent{{ID: "e1", Chapter: 1, Description: "first"}}})
	g.Apply(3, tracker.StateDelta{Events: []tracker.TimelineEvent{{ID: "e3", Chapter: 3, Description: "third"}}})

	at2 := g.QueryAt(2)
	if len(at2.Events) != 1 || at2.Events[0].ID != "e1" {
		t.Errorf("chapter 2 should only have e1, got %+v", at2.Events)
	}

	at3 := g.QueryAt(3)
	if len(at3.Events) != 2 {
		t.Errorf("chapter 3 should have 2 events, got %d", len(at3.Events))
	}
}

func TestStateGraphActiveForeshadows(t *testing.T) {
	g := tracker.NewJSONStateGraph("")
	g.Apply(1, tracker.StateDelta{AddedFS: []string{"fs1", "fs2"}})
	g.Apply(3, tracker.StateDelta{ResolvedFS: []string{"fs1"}})

	at2 := g.QueryAt(2)
	if len(at2.ActiveFS) != 2 {
		t.Errorf("chapter 2: expected 2 active, got %v", at2.ActiveFS)
	}

	at3 := g.QueryAt(3)
	if len(at3.ActiveFS) != 1 || at3.ActiveFS[0] != "fs2" {
		t.Errorf("chapter 3: expected only fs2 active, got %v", at3.ActiveFS)
	}
}

func TestStateGraphRelationshipUpsert(t *testing.T) {
	g := tracker.NewJSONStateGraph("")
	g.Apply(1, tracker.StateDelta{Relationships: []tracker.RelationshipEdge{
		{From: "A", To: "B", Status: "敵對"},
	}})
	g.Apply(3, tracker.StateDelta{Relationships: []tracker.RelationshipEdge{
		{From: "A", To: "B", Status: "和解"},
	}})

	at1 := g.QueryAt(1)
	if len(at1.Relationships) != 1 || at1.Relationships[0].Status != "敵對" {
		t.Errorf("chapter 1: expected 敵對, got %+v", at1.Relationships)
	}

	at3 := g.QueryAt(3)
	if len(at3.Relationships) != 1 || at3.Relationships[0].Status != "和解" {
		t.Errorf("chapter 3: expected 和解, got %+v", at3.Relationships)
	}
}

func TestStateGraphCharacterState(t *testing.T) {
	g := tracker.NewJSONStateGraph("")
	g.Apply(2, tracker.StateDelta{Characters: map[string]tracker.CharacterState{
		"Alice": {Name: "Alice", Status: "alive"},
	}})
	g.Apply(5, tracker.StateDelta{Characters: map[string]tracker.CharacterState{
		"Alice": {Name: "Alice", Status: "dead"},
	}})

	at4 := g.QueryAt(4)
	if at4.Characters["Alice"].Status != "alive" {
		t.Errorf("chapter 4: expected alive, got %s", at4.Characters["Alice"].Status)
	}

	at5 := g.QueryAt(5)
	if at5.Characters["Alice"].Status != "dead" {
		t.Errorf("chapter 5: expected dead, got %s", at5.Characters["Alice"].Status)
	}
}

func TestStateGraphExcludesFutureDeltas(t *testing.T) {
	g := tracker.NewJSONStateGraph("")
	g.Apply(10, tracker.StateDelta{Events: []tracker.TimelineEvent{{ID: "future", Chapter: 10}}})

	at5 := g.QueryAt(5)
	if len(at5.Events) != 0 {
		t.Errorf("chapter 5 should not see chapter-10 events, got %v", at5.Events)
	}
}

func TestStateGraphSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sg.json")

	g := tracker.NewJSONStateGraph(path)
	g.Apply(1, tracker.StateDelta{AddedFS: []string{"fs1"}})
	g.Apply(2, tracker.StateDelta{Events: []tracker.TimelineEvent{{ID: "e1", Chapter: 2}}})
	if err := g.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	g2 := tracker.NewJSONStateGraph(path)
	if err := g2.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	state := g2.QueryAt(2)
	if len(state.ActiveFS) != 1 || state.ActiveFS[0] != "fs1" {
		t.Errorf("after reload: expected fs1 active, got %v", state.ActiveFS)
	}
	if len(state.Events) != 1 || state.Events[0].ID != "e1" {
		t.Errorf("after reload: expected e1 event, got %v", state.Events)
	}
}

func TestStateGraphLoadMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	g := tracker.NewJSONStateGraph(path)
	if err := g.Load(); err != nil {
		t.Errorf("loading missing file should be a no-op, got: %v", err)
	}
}

func TestStateGraphSaveCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sg.json")
	g := tracker.NewJSONStateGraph(path)
	g.Apply(1, tracker.StateDelta{AddedFS: []string{"x"}})
	if err := g.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist after save: %v", err)
	}
}

var _ tracker.StateGraph = tracker.NewJSONStateGraph("")
