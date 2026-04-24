package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// EvaluationScores holds raw 1–5 scores per dimension.
type EvaluationScores struct {
	Plot      int `json:"plot"`
	Character int `json:"character"`
	Style     int `json:"style"`
	Pacing    int `json:"pacing"`
	Hook      int `json:"hook"`
}

// EvaluationReasons maps each dimension to a one-sentence Chinese explanation.
type EvaluationReasons map[string]string

// EvaluationResponse is one LLM run's parsed output.
type EvaluationResponse struct {
	Scores  EvaluationScores  `json:"scores"`
	Reasons EvaluationReasons `json:"reasons"`
}

// StabilityResult aggregates N parallel evaluation runs.
type StabilityResult struct {
	MedianScores  EvaluationScores  `json:"median_scores"`
	MedianReasons EvaluationReasons `json:"median_reasons"`
	Variance      int               `json:"variance"`
	SuccessRuns   int               `json:"success_runs"`
	Confidence    string            `json:"confidence"` // "High" | "Medium" | "Low"
}

const evalSystem = "你是專業小說評審，只輸出 JSON，不輸出任何額外文字。請用繁體中文填寫 reasons 欄位。"

func evalPrompt(chapter string) string {
	return fmt.Sprintf(`評估以下章節，對五個維度各給 1–5 整數分（5 最高），並為每個維度附上一句繁體中文說明。
輸出嚴格 JSON，不含任何其他文字：
{
  "plot": <1-5>,
  "character": <1-5>,
  "style": <1-5>,
  "pacing": <1-5>,
  "hook": <1-5>,
  "reasons": {
    "plot": "...",
    "character": "...",
    "style": "...",
    "pacing": "...",
    "hook": "..."
  }
}

維度定義：
- plot（情節）：故事推進、轉折、因果邏輯
- character（人物）：角色動機、個性、一致性
- style（文風）：語言質感、意象、文字張力
- pacing（節奏）：段落節奏、張弛、閱讀流暢度
- hook（鉤子）：吸引讀者繼續閱讀的懸念與張力

章節內容：
%s`, chapter)
}

// WeightedScore converts EvaluationScores to a deterministic 0–100 integer.
// Weights: plot 25, character 25, style 20, pacing 15, hook 15.
func WeightedScore(s EvaluationScores) int {
	return s.Plot*5 + s.Character*5 + s.Style*4 + s.Pacing*3 + s.Hook*3
}

func parseEvalResponse(raw string) (*EvaluationResponse, error) {
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("無法在回應中找到 JSON 物件")
	}
	var flat struct {
		Plot      int               `json:"plot"`
		Character int               `json:"character"`
		Style     int               `json:"style"`
		Pacing    int               `json:"pacing"`
		Hook      int               `json:"hook"`
		Reasons   EvaluationReasons `json:"reasons"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &flat); err != nil {
		return nil, fmt.Errorf("JSON 解析失敗：%w", err)
	}
	resp := &EvaluationResponse{
		Scores: EvaluationScores{
			Plot:      clampScore(flat.Plot),
			Character: clampScore(flat.Character),
			Style:     clampScore(flat.Style),
			Pacing:    clampScore(flat.Pacing),
			Hook:      clampScore(flat.Hook),
		},
		Reasons: flat.Reasons,
	}
	return resp, nil
}

func clampScore(v int) int {
	if v < 1 {
		return 1
	}
	if v > 5 {
		return 5
	}
	return v
}

// EvaluateChapter runs 3 parallel LLM evaluations and returns aggregated median scores.
func (c *Checker) EvaluateChapter(ctx context.Context, chapter, systemPrefix string, onProgress func(run, total int)) (*StabilityResult, error) {
	const total = 3
	type runResult struct {
		resp *EvaluationResponse
		err  error
	}
	results := make([]runResult, total)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var completed atomic.Int32

	systemPrompt := evalSystem
	if systemPrefix != "" {
		systemPrompt = systemPrefix + "\n\n" + evalSystem
	}
	prompt := evalPrompt(chapter)

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			var buf strings.Builder
			err := c.stream(ctx, systemPrompt, prompt, &buf)
			var resp *EvaluationResponse
			if err == nil {
				resp, err = parseEvalResponse(buf.String())
			}
			mu.Lock()
			results[idx] = runResult{resp: resp, err: err}
			mu.Unlock()
			if onProgress != nil {
				onProgress(int(completed.Add(1)), total)
			}
		}(i)
	}
	wg.Wait()

	var successes []*EvaluationResponse
	for _, r := range results {
		if r.err == nil && r.resp != nil {
			successes = append(successes, r.resp)
		}
	}
	if len(successes) == 0 {
		return nil, fmt.Errorf("所有評估執行均失敗")
	}

	// Sort by weighted score to pick median
	sort.Slice(successes, func(i, j int) bool {
		return WeightedScore(successes[i].Scores) < WeightedScore(successes[j].Scores)
	})
	medianIdx := len(successes) / 2
	median := successes[medianIdx]

	// Variance = max - min of weighted scores
	minScore := WeightedScore(successes[0].Scores)
	maxScore := WeightedScore(successes[len(successes)-1].Scores)
	variance := maxScore - minScore

	confidence := "Low"
	if variance <= 3 && len(successes) == total {
		confidence = "High"
	} else if variance <= 7 {
		confidence = "Medium"
	}

	reasons := median.Reasons
	if reasons == nil {
		reasons = EvaluationReasons{}
	}

	return &StabilityResult{
		MedianScores:  median.Scores,
		MedianReasons: reasons,
		Variance:      variance,
		SuccessRuns:   len(successes),
		Confidence:    confidence,
	}, nil
}
