package retriever

import "context"

// RerankingRetriever wraps a base Retriever and applies a Reranker to its results.
// Use PassthroughReranker (the default) for zero-overhead passthrough.
type RerankingRetriever struct {
	base    Retriever
	reranker Reranker
}

// NewRerankingRetriever creates a RerankingRetriever. If reranker is nil, PassthroughReranker is used.
func NewRerankingRetriever(base Retriever, reranker Reranker) *RerankingRetriever {
	if reranker == nil {
		reranker = PassthroughReranker{}
	}
	return &RerankingRetriever{base: base, reranker: reranker}
}

func (r *RerankingRetriever) Retrieve(ctx context.Context, req Request) ([]Chunk, error) {
	chunks, err := r.base.Retrieve(ctx, req)
	if err != nil || len(chunks) == 0 {
		return chunks, err
	}
	return r.reranker.Rerank(ctx, req.Query, chunks)
}
