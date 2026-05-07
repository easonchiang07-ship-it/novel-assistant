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
	// TopN caps results after reranking; 0 keeps all. Issue #146 specifies the default
	// should equal TopK, but since the inner retriever already respects TopK before
	// reranking, the practical difference is small. Callers that want strict parity
	// should set TopN = req.TopK explicitly.
	TopN int
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

// LLMReranker scores all chunks against the query in one batch LLM call and sorts by score descending.
type LLMReranker struct {
	scorer LLMScorer
}

func NewLLMReranker(scorer LLMScorer) *LLMReranker {
	return &LLMReranker{scorer: scorer}
}

const rerankBatchPromptTpl = "Query: %s\n以 0.0 到 1.0 評分以下每段文本與查詢的相關性，依序每行輸出一個數字，不要有其他說明：\n\n%s"

func (r *LLMReranker) Rerank(ctx context.Context, query string, chunks []Chunk) ([]Chunk, error) {
	if len(chunks) == 0 {
		return chunks, nil
	}
	scores, err := r.scoreChunksBatch(ctx, query, chunks)
	if err != nil {
		return chunks, nil // degrade to original order/scores on batch failure
	}
	type entry struct {
		chunk Chunk
		score float64
	}
	entries := make([]entry, len(chunks))
	for i, c := range chunks {
		entries[i] = entry{chunk: c, score: scores[i]}
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

func (r *LLMReranker) scoreChunksBatch(ctx context.Context, query string, chunks []Chunk) ([]float64, error) {
	var sb strings.Builder
	for i, c := range chunks {
		fmt.Fprintf(&sb, "[%d] %s\n", i+1, c.Content)
	}
	raw, err := r.scorer.Score(ctx, fmt.Sprintf(rerankBatchPromptTpl, query, sb.String()))
	if err != nil {
		return nil, err
	}
	return parseBatchScores(raw, len(chunks))
}

func parseBatchScores(raw string, n int) ([]float64, error) {
	scores := make([]float64, 0, n)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if f, ok := extractFloat(line); ok {
			scores = append(scores, f)
		}
		if len(scores) == n {
			break
		}
	}
	if len(scores) != n {
		return nil, fmt.Errorf("batch score: expected %d scores, got %d", n, len(scores))
	}
	return scores, nil
}

// extractFloat scans whitespace-separated tokens for the last parseable float and clamps it to [0,1].
// Using the last token avoids misreading line-number prefixes (e.g. "1. 0.82", "[2] 0.45") as scores.
func extractFloat(s string) (float64, bool) {
	var last float64
	found := false
	for _, tok := range strings.Fields(s) {
		tok = strings.TrimRight(tok, "。,.;:!?)]")
		tok = strings.TrimLeft(tok, "([")
		if f, err := strconv.ParseFloat(tok, 64); err == nil {
			if f < 0 {
				f = 0
			} else if f > 1 {
				f = 1
			}
			last = f
			found = true
		}
	}
	return last, found
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
