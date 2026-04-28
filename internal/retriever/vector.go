package retriever

import (
	"context"
	"novel-assistant/internal/vectorstore"
)

type VectorStorer interface {
	Len() int
	QueryFilteredBeforeChapter(queryVec []float64, topK int, types []string, threshold float64, beforeChapter int) []vectorstore.ScoredDocument
}

type VectorRetriever struct {
	embedder Embedder
	store    VectorStorer
}

func NewVector(emb Embedder, store VectorStorer) *VectorRetriever {
	return &VectorRetriever{embedder: emb, store: store}
}

// Retrieve 行為等價於原 buildReferenceContext 中的 embed + QueryFilteredBeforeChapter。
// store 為空時直接回傳 nil, nil（與原邏輯一致）。
func (r *VectorRetriever) Retrieve(ctx context.Context, req Request) ([]Chunk, error) {
	if r.store.Len() == 0 {
		return nil, nil
	}
	vec, err := r.embedder.Embed(ctx, req.Query)
	if err != nil {
		return nil, err
	}
	docs := r.store.QueryFilteredBeforeChapter(vec, req.TopK, req.Types, req.Threshold, req.BeforeChapter)
	out := make([]Chunk, len(docs))
	for i, d := range docs {
		out[i] = Chunk{Document: d.Document, Score: d.Score}
	}
	return out, nil
}
