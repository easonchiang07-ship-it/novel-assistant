package retriever_test

import (
	"context"
	"testing"

	"novel-assistant/internal/retriever"
	"novel-assistant/internal/vectorstore"
)

type stubHybridStore struct {
	docs      []vectorstore.ScoredDocument
	gotAlpha  float64
	gotBefore int
}

func (s *stubHybridStore) Len() int { return len(s.docs) }
func (s *stubHybridStore) QueryHybrid(_ []float64, _ string, topK int, _ []string, _ float64, alpha float64, beforeChapter int) []vectorstore.ScoredDocument {
	s.gotAlpha = alpha
	s.gotBefore = beforeChapter
	if topK < len(s.docs) {
		return s.docs[:topK]
	}
	return s.docs
}

func TestHybridRetrieverEmptyStore(t *testing.T) {
	r := retriever.NewHybrid(&stubEmbedder{vec: []float64{1}}, &stubHybridStore{}, 0.5)
	chunks, err := r.Retrieve(context.Background(), retriever.Request{Query: "test", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty store, got %d", len(chunks))
	}
}

func TestHybridRetrieverReturnsChunks(t *testing.T) {
	doc := vectorstore.ScoredDocument{
		Document: vectorstore.Document{ID: "char_林昊", Type: "character", Content: "主角"},
		Score:    0.85,
	}
	store := &stubHybridStore{docs: []vectorstore.ScoredDocument{doc}}
	r := retriever.NewHybrid(&stubEmbedder{vec: []float64{1}}, store, 0.5)

	chunks, err := r.Retrieve(context.Background(), retriever.Request{Query: "林昊", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].ID != "char_林昊" {
		t.Errorf("expected ID=char_林昊, got %s", chunks[0].ID)
	}
	if chunks[0].Score != 0.85 {
		t.Errorf("expected Score=0.85, got %f", chunks[0].Score)
	}
}

func TestHybridRetrieverForwardsAlpha(t *testing.T) {
	store := &stubHybridStore{
		docs: []vectorstore.ScoredDocument{
			{Document: vectorstore.Document{ID: "d1"}, Score: 0.5},
		},
	}
	r := retriever.NewHybrid(&stubEmbedder{vec: []float64{1}}, store, 0.3)
	_, err := r.Retrieve(context.Background(), retriever.Request{Query: "q", TopK: 1})
	if err != nil {
		t.Fatal(err)
	}
	if store.gotAlpha != 0.3 {
		t.Errorf("alpha not forwarded: got %f, want 0.3", store.gotAlpha)
	}
}

func TestHybridRetrieverForwardsBeforeChapter(t *testing.T) {
	store := &stubHybridStore{
		docs: []vectorstore.ScoredDocument{
			{Document: vectorstore.Document{ID: "d1"}, Score: 0.5},
		},
	}
	r := retriever.NewHybrid(&stubEmbedder{vec: []float64{1}}, store, 0.5)
	_, err := r.Retrieve(context.Background(), retriever.Request{Query: "q", TopK: 1, BeforeChapter: 7})
	if err != nil {
		t.Fatal(err)
	}
	if store.gotBefore != 7 {
		t.Errorf("beforeChapter not forwarded: got %d, want 7", store.gotBefore)
	}
}

func TestHybridRetrieverSatisfiesRetrieverInterface(t *testing.T) {
	var _ retriever.Retriever = retriever.NewHybrid(nil, &stubHybridStore{}, 0.5)
}
