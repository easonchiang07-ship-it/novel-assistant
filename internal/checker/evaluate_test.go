package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func mockEvalResponse(plot, char, style, pacing, hook int) string {
	return fmt.Sprintf(`{"plot":%d,"character":%d,"style":%d,"pacing":%d,"hook":%d,"reasons":{"plot":"測試情節","character":"測試人物","style":"測試文風","pacing":"測試節奏","hook":"測試鉤子"}}`, plot, char, style, pacing, hook)
}

func ollamaEvalServer(responses []string) *httptest.Server {
	var callCount atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(callCount.Add(1)) - 1
		if n >= len(responses) {
			http.Error(w, "unexpected call", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		chunk, _ := json.Marshal(map[string]any{"response": responses[n], "done": true})
		w.Write(chunk)
	}))
}

func TestEvaluateChapterMedian(t *testing.T) {
	t.Parallel()

	// Run 1: weighted = 4*5+4*5+4*4+4*3+4*3 = 80
	// Run 2: weighted = 5*5+5*5+5*4+5*3+5*3 = 100
	// Run 3: weighted = 3*5+3*5+3*4+3*3+3*3 = 60
	// Sorted: [60, 80, 100] → median idx=1 → scores all 4s
	responses := []string{
		mockEvalResponse(4, 4, 4, 4, 4),
		mockEvalResponse(5, 5, 5, 5, 5),
		mockEvalResponse(3, 3, 3, 3, 3),
	}
	srv := ollamaEvalServer(responses)
	defer srv.Close()

	c := New(srv.URL, "mock")
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

	// Run 1: valid JSON
	// Run 2: HTTP 500 (parse fails)
	// Run 3: valid JSON
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(callCount.Add(1))
		if n == 2 {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := mockEvalResponse(3, 3, 3, 3, 3)
		chunk, _ := json.Marshal(map[string]any{"response": resp, "done": true})
		w.Write(chunk)
	}))
	defer srv.Close()

	c := New(srv.URL, "mock")
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
			// variance=7: Low (> 7 means Low; exactly 7 is Medium)
			// weighted: 3*5+3*5+3*4+3*3+3*3=60, 4*5+4*5+4*4+4*3+4*3=80, 5*5+5*5+5*4+5*3+5*3=100
			// max-min = 100-60 = 40 → Low
			name:       "high variance low",
			scores:     [3][5]int{{3, 3, 3, 3, 3}, {4, 4, 4, 4, 4}, {5, 5, 5, 5, 5}},
			wantConf:   "Low",
			successAll: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var callCount atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := int(callCount.Add(1)) - 1
				s := tc.scores[n%3]
				w.Header().Set("Content-Type", "application/json")
				resp := mockEvalResponse(s[0], s[1], s[2], s[3], s[4])
				chunk, _ := json.Marshal(map[string]any{"response": resp, "done": true})
				w.Write(chunk)
			}))
			defer srv.Close()

			c := New(srv.URL, "mock")
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
