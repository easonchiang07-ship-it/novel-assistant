package checker

import (
	"context"
	"fmt"
	"testing"
)

func mockEvalResponse(plot, char, style, pacing, hook int) string {
	return fmt.Sprintf(`{"plot":%d,"character":%d,"style":%d,"pacing":%d,"hook":%d,"reasons":{"plot":"測試情節","character":"測試人物","style":"測試文風","pacing":"測試節奏","hook":"測試鉤子"}}`, plot, char, style, pacing, hook)
}

func TestEvaluateChapterMedian(t *testing.T) {
	t.Parallel()

	// Run 1: weighted = 4*5+4*5+4*4+4*3+4*3 = 80
	// Run 2: weighted = 5*5+5*5+5*4+5*3+5*3 = 100
	// Run 3: weighted = 3*5+3*5+3*4+3*3+3*3 = 60
	// Sorted: [60, 80, 100] → median idx=1 → scores all 4s
	c := NewWithStreamer(&sequentialStreamer{responses: []string{
		mockEvalResponse(4, 4, 4, 4, 4),
		mockEvalResponse(5, 5, 5, 5, 5),
		mockEvalResponse(3, 3, 3, 3, 3),
	}})
	result, err := c.EvaluateChapter(context.Background(), "章節文本", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SuccessRuns != 3 {
		t.Errorf("expected 3 success runs, got %d", result.SuccessRuns)
	}
	if result.MedianScores.Plot != 4 {
		t.Errorf("expected median plot=4, got %d", result.MedianScores.Plot)
	}
	if result.Variance != 40 { // 100 - 60
		t.Errorf("expected variance=40, got %d", result.Variance)
	}
}

func TestEvaluateChapterOneRunFails(t *testing.T) {
	t.Parallel()

	// Run 1: valid JSON; Run 2: error; Run 3: valid JSON
	c := NewWithStreamer(&errorOnNthStreamer{
		response: mockEvalResponse(3, 3, 3, 3, 3),
		failOn:   2,
	})
	result, err := c.EvaluateChapter(context.Background(), "章節文本", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SuccessRuns != 2 {
		t.Errorf("expected 2 success runs, got %d", result.SuccessRuns)
	}
}

func TestEvaluateChapterConfidenceBoundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		scores     [3][5]int // [run][dimension]
		wantConf   string
		successAll bool
	}{
		{
			// All runs identical → variance=0 → High
			name:       "identical runs",
			scores:     [3][5]int{{3, 3, 3, 3, 3}, {3, 3, 3, 3, 3}, {3, 3, 3, 3, 3}},
			wantConf:   "High",
			successAll: true,
		},
		{
			// weighted: 60, 80, 100 → variance=40 → Low
			name:       "high variance low",
			scores:     [3][5]int{{3, 3, 3, 3, 3}, {4, 4, 4, 4, 4}, {5, 5, 5, 5, 5}},
			wantConf:   "Low",
			successAll: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := tc.scores
			c := NewWithStreamer(&sequentialStreamer{responses: []string{
				mockEvalResponse(s[0][0], s[0][1], s[0][2], s[0][3], s[0][4]),
				mockEvalResponse(s[1][0], s[1][1], s[1][2], s[1][3], s[1][4]),
				mockEvalResponse(s[2][0], s[2][1], s[2][2], s[2][3], s[2][4]),
			}})
			result, err := c.EvaluateChapter(context.Background(), "章節文本", "", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Confidence != tc.wantConf {
				t.Errorf("expected confidence=%s, got %s (variance=%d)", tc.wantConf, result.Confidence, result.Variance)
			}
		})
	}
}

func TestWeightedScore(t *testing.T) {
	s := EvaluationScores{Plot: 5, Character: 5, Style: 5, Pacing: 5, Hook: 5}
	if got := WeightedScore(s); got != 100 {
		t.Errorf("max score expected 100, got %d", got)
	}
	s2 := EvaluationScores{Plot: 1, Character: 1, Style: 1, Pacing: 1, Hook: 1}
	if got := WeightedScore(s2); got != 20 {
		t.Errorf("min score expected 20, got %d", got)
	}
}
