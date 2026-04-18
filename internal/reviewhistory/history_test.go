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

func TestAddSceneVersionsAreIsolatedFromChapterVersions(t *testing.T) {
	t.Parallel()

	store := &Store{}
	// Full-chapter review
	store.Add(&Entry{Kind: "review", ChapterFile: "第01章.md"})
	// Scene-scoped reviews — should start their own version counters
	store.Add(&Entry{Kind: "review", ChapterFile: "第01章.md", SceneTitle: "Scene 1: Opening"})
	store.Add(&Entry{Kind: "review", ChapterFile: "第01章.md", SceneTitle: "Scene 1: Opening"})
	// Different scene — own counter again
	store.Add(&Entry{Kind: "review", ChapterFile: "第01章.md", SceneTitle: "Scene 2: Rain"})

	chapterReview := store.Items[0]
	scene1First := store.Items[1]
	scene1Second := store.Items[2]
	scene2First := store.Items[3]

	if chapterReview.ChapterVersion != 1 || chapterReview.KindVersion != 1 {
		t.Fatalf("full-chapter review: unexpected versions %d/%d", chapterReview.ChapterVersion, chapterReview.KindVersion)
	}
	if scene1First.ChapterVersion != 1 || scene1First.KindVersion != 1 {
		t.Fatalf("scene1 first: expected 1/1, got %d/%d", scene1First.ChapterVersion, scene1First.KindVersion)
	}
	if scene1Second.ChapterVersion != 2 || scene1Second.KindVersion != 2 {
		t.Fatalf("scene1 second: expected 2/2, got %d/%d", scene1Second.ChapterVersion, scene1Second.KindVersion)
	}
	if scene2First.ChapterVersion != 1 || scene2First.KindVersion != 1 {
		t.Fatalf("scene2 first: expected 1/1, got %d/%d", scene2First.ChapterVersion, scene2First.KindVersion)
	}
}
