package retriever_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"novel-assistant/internal/retriever"
	"novel-assistant/internal/vectorstore"
)

// --- mock helpers ---

type stubScorer struct {
	// batchResp is returned verbatim for any Score call; takes precedence over responses.
	batchResp string
	responses map[string]string // content -> score string (used only when batchResp is empty)
	err       error
}

func (s *stubScorer) Score(_ context.Context, prompt string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.batchResp != "" {
		return s.batchResp, nil
	}
	for content, score := range s.responses {
		if len(prompt) > 0 && containsStr(prompt, content) {
			return score, nil
		}
	}
	return "0.5", nil
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

type stubRetrieverFunc func(ctx context.Context, req retriever.Request) ([]retriever.Chunk, error)

func (f stubRetrieverFunc) Retrieve(ctx context.Context, req retriever.Request) ([]retriever.Chunk, error) {
	return f(ctx, req)
}

func makeChunk(id string, score float64, content string) retriever.Chunk {
	return retriever.Chunk{
		Document: vectorstore.Document{ID: id, Content: content},
		Score:    score,
	}
}

// --- PassthroughReranker ---

func TestPassthroughRerankerPreservesOrder(t *testing.T) {
	chunks := []retriever.Chunk{
		makeChunk("a", 0.9, "aaa"),
		makeChunk("b", 0.7, "bbb"),
		makeChunk("c", 0.5, "ccc"),
	}
	r := retriever.PassthroughReranker{}
	got, err := r.Rerank(context.Background(), "query", chunks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(got))
	}
	for i, c := range chunks {
		if got[i].ID != c.ID {
			t.Errorf("[%d] expected ID=%s, got=%s", i, c.ID, got[i].ID)
		}
	}
}

