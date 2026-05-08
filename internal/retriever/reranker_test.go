package retriever_test

import (
	"context"
	"errors"
	"testing"

	"novel-assistant/internal/retriever"
	"novel-assistant/internal/vectorstore"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func makeChunk(id string, score float64) retriever.Chunk {
	return retriever.Chunk{
		Document: vectorstore.Document{ID: id, Content: "content of " + id},
		Score:    score,
	}
}

// fixedScorer maps chunk content → score for deterministic tests.
type fixedScorer struct {
	scores map[string]float64
	err    error
}

func (s *fixedScorer) ScoreBatch(_ context.Context, _ string, contents []string) ([]float64, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]float64, len(contents))
	for i, c := range contents {
		matched := false
		for id, sc := range s.scores {
			if c == "content of "+id {
				out[i] = sc
				matched = true
				break
			}
		}
		if !matched {
			out[i] = 0.5
		}
	}
	return out, nil
}

// wrongCountScorer returns a slice of the wrong length.
type wrongCountScorer struct{}

func (wrongCountScorer) ScoreBatch(_ context.Context, _ string, _ []string) ([]float64, error) {
	return []float64{0.9}, nil // always returns 1 score regardless of input count
}

type stubBaseRetriever struct {
	chunks []retriever.Chunk
}

func (r *stubBaseRetriever) Retrieve(_ context.Context, _ retriever.Request) ([]retriever.Chunk, error) {
	return r.chunks, nil
}

// ── PassthroughReranker ───────────────────────────────────────────────────────

func TestPassthroughRerankerReturnsSameChunks(t *testing.T) {
	chunks := []retriever.Chunk{makeChunk("a", 0.9), makeChunk("b", 0.5)}
	pr := retriever.PassthroughReranker{}
	got, err := pr.Rerank(context.Background(), "query", chunks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("expected unchanged chunks, got %v", got)
	}
}

func TestPassthroughRerankerEmptyInput(t *testing.T) {
	pr := retriever.PassthroughReranker{}
	got, err := pr.Rerank(context.Background(), "query", nil)
	if err != nil || len(got) != 0 {
		t.Errorf("expected nil slice and no error, got %v / %v", got, err)
	}
}

// ── LLMReranker ───────────────────────────────────────────────────────────────

func TestLLMRerankerSortsByScore(t *testing.T) {
	chunks := []retriever.Chunk{
		makeChunk("low", 0.9),
		makeChunk("high", 0.3),
		makeChunk("mid", 0.6),
	}
	scorer := &fixedScorer{scores: map[string]float64{
		"low":  0.1,
		"high": 0.95,
		"mid":  0.5,
	}}
	r := retriever.NewLLMReranker(scorer, 0)
	got, err := r.Rerank(context.Background(), "q", chunks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(got))
	}
	if got[0].ID != "high" || got[1].ID != "mid" || got[2].ID != "low" {
		t.Errorf("wrong order: %v", []string{got[0].ID, got[1].ID, got[2].ID})
	}
}

func TestLLMRerankerTrimsToTopN(t *testing.T) {
	chunks := []retriever.Chunk{makeChunk("a", 0.9), makeChunk("b", 0.8), makeChunk("c", 0.7)}
	scorer := &fixedScorer{scores: map[string]float64{"a": 0.9, "b": 0.8, "c": 0.7}}
	r := retriever.NewLLMReranker(scorer, 2)
	got, err := r.Rerank(context.Background(), "q", chunks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 chunks (topN=2), got %d", len(got))
	}
}

func TestLLMRerankerTopNLargerThanInput(t *testing.T) {
	chunks := []retriever.Chunk{makeChunk("a", 0.5)}
	scorer := &fixedScorer{scores: map[string]float64{"a": 0.5}}
	r := retriever.NewLLMReranker(scorer, 10)
	got, err := r.Rerank(context.Background(), "q", chunks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(got))
	}
}

// Scorer batch error → return chunks in original order, no error propagated.
func TestLLMRerankerBatchErrorFallsBackToOriginalOrder(t *testing.T) {
	chunks := []retriever.Chunk{makeChunk("a", 0.9), makeChunk("b", 0.1)}
	scorer := &fixedScorer{err: errors.New("scorer unavailable")}
	r := retriever.NewLLMReranker(scorer, 0)
	got, err := r.Rerank(context.Background(), "q", chunks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("expected original order preserved, got %v", got)
	}
}

