package retriever

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Scorer scores multiple chunks against a query in a single call,
// returning one score in [0, 1] per chunk in the same order.
type Scorer interface {
	ScoreBatch(ctx context.Context, query string, contents []string) ([]float64, error)
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

// LLMReranker scores all chunks in a single ScoreBatch call and returns the top-N results.
// If scoring fails entirely, chunks are returned in their original order.
// If the number of returned scores doesn't match, original scores are used as fallback.
type LLMReranker struct {
	scorer Scorer
	topN   int
}

func NewLLMReranker(scorer Scorer, topN int) *LLMReranker {
	return &LLMReranker{scorer: scorer, topN: topN}
}

func (r *LLMReranker) Rerank(ctx context.Context, query string, chunks []Chunk) ([]Chunk, error) {
	contents := make([]string, len(chunks))
	for i, c := range chunks {
		contents[i] = c.Content
	}

	scores, err := r.scorer.ScoreBatch(ctx, query, contents)
	if err != nil {
		log.Printf("reranker scorer batch error: %v — using original order", err)
		return chunks, nil
	}
	if len(scores) != len(chunks) {
		log.Printf("reranker scorer returned %d scores for %d chunks — using original order", len(scores), len(chunks))
		return chunks, nil
	}

	type scored struct {
		chunk Chunk
		score float64
	}
	results := make([]scored, len(chunks))
	for i, c := range chunks {
		results[i] = scored{chunk: c, score: scores[i]}
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

// jsonArrayRe matches a JSON number array anywhere in a string, e.g. [0.9, 0.3, 1.0]
var jsonArrayRe = regexp.MustCompile(`\[[\d.,\s]+\]`)

// parseScores extracts a []float64 from an LLM response that should contain a JSON array.
// It tries strict JSON parsing first, then regex extraction, then line-by-line fallback.
// Each score is clamped to [0, 1].
func parseScores(raw string) ([]float64, error) {
	raw = strings.TrimSpace(raw)

	// attempt 1: the whole response is a JSON array
	var scores []float64
	if err := json.Unmarshal([]byte(raw), &scores); err == nil {
		return clamp(scores), nil
	}

	// attempt 2: find the first JSON array anywhere in the response
	if m := jsonArrayRe.FindString(raw); m != "" {
		if err := json.Unmarshal([]byte(m), &scores); err == nil {
			return clamp(scores), nil
		}
	}

	// attempt 3: one float per non-empty line
	var fallback []float64
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f, err := strconv.ParseFloat(line, 64)
		if err != nil {
			return nil, fmt.Errorf("cannot parse scores from response: %q", raw)
		}
		fallback = append(fallback, f)
	}
	if len(fallback) > 0 {
		return clamp(fallback), nil
	}
	return nil, fmt.Errorf("no scores found in response: %q", raw)
}

func clamp(scores []float64) []float64 {
	for i, s := range scores {
		if s < 0 {
			scores[i] = 0
		} else if s > 1 {
			scores[i] = 1
		}
	}
	return scores
}

// OllamaScorer scores all chunks in a single Ollama /api/generate call (non-streaming).
type OllamaScorer struct {
	baseURL string
	model   string
}

func NewOllamaScorer(baseURL, model string) *OllamaScorer {
	return &OllamaScorer{baseURL: baseURL, model: model}
}

func (o *OllamaScorer) ScoreBatch(ctx context.Context, query string, contents []string) ([]float64, error) {
	var sb strings.Builder
	sb.WriteString("Rate the relevance of each passage below to the query on a scale from 0.0 to 1.0.\n")
	sb.WriteString("Respond with a JSON array of numbers only, one score per passage in the same order.\n")
	sb.WriteString("Example for 3 passages: [0.9, 0.3, 0.75]\n\n")
	sb.WriteString("Query: ")
	sb.WriteString(query)
	sb.WriteString("\n\nPassages:\n")
	for i, c := range contents {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, c)
	}

	type scoreReq struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Stream bool   `json:"stream"`
	}
	type scoreResp struct {
		Response string `json:"response"`
	}

	body, err := json.Marshal(scoreReq{Model: o.model, Prompt: sb.String(), Stream: false})
	if err != nil {
		return nil, fmt.Errorf("marshal score request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama scorer unavailable: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("ollama scorer failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var result scoreResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode score response: %w", err)
	}

	scores, err := parseScores(result.Response)
	if err != nil {
		return nil, err
	}
	return scores, nil
}