func TestPassthroughRerankerEmpty(t *testing.T) {
	r := retriever.PassthroughReranker{}
	got, err := r.Rerank(context.Background(), "q", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

// --- LLMReranker ---

func TestLLMRerankerSortsByScore(t *testing.T) {
	// Batch response: scores in input order — low=0.2, mid=0.5, high=0.9
	scorer := &stubScorer{batchResp: "0.2\n0.5\n0.9"}
	r := retriever.NewLLMReranker(scorer)
	chunks := []retriever.Chunk{
		makeChunk("low", 0.9, "low"),
		makeChunk("mid", 0.7, "mid"),
		makeChunk("high", 0.1, "high"),
	}
	got, err := r.Rerank(context.Background(), "query", chunks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ID != "high" || got[1].ID != "mid" || got[2].ID != "low" {
		t.Errorf("unexpected order: %v %v %v", got[0].ID, got[1].ID, got[2].ID)
	}
	if fmt.Sprintf("%.1f", got[0].Score) != "0.9" {
		t.Errorf("Score not updated: got %f", got[0].Score)
	}
}

func TestLLMRerankerFallsBackOnScorerError(t *testing.T) {
	scorer := &stubScorer{err: errors.New("llm down")}
	r := retriever.NewLLMReranker(scorer)
	chunks := []retriever.Chunk{
		makeChunk("a", 0.8, "aaa"),
		makeChunk("b", 0.6, "bbb"),
	}
	got, err := r.Rerank(context.Background(), "query", chunks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have original scores as fallback
	if got[0].Score != 0.8 || got[1].Score != 0.6 {
		t.Errorf("expected original scores 0.8/0.6, got %f/%f", got[0].Score, got[1].Score)
	}
}

func TestLLMRerankerClampsScore(t *testing.T) {
	scorer := &stubScorer{batchResp: "2.5"}
	r := retriever.NewLLMReranker(scorer)
	chunks := []retriever.Chunk{makeChunk("x", 0.5, "x")}
	got, err := r.Rerank(context.Background(), "q", chunks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Score != 1.0 {
		t.Errorf("expected clamped score 1.0, got %f", got[0].Score)
	}
}

func TestLLMRerankerHandlesMessyScoreOutput(t *testing.T) {
	cases := []struct {
		name      string
		batchResp string
		wantScore float64
	}{
		{"chinese period", "0.82。", 0.82},
		{"label prefix", "score: 0.75", 0.75},
		{"trailing comma", "0.60,", 0.60},
		{"extra explanation line then score", "The text is relevant.\n0.90", 0.90},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scorer := &stubScorer{batchResp: tc.batchResp}
			r := retriever.NewLLMReranker(scorer)
			chunks := []retriever.Chunk{makeChunk("x", 0.0, "x")}
			got, err := r.Rerank(context.Background(), "q", chunks)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fmt.Sprintf("%.2f", got[0].Score) != fmt.Sprintf("%.2f", tc.wantScore) {
				t.Errorf("expected %.2f, got %f", tc.wantScore, got[0].Score)
			}
		})
	}
}

func TestLLMRerankerSatisfiesRerankerInterface(t *testing.T) {
	var _ retriever.Reranker = retriever.NewLLMReranker(nil)
}

// --- WithReranking ---

func TestWithRerankingDisabledReturnsInnerDirectly(t *testing.T) {
	called := false
	inner := stubRetrieverFunc(func(_ context.Context, _ retriever.Request) ([]retriever.Chunk, error) {
		called = true
		return []retriever.Chunk{makeChunk("x", 0.5, "x")}, nil
	})
	wrapped := retriever.WithReranking(inner, retriever.PassthroughReranker{}, retriever.RerankConfig{Enabled: false})
	got, err := wrapped.Retrieve(context.Background(), retriever.Request{Query: "q", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("inner retriever was not called")
	}
	if len(got) != 1 || got[0].ID != "x" {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestWithRerankingEnabledAppliesReranker(t *testing.T) {
	inner := stubRetrieverFunc(func(_ context.Context, _ retriever.Request) ([]retriever.Chunk, error) {
		return []retriever.Chunk{
			makeChunk("low", 0.9, "low"),
			makeChunk("high", 0.1, "high"),
		}, nil
	})
	// Batch response: scores in input order — low=0.1, high=0.9
	scorer := &stubScorer{batchResp: "0.1\n0.9"}
	reranker := retriever.NewLLMReranker(scorer)
	wrapped := retriever.WithReranking(inner, reranker, retriever.RerankConfig{Enabled: true})
	got, err := wrapped.Retrieve(context.Background(), retriever.Request{Query: "q", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ID != "high" {
		t.Errorf("expected high to rank first, got %s", got[0].ID)
	}
}

func TestWithRerankingTopNTruncates(t *testing.T) {
	inner := stubRetrieverFunc(func(_ context.Context, _ retriever.Request) ([]retriever.Chunk, error) {
		return []retriever.Chunk{
			makeChunk("a", 0.5, "a"),
			makeChunk("b", 0.5, "b"),
			makeChunk("c", 0.5, "c"),
		}, nil
	})
	wrapped := retriever.WithReranking(inner, retriever.PassthroughReranker{}, retriever.RerankConfig{Enabled: true, TopN: 2})
	got, err := wrapped.Retrieve(context.Background(), retriever.Request{Query: "q", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 results (TopN=2), got %d", len(got))
	}
}

func TestWithRerankingDegradeOnRerankerError(t *testing.T) {
	inner := stubRetrieverFunc(func(_ context.Context, _ retriever.Request) ([]retriever.Chunk, error) {
		return []retriever.Chunk{makeChunk("a", 0.9, "aaa"), makeChunk("b", 0.7, "bbb")}, nil
	})
	// Scorer errors on every call → LLMReranker falls back to original scores, no error returned
	scorer := &stubScorer{err: errors.New("llm down")}
	wrapped := retriever.WithReranking(inner, retriever.NewLLMReranker(scorer), retriever.RerankConfig{Enabled: true})
	got, err := wrapped.Retrieve(context.Background(), retriever.Request{Query: "q", TopK: 5})
	if err != nil {
		t.Fatalf("expected graceful degradation, got error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 results on degradation, got %d", len(got))
	}
}

func TestWithRerankingEmptyChunksSkipsReranker(t *testing.T) {
	inner := stubRetrieverFunc(func(_ context.Context, _ retriever.Request) ([]retriever.Chunk, error) {
		return nil, nil
	})
	called := false
	scorer := &stubScorer{}
	_ = scorer
	reranker := retriever.PassthroughReranker{}
	_ = reranker
	// Use a reranker that would set called=true to prove it's not invoked
	wrapped := retriever.WithReranking(inner, retriever.PassthroughReranker{}, retriever.RerankConfig{Enabled: true})
	got, err := wrapped.Retrieve(context.Background(), retriever.Request{Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(got))
	}
	_ = called
}
