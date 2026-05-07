package vectorstore

import (
	"fmt"
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"林昊 is the hero", []string{"林", "昊", "is", "the", "hero"}},
		{"台北市中山區", []string{"台", "北", "市", "中", "山", "區"}},
		{"go1.21", []string{"go1", "21"}},
		{"", nil},
	}
	for _, tc := range tests {
		got := tokenize(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("tokenize(%q): got %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("tokenize(%q)[%d]: got %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestBM25ScoreKeywordMatch(t *testing.T) {
	docs := []Document{
		{ID: "char_林昊", Type: "character", Content: "林昊是主角，林昊很強"},
		{ID: "world_台北", Type: "world", Content: "台北是故事的舞台"},
	}
	idx := buildBM25Index(docs)

	// 查詢「林昊」，char_林昊 的分數應高於 world_台北
	terms := tokenize("林昊")
	score1 := idx.score(terms, "char_林昊")
	score2 := idx.score(terms, "world_台北")
	if score1 <= score2 {
		t.Errorf("expected char_林昊 (%.4f) > world_台北 (%.4f) for query 林昊", score1, score2)
	}
}

func TestBM25ScoreUnknownDoc(t *testing.T) {
	idx := buildBM25Index([]Document{{ID: "d1", Content: "test"}})
	if s := idx.score([]string{"test"}, "nonexistent"); s != 0 {
		t.Errorf("expected 0 for unknown docID, got %f", s)
	}
}

func TestBM25ScoreEmptyIndex(t *testing.T) {
	idx := buildBM25Index(nil)
	if s := idx.score([]string{"hello"}, "d1"); s != 0 {
		t.Errorf("expected 0 for empty index, got %f", s)
	}
}

func TestQueryHybridAlpha1PureVector(t *testing.T) {
	s := New("") // no file path needed, in-memory only
	docs := []Document{
		{ID: "char_林昊", Type: "character", Content: "林昊是主角", Embedding: []float64{1, 0}},
		{ID: "world_台北", Type: "world", Content: "台北是故事舞台", Embedding: []float64{0, 1}},
	}
	for _, d := range docs {
		s.Upsert(d)
	}

	// Query vector close to char_林昊
	results := s.QueryHybrid([]float64{1, 0}, "台北", 2, nil, 0, 1.0, 0)
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// alpha=1.0 → pure vector: char_林昊 should rank first (cosine=1.0 vs 0.0)
	if results[0].ID != "char_林昊" {
		t.Errorf("alpha=1.0: expected char_林昊 first, got %s", results[0].ID)
	}
}

func TestQueryHybridAlpha0PureBM25(t *testing.T) {
	s := New("")
	docs := []Document{
		// char_林昊 has keyword match for "台北" but vector points away
		{ID: "char_林昊", Type: "character", Content: "台北台北台北台北台北", Embedding: []float64{0, 1}},
		// world_台北 vector matches but no keyword match
		{ID: "world_台北", Type: "world", Content: "完全不同的內容", Embedding: []float64{1, 0}},
	}
	for _, d := range docs {
		s.Upsert(d)
	}

	// Query vector close to world_台北 but text matches char_林昊
	results := s.QueryHybrid([]float64{1, 0}, "台北", 2, nil, 0, 0.0, 0)
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// alpha=0.0 → pure BM25: char_林昊 should rank first (keyword hit)
	if results[0].ID != "char_林昊" {
		t.Errorf("alpha=0.0: expected char_林昊 first (BM25), got %s", results[0].ID)
	}
}

func TestQueryHybridTypeFilter(t *testing.T) {
	s := New("")
	s.Upsert(Document{ID: "char_a", Type: "character", Content: "hello", Embedding: []float64{1, 0}})
	s.Upsert(Document{ID: "world_b", Type: "world", Content: "hello", Embedding: []float64{1, 0}})

	results := s.QueryHybrid([]float64{1, 0}, "hello", 10, []string{"character"}, 0, 0.5, 0)
	if len(results) != 1 || results[0].ID != "char_a" {
		t.Errorf("type filter failed: got %v", results)
	}
}

func TestQueryHybridBeforeChapterFilter(t *testing.T) {
	s := New("")
	s.Upsert(Document{ID: "ch1", Type: "chapter", Content: "first chapter", Embedding: []float64{1, 0}, ChapterIndex: 1})
	s.Upsert(Document{ID: "ch5", Type: "chapter", Content: "fifth chapter", Embedding: []float64{1, 0}, ChapterIndex: 5})

	results := s.QueryHybrid([]float64{1, 0}, "chapter", 10, nil, 0, 0.5, 3)
	for _, r := range results {
		if r.ChapterIndex >= 3 {
			t.Errorf("beforeChapter=3: got doc with ChapterIndex=%d", r.ChapterIndex)
		}
	}
}

func TestQueryHybridUpsertUpdatesIndex(t *testing.T) {
	s := New("")
	s.Upsert(Document{ID: "d1", Type: "character", Content: "原始內容", Embedding: []float64{1, 0}})

	// Replace with new content — incremental update should remove old terms
	s.Upsert(Document{ID: "d1", Type: "character", Content: "林昊林昊林昊", Embedding: []float64{1, 0}})

	results := s.QueryHybrid([]float64{1, 0}, "林昊", 1, nil, 0, 0.0, 0)
	if len(results) == 0 || results[0].ID != "d1" {
		t.Error("BM25 index not updated after Upsert replace")
	}
	// Old terms must be removed from the index
	idx := s.bm25
	if idx.df["原"] != 0 || idx.df["始"] != 0 {
		t.Errorf("old terms still in df after replace: 原=%d 始=%d", idx.df["原"], idx.df["始"])
	}
	// n should still be 1
	if idx.n != 1 {
		t.Errorf("expected n=1 after replace, got %d", idx.n)
	}
}

func TestBM25IncrementalNEqualsDocCount(t *testing.T) {
	s := New("")
	for i := range 5 {
		s.Upsert(Document{
			ID:      fmt.Sprintf("d%d", i),
			Content: fmt.Sprintf("doc %d content", i),
		})
	}
	if s.bm25.n != 5 {
		t.Errorf("expected n=5, got %d", s.bm25.n)
	}
	// Overwrite one — n should stay 5
	s.Upsert(Document{ID: "d0", Content: "replaced"})
	if s.bm25.n != 5 {
		t.Errorf("after replace expected n=5, got %d", s.bm25.n)
	}
}
