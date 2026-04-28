package retriever

import (
	"context"
	"novel-assistant/internal/vectorstore"
)

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

type Request struct {
	Query         string
	Types         []string
	TopK          int
	Threshold     float64
	BeforeChapter int
}

type Chunk struct {
	vectorstore.Document
	Score float64
}

type Retriever interface {
	Retrieve(ctx context.Context, req Request) ([]Chunk, error)
}
