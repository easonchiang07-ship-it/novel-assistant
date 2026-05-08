package retriever

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Scorer scores the relevance of a chunk to a query, returning a value in [0, 1].
type Scorer interface {
	Score(ctx context.Context, query, chunk string) (float64, error)
}

// Reranker reorders (and optionally trims) chunks after initial retrieval.
type Reranker interface {
	Rerank(ctx context.Context, query string, chunks []Chunk) ([]Chunk, error)
}

// RerankConfig controls whether reranking is applied and how many results are kept.
type RerankConfig struct {
	Enabled bool
	TopN    int // 0 means keep all
}

// PassthroughReranker is the default no-op; it returns chunks unchanged.
type PassthroughReranker struct{}

func (PassthroughReranker) Rerank(_ context.Context, _ string, chunks []Chunk) ([]Chunk, error) {
	return chunks, nil
}

// LLMReranker scores each chunk with a Scorer and returns the top-N results sorted by score.
// If a chunk fails to score, its original retrieval score is used as fallback.
type LLMReranker struct {
	scorer Scorer
	topN   int
}

func NewLLMReranker(scorer Scorer, topN int) *LLMReranker {
	return &LLMReranker{scorer: scorer, topN: topN}
}

func (r *LLMReranker) Rerank(ctx context.Context, query string, chunks []Chunk) ([]Chunk, error) {
	type scored struct {
		chunk Chunk
		score float64
	}
	results := make([]scored, 0, len(chunks))
	for _, c := range chunks {
		s, err := r.scorer.Score(ctx, query, c.Content)
		if err != nil {
			log.Printf("reranker scorer error (chunk %s): %v — using original score", c.ID, err)
			s = c.Score
		}
		results = append(results, scored{chunk: c, score: s})
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	n := r.topN
	if n <= 0 || n > len(results) {
		n = len(results)
	}
	out := make([]Chunk, n)
	for i := 0; i < n; i++ {
		out[i] = results[i].chunk
		out[i].Score = results[i].score
	}
	return out, nil
}

// OllamaScorer scores a query-chunk pair using Ollama's non-streaming generate API.
type OllamaScorer struct {
	baseURL string
	model   string
}

func NewOllamaScorer(baseURL, model string) *OllamaScorer {
	return &OllamaScorer{baseURL: baseURL, model: model}
}

func (o *OllamaScorer) Score(ctx context.Context, query, chunk string) (float64, error) {
	prompt := "Rate the relevance of the following passage to the query on a scale from 0.0 to 1.0.\n" +
		"Respond with only a single decimal number between 0.0 and 1.0, nothing else.\n\n" +
		"Query: " + query + "\n\nPassage: " + chunk

	type scoreReq struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Stream bool   `json:"stream"`
	}
	type scoreResp struct {
		Response string `json:"response"`
	}

	body, err := json.Marshal(scoreReq{Model: o.model, Prompt: prompt, Stream: false})
	if err != nil {
		return 0, fmt.Errorf("marshal score request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("ollama scorer unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return 0, fmt.Errorf("ollama scorer failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var result scoreResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode score response: %w", err)
	}

	score, err := strconv.ParseFloat(strings.TrimSpace(result.Response), 64)
	if err != nil {
		return 0, fmt.Errorf("scorer returned non-numeric response: %q", result.Response)
	}
	if score < 0 {
		score = 0
	} else if score > 1 {
		score = 1
	}
	return score, nil
}
