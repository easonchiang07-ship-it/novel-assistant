package retriever

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Reranker re-scores retrieval candidates and returns a re-ranked slice.
type Reranker interface {
	Rerank(ctx context.Context, query string, chunks []Chunk) ([]Chunk, error)
}

// RerankConfig controls optional reranking after initial retrieval.
type RerankConfig struct {
	Enabled bool
	TopN    int // 0 means keep all reranked results
}

// PassthroughReranker is the no-op default — preserves input order with zero LLM cost.
type PassthroughReranker struct{}

func (PassthroughReranker) Rerank(_ context.Context, _ string, chunks []Chunk) ([]Chunk, error) {
	return chunks, nil
}

// LLMScorer issues a single LLM call and returns the full text response.
type LLMScorer interface {
	Score(ctx context.Context, prompt string) (string, error)
}

// OllamaScorer implements LLMScorer via Ollama /api/generate.
type OllamaScorer struct {
	BaseURL string
	Model   string
}

type ollamaScoreReq struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaScoreChunk struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (o *OllamaScorer) Score(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(ollamaScoreReq{Model: o.Model, Prompt: prompt, Stream: true})
	req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("ollama score failed: %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	var sb strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var chunk ollamaScoreChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		sb.WriteString(chunk.Response)
		if chunk.Done {
			break
		}
	}
	return sb.String(), scanner.Err()
}

// LLMReranker scores each chunk against the query using an LLM and sorts by score descending.
type LLMReranker struct {
	scorer LLMScorer
}

func NewLLMReranker(scorer LLMScorer) *LLMReranker {
	return &LLMReranker{scorer: scorer}
}

const rerankPromptTpl = "Query: %s\nText: %s\n以 0.0 到 1.0 評分此文本與查詢的相關性，僅輸出數字。"

func (r *LLMReranker) Rerank(ctx context.Context, query string, chunks []Chunk) ([]Chunk, error) {
	type entry struct {
		chunk Chunk
		score float64
	}
	entries := make([]entry, len(chunks))
	for i, c := range chunks {
		s, err := r.scoreChunk(ctx, query, c.Content)
		if err != nil {
			s = c.Score // degrade to original score on per-chunk error
		}
		entries[i] = entry{chunk: c, score: s}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})
	out := make([]Chunk, len(entries))
	for i, e := range entries {
		out[i] = e.chunk
		out[i].Score = e.score
	}
	return out, nil
}

func (r *LLMReranker) scoreChunk(ctx context.Context, query, content string) (float64, error) {
	raw, err := r.scorer.Score(ctx, fmt.Sprintf(rerankPromptTpl, query, content))
	if err != nil {
		return 0, err
	}
	raw = strings.TrimSpace(raw)
	score, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("parse score %q: %w", raw, err)
	}
	if score < 0 {
		score = 0
	} else if score > 1 {
		score = 1
	}
	return score, nil
}

// WithReranking wraps inner with reranking. When cfg.Enabled is false, inner is returned as-is
// (true zero overhead — no extra interface dispatch on the retrieval hot path).
func WithReranking(inner Retriever, reranker Reranker, cfg RerankConfig) Retriever {
	if !cfg.Enabled {
		return inner
	}
	return &rerankingRetriever{inner: inner, reranker: reranker, topN: cfg.TopN}
}

type rerankingRetriever struct {
	inner    Retriever
	reranker Reranker
	topN     int
}

func (r *rerankingRetriever) Retrieve(ctx context.Context, req Request) ([]Chunk, error) {
	chunks, err := r.inner.Retrieve(ctx, req)
	if err != nil || len(chunks) == 0 {
		return chunks, err
	}
	ranked, err := r.reranker.Rerank(ctx, req.Query, chunks)
	if err != nil {
		return chunks, nil // degrade gracefully on rerank error
	}
	if r.topN > 0 && len(ranked) > r.topN {
		ranked = ranked[:r.topN]
	}
	return ranked, nil
}
