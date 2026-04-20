package worldstate

import (
	"path/filepath"
	"testing"
)

func TestStoreGetLatestBefore(t *testing.T) {
	t.Parallel()

	store := New(filepath.Join(t.TempDir(), "worldstate.json"))
	store.Upsert(&Snapshot{
		ChapterFile:  "第01章.md",
		ChapterIndex: 1,
		Changes:      []Change{{Entity: "林昊", ChangeType: "status", Description: "開始追查夜港塔"}},
	})
	store.Upsert(&Snapshot{
		ChapterFile:  "第03章.md",
		ChapterIndex: 3,
		Changes:      []Change{{Entity: "傳家寶劍", ChangeType: "lost", Description: "已賣出"}},
	})

	if got := store.GetLatestBefore(1); got != nil {
		t.Fatalf("expected no snapshot before chapter 1, got %#v", got)
	}

	got := store.GetLatestBefore(3)
	if got == nil || got.ChapterFile != "第01章.md" {
		t.Fatalf("expected chapter 1 snapshot before chapter 3, got %#v", got)
	}

	got = store.GetLatestBefore(4)
	if got == nil || got.ChapterFile != "第03章.md" {
		t.Fatalf("expected chapter 3 snapshot before chapter 4, got %#v", got)
	}
}

func TestStoreUpsertReplacesExistingChapterSnapshot(t *testing.T) {
	t.Parallel()

	store := New(filepath.Join(t.TempDir(), "worldstate.json"))
	store.Upsert(&Snapshot{
		ChapterFile:  "第02章.md",
		ChapterIndex: 2,
		Changes:      []Change{{Entity: "夜港塔", ChangeType: "moved", Description: "抵達塔頂"}},
	})
	store.Upsert(&Snapshot{
		ChapterFile:  "第02章.md",
		ChapterIndex: 2,
		Changes:      []Change{{Entity: "林昊", ChangeType: "status", Description: "受傷"}},
	})

	items := store.GetAll()
	if len(items) != 1 {
		t.Fatalf("expected single snapshot after upsert, got %d", len(items))
	}
	if len(items[0].Changes) != 1 || items[0].Changes[0].Description != "受傷" {
		t.Fatalf("expected replacement snapshot, got %#v", items[0])
	}
}
