package vectorstore

import "testing"

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
