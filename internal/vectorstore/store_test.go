package vectorstore

import (
	"path/filepath"
	"testing"
)

func TestQueryFilteredScoredSupportsThresholdAndTypeFilters(t *testing.T) {
	t.Parallel()

	store := &Store{
		docs: []Document{
			{ID: "char_1", Type: "character", Embedding: []float64{1, 0}},
			{ID: "world_1", Type: "world", Embedding: []float64{0.8, 0.2}},
			{ID: "style_1", Type: "style", Embedding: []float64{0, 1}},
		},
	}
	query := []float64{1, 0}

	all := store.QueryFilteredScored(query, 10, nil, 0)
	if len(all) != 3 {
		t.Fatalf("expected all documents with threshold 0, got %d", len(all))
	}

	charactersOnly := store.QueryFilteredScored(query, 10, []string{"character"}, 0)
	if len(charactersOnly) != 1 || charactersOnly[0].Type != "character" {
		t.Fatalf("expected only character docs, got %#v", charactersOnly)
	}

	highThreshold := store.QueryFilteredScored(query, 10, nil, 0.9)
	if len(highThreshold) != 2 {
		t.Fatalf("expected threshold to filter low-similarity docs, got %d", len(highThreshold))
	}
	for _, item := range highThreshold {
		if item.Score < 0.9 {
			t.Fatalf("expected score >= 0.9, got %f", item.Score)
		}
	}
}

func TestQueryFilteredScoredRespectsTopK(t *testing.T) {
	t.Parallel()

	store := &Store{
		docs: []Document{
			{ID: "a", Type: "character", Embedding: []float64{1, 0}},
			{ID: "b", Type: "world", Embedding: []float64{0.9, 0.1}},
			{ID: "c", Type: "style", Embedding: []float64{0.8, 0.2}},
		},
	}

	results := store.QueryFilteredScored([]float64{1, 0}, 2, nil, 0)
	if len(results) != 2 {
		t.Fatalf("expected top-k cap of 2, got %d", len(results))
	}
	if results[0].Score < results[1].Score {
		t.Fatalf("expected scores sorted descending, got %#v", results)
	}
}

func TestDocumentMetadataSurvivesSaveLoad(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "store.json")
	store := New(path)
	store.Upsert(Document{
		ID:           "chapter_第03章.md_scene_1",
		Type:         "chapter",
		Content:      "雨落下來。",
		Embedding:    []float64{0.1, 0.2},
		ChapterFile:  "第03章.md",
		ChapterIndex: 3,
		SceneIndex:   1,
		ChunkType:    "scene",
	})
	if err := store.Save(); err != nil {
		t.Fatalf("save store: %v", err)
	}

	loaded := New(path)
	if err := loaded.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	items := loaded.QueryFilteredScored([]float64{0.1, 0.2}, 1, []string{"chapter"}, 0)
	if len(items) != 1 {
		t.Fatalf("expected 1 document, got %d", len(items))
	}
	if items[0].ChapterFile != "第03章.md" || items[0].ChapterIndex != 3 || items[0].SceneIndex != 1 || items[0].ChunkType != "scene" {
		t.Fatalf("unexpected metadata after reload: %#v", items[0].Document)
	}
}
