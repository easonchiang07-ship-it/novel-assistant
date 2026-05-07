package retriever

import (
	"context"
	"novel-assistant/internal/vectorstore"
)

// HybridStorer is the store interface required by HybridRetriever.
type HybridStorer interface {
	Len() int
	QueryHybrid(queryVec []float64, queryText string, topK int, types []string, threshold float64, alpha float64, beforeChapter int) []vectorstore.ScoredDocument
}

// HybridRetriever combines BM25 keyword scoring with vector similarity.
// alpha=1.0 is pure vector; alpha=0.0 is pure BM25; default 0.5.
type HybridRetriever struct {
	embedder Embedder
	store    HybridStorer
	alpha    float64
}

// NewHybrid creates a HybridRetriever. alpha is clamped to [0.0, 1.0].
func NewHybrid(emb Embedder, store HybridStorer, alpha float64) *HybridRetriever {
	if alpha < 0 {
		alpha = 0
	} else if alpha > 1 {
		alpha = 1
	}
	return &HybridRetriever{embedder: emb, store: store, alpha: alpha}
}

func (r *HybridRetriever) Retrieve(ctx context.Context, req Request) ([]Chunk, error) {
	if r.store.Len() == 0 {
		return nil, nil
	}
	vec, err := r.embedder.Embed(ctx, req.Query)
	if err != nil {
		return nil, err
	}
	docs := r.store.QueryHybrid(vec, req.Query, req.TopK, req.Types, req.Threshold, r.alpha, req.BeforeChapter)
	out := make([]Chunk, len(docs))
	for i, d := range docs {
		out[i] = Chunk{Document: d.Document, Score: d.Score}
	}
	return out, nil
}
