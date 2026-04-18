package reviewhistory

import (
	"testing"
	"time"
)

func TestDeleteRemovesEntry(t *testing.T) {
	t.Parallel()

	store := &Store{
		Items: []*Entry{
			{ID: "a"},
			{ID: "b"},
		},
	}

	if !store.Delete("a") {
		t.Fatal("expected delete to return true")
	}
	if len(store.Items) != 1 || store.Items[0].ID != "b" {
		t.Fatalf("unexpected items after delete: %#v", store.Items)
	}
}

func TestSelectReturnsRequestedEntriesSortedByTime(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := &Store{
		Items: []*Entry{
			{ID: "older", CreatedAt: now.Add(-time.Hour)},
			{ID: "newer", CreatedAt: now},
			{ID: "other", CreatedAt: now.Add(-2 * time.Hour)},
		},
	}

	items := store.Select([]string{"older", "newer"}, 10)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != "newer" || items[1].ID != "older" {
		t.Fatalf("unexpected order: %#v", items)
	}
}

func TestAddAssignsChapterAndKindVersions(t *testing.T) {
	t.Parallel()

	store := &Store{}
	store.Add(&Entry{Kind: "review", ChapterFile: "第01章.md"})
	store.Add(&Entry{Kind: "review", ChapterFile: "第01章.md"})
	store.Add(&Entry{Kind: "rewrite", ChapterFile: "第01章.md"})

	if store.Items[0].ChapterVersion != 1 || store.Items[0].KindVersion != 1 {
		t.Fatalf("unexpected first entry versions: %#v", store.Items[0])
	}
	if store.Items[1].ChapterVersion != 2 || store.Items[1].KindVersion != 2 {
		t.Fatalf("unexpected second entry versions: %#v", store.Items[1])
	}
	if store.Items[2].ChapterVersion != 3 || store.Items[2].KindVersion != 1 {
		t.Fatalf("unexpected third entry versions: %#v", store.Items[2])
	}
}
