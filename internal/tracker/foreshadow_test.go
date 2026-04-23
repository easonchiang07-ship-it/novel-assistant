package tracker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestForeshadowTrackerSaveLoadRoundtrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "foreshadow.json")
	tr := NewForeshadowTracker(path)
	tr.Add(&Foreshadowing{Chapter: 2, Description: "神秘信封", PlantedIn: "桌上的信"})
	hooks := []PendingHook{{Description: "破損徽章", Context: "地板上", Confidence: "高"}}
	tr.AddPending(hooks)
	if err := tr.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	tr2 := NewForeshadowTracker(path)
	if err := tr2.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tr2.Items) != 1 || tr2.Items[0].Description != "神秘信封" {
		t.Fatalf("unexpected items: %#v", tr2.Items)
	}
	if len(tr2.Pending) != 1 || tr2.Pending[0].Description != "破損徽章" {
		t.Fatalf("unexpected pending: %#v", tr2.Pending)
	}
}

func TestForeshadowTrackerLegacyArrayLoad(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "foreshadow.json")
	// Write old-format JSON (plain array)
	old := []*Foreshadowing{{ID: "fs_old", Chapter: 1, Description: "古老伏筆", Status: "未回收"}}
	data, _ := json.MarshalIndent(old, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	tr := NewForeshadowTracker(path)
	if err := tr.Load(); err != nil {
		t.Fatalf("load legacy: %v", err)
	}
	if len(tr.Items) != 1 || tr.Items[0].Description != "古老伏筆" {
		t.Fatalf("unexpected items after legacy load: %#v", tr.Items)
	}
}

func TestConfirmPendingMovesToItems(t *testing.T) {
	t.Parallel()

	tr := NewForeshadowTracker("")
	tr.AddPending([]PendingHook{{Description: "遺失的鑰匙", Context: "抽屜", Confidence: "中"}})
	pending := tr.GetPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	id := pending[0].ID

	if !tr.ConfirmPending(id, 3, "林昊隨手扔進口袋") {
		t.Fatal("expected ConfirmPending to return true")
	}
	if len(tr.GetPending()) != 0 {
		t.Fatal("pending should be empty after confirm")
	}
	items := tr.GetAll()
	if len(items) != 1 || items[0].Description != "遺失的鑰匙" {
		t.Fatalf("unexpected items after confirm: %#v", items)
	}
	if items[0].Chapter != 3 || items[0].PlantedIn != "林昊隨手扔進口袋" {
		t.Fatalf("unexpected item fields: %#v", items[0])
	}
}

func TestDismissPendingRemovesWithoutAdding(t *testing.T) {
	t.Parallel()

	tr := NewForeshadowTracker("")
	tr.AddPending([]PendingHook{{Description: "神秘聲音", Confidence: "低"}})
	id := tr.GetPending()[0].ID

	if !tr.DismissPending(id) {
		t.Fatal("expected DismissPending to return true")
	}
	if len(tr.GetPending()) != 0 {
		t.Fatal("pending should be empty after dismiss")
	}
	if len(tr.GetAll()) != 0 {
		t.Fatal("items should remain empty after dismiss")
	}
}

func TestStaleForeshadows(t *testing.T) {
	t.Parallel()

	tr := NewForeshadowTracker("")
	// planted ch1, not seen since → stale when current=5, threshold=3
	tr.Add(&Foreshadowing{Chapter: 1, Description: "舊伏筆"})
	// planted ch4, seen at ch4 → not stale at current=5
	tr.Add(&Foreshadowing{Chapter: 4, Description: "新伏筆", LastSeenChapter: 4})
	// resolved → not counted
	tr.Add(&Foreshadowing{Chapter: 1, Description: "已回收", Status: "已回收"})

	// Fix status of first two (Add sets status)
	tr.Items[2].Status = "已回收"

	stale := tr.StaleForeshadows(5, 3)
	if len(stale) != 1 || stale[0].Description != "舊伏筆" {
		t.Fatalf("expected only 舊伏筆, got %#v", stale)
	}
}

func TestConfirmPendingReturnsFalseForMissingID(t *testing.T) {
	t.Parallel()

	tr := NewForeshadowTracker("")
	if tr.ConfirmPending("nonexistent", 1, "") {
		t.Fatal("expected false for missing id")
	}
}
