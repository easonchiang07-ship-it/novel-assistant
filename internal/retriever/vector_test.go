package retriever_test

import (
	"context"
	"testing"

	"novel-assistant/internal/retriever"
	"novel-assistant/internal/vectorstore"
)

type stubEmbedder struct{ vec []float64 }

func (s *stubEmbedder) Embed(_ context.Context, _ string) ([]float64, error) {
	return s.vec, nil
}

type stubStore struct {
	docs []vectorstore.ScoredDocument
}

func (s *stubStore) Len() int { return len(s.docs) }
func (s *stubStore) QueryFilteredBeforeChapter(_ []float64, topK int, _ []string, _ float64, _ int) []vectorstore.ScoredDocument {
	if topK < len(s.docs) {
		return s.docs[:topK]
	}
	return s.docs
}

type recordingStore struct {
	docs         []vectorstore.ScoredDocument
	gotTopK      int
	gotTypes     []string
	gotThreshold float64
	gotBefore    int
}

func (s *recordingStore) Len() int { return len(s.docs) }
func (s *recordingStore) QueryFilteredBeforeChapter(_ []float64, topK int, types []string, threshold float64, beforeChapter int) []vectorstore.ScoredDocument {
	s.gotTopK = topK
	s.gotTypes = types
	s.gotThreshold = threshold
	s.gotBefore = beforeChapter
	if topK < len(s.docs) {
		return s.docs[:topK]
	}
	return s.docs
}

func TestVectorRetrieverEmptyStore(t *testing.T) {
	r := retriever.NewVector(&stubEmbedder{vec: []float64{1}}, &stubStore{})
	chunks, err := r.Retrieve(context.Background(), retriever.Request{Query: "test", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty store, got %d", len(chunks))
	}
}

func TestVectorRetrieverReturnsChunks(t *testing.T) {
	doc := vectorstore.ScoredDocument{
		Document: vectorstore.Document{ID: "char_林昊", Type: "character", Content: "主角"},
		Score:    0.9,
	}
	store := &stubStore{docs: []vectorstore.ScoredDocument{doc}}
	r := retriever.NewVector(&stubEmbedder{vec: []float64{1}}, store)

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
	if chunks[0].Score != 0.9 {
		t.Errorf("expected Score=0.9, got %f", chunks[0].Score)
	}
}

func TestVectorRetrieverPassesRequestParams(t *testing.T) {
	doc := vectorstore.ScoredDocument{
		Document: vectorstore.Document{ID: "char_X", Type: "character"},
		Score:    0.5,
	}
	store := &recordingStore{docs: []vectorstore.ScoredDocument{doc, doc, doc, doc, doc}}
	r := retriever.NewVector(&stubEmbedder{vec: []float64{1}}, store)

	req := retriever.Request{
		Query:         "test",
		Types:         []string{"character", "world"},
		TopK:          2,
		Threshold:     0.3,
		BeforeChapter: 4,
	}
	chunks, err := r.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks (topK=2), got %d", len(chunks))
	}
	if store.gotTopK != 2 {
		t.Errorf("topK not forwarded: got %d, want 2", store.gotTopK)
	}
	if len(store.gotTypes) != 2 || store.gotTypes[0] != "character" || store.gotTypes[1] != "world" {
		t.Errorf("types not forwarded: got %v", store.gotTypes)
	}
	if store.gotThreshold != 0.3 {
		t.Errorf("threshold not forwarded: got %f, want 0.3", store.gotThreshold)
	}
	if store.gotBefore != 4 {
		t.Errorf("beforeChapter not forwarded: got %d, want 4", store.gotBefore)
	}
}