// Scorer returns wrong number of scores → fall back to original order.
func TestLLMRerankerWrongScoreCountFallsBackToOriginalOrder(t *testing.T) {
	chunks := []retriever.Chunk{makeChunk("a", 0.9), makeChunk("b", 0.5), makeChunk("c", 0.1)}
	r := retriever.NewLLMReranker(wrongCountScorer{}, 0)
	got, err := r.Rerank(context.Background(), "q", chunks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 || got[0].ID != "a" {
		t.Errorf("expected original order, got %v", got)
	}
}

func TestLLMRerankerUpdatesScoreField(t *testing.T) {
	chunks := []retriever.Chunk{makeChunk("a", 0.3)}
	scorer := &fixedScorer{scores: map[string]float64{"a": 0.99}}
	r := retriever.NewLLMReranker(scorer, 0)
	got, _ := r.Rerank(context.Background(), "q", chunks)
	if got[0].Score != 0.99 {
		t.Errorf("expected Score updated to 0.99, got %f", got[0].Score)
	}
}

// ── parseScores (exported for testing via ParseScoresForTest) ─────────────────
// We test parseScores indirectly through a mock scorer that calls it.

func TestParseScores(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantN   int
		wantErr bool
	}{
		{"bare JSON array", "[0.9, 0.3, 0.75]", 3, false},
		{"JSON array with surrounding text", "Sure, here are the scores: [0.9, 0.3, 0.75]\nDone.", 3, false},
		{"one per line", "0.9\n0.3\n0.75", 3, false},
		{"JSON array with trailing period", "[0.82.]", 0, true},
		{"empty string", "", 0, true},
		{"non-numeric", "high medium low", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scores, err := retriever.ParseScoresForTest(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got scores %v", scores)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(scores) != tc.wantN {
				t.Errorf("expected %d scores, got %d: %v", tc.wantN, len(scores), scores)
			}
		})
	}
}

// ── RerankingRetriever ────────────────────────────────────────────────────────

func TestRerankingRetrieverAppliesReranker(t *testing.T) {
	base := &stubBaseRetriever{chunks: []retriever.Chunk{
		makeChunk("low", 0.9),
		makeChunk("high", 0.1),
	}}
	scorer := &fixedScorer{scores: map[string]float64{"low": 0.1, "high": 0.9}}
	rr := retriever.NewRerankingRetriever(base, retriever.NewLLMReranker(scorer, 0))

	got, err := rr.Retrieve(context.Background(), retriever.Request{Query: "test", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "high" {
		t.Errorf("expected high-scored chunk first, got %v", got)
	}
}

func TestRerankingRetrieverPassthroughNilReranker(t *testing.T) {
	chunks := []retriever.Chunk{makeChunk("a", 0.5), makeChunk("b", 0.3)}
	base := &stubBaseRetriever{chunks: chunks}
	rr := retriever.NewRerankingRetriever(base, nil) // nil → PassthroughReranker

	got, err := rr.Retrieve(context.Background(), retriever.Request{Query: "q", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "a" {
		t.Errorf("expected passthrough order preserved, got %v", got)
	}
}

func TestRerankingRetrieverEmptyBaseResult(t *testing.T) {
	base := &stubBaseRetriever{chunks: nil}
	scorer := &fixedScorer{}
	rr := retriever.NewRerankingRetriever(base, retriever.NewLLMReranker(scorer, 0))

	got, err := rr.Retrieve(context.Background(), retriever.Request{Query: "q", TopK: 5})
	if err != nil || len(got) != 0 {
		t.Errorf("expected empty result, got %v / %v", got, err)
	}
}

// compile-time interface checks
var _ retriever.Reranker = retriever.PassthroughReranker{}
var _ retriever.Reranker = (*retriever.LLMReranker)(nil)
var _ retriever.Retriever = (*retriever.RerankingRetriever)(nil)
var _ retriever.Scorer = (*retriever.OllamaScorer)(nil)
