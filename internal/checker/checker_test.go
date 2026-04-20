package checker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"novel-assistant/internal/profile"
	"strings"
	"testing"
)

func TestStreamReturnsErrorOnNonOKStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New(srv.URL, "mock")
	var out bytes.Buffer
	err := c.stream(context.Background(), "system", "prompt", &out)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}

func TestExtractNamesFindsMentionedCharacters(t *testing.T) {
	t.Parallel()

	names := ExtractNames("林昊看向張雷，沒有回答。", []string{"林昊", "張雷", "王雪"})
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "林昊" || names[1] != "張雷" {
		t.Fatalf("unexpected names: %v", names)
	}
}

func TestCheckWorldConflictStreamUsesWorldPrompt(t *testing.T) {
	t.Parallel()

	var captured genReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"response\":\"ok\",\"done\":true}\n"))
	}))
	defer srv.Close()

	c := New(srv.URL, "mock")
	var out bytes.Buffer
	if err := c.CheckWorldConflictStream(context.Background(), "世界規則", "章節內容", &out); err != nil {
		t.Fatalf("expected stream success, got error: %v", err)
	}
	if !strings.Contains(captured.Prompt, "世界觀與規則設定") {
		t.Fatalf("expected world prompt marker, got %q", captured.Prompt)
	}
	if !strings.Contains(out.String(), "ok") {
		t.Fatalf("expected output to contain response, got %q", out.String())
	}
}

func TestAnalyzeStyleParsesJSONResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"response\":\"{\\\"dialogue_ratio\\\":\\\"高\\\",\\\"sensory_freq\\\":\\\"中\\\",\\\"avg_sentence_len\\\":\\\"綿長\\\",\\\"tone\\\":\\\"詩意\\\",\\\"summary\\\":\\\"意象濃厚，句子拉長\\\"}\",\"done\":true}\n"))
	}))
	defer srv.Close()

	c := New(srv.URL, "mock")
	got, err := c.AnalyzeStyle(context.Background(), "一段文字")
	if err != nil {
		t.Fatalf("expected analyze style success, got error: %v", err)
	}

	want := &profile.StyleAnalysis{
		DialogueRatio:  "高",
		SensoryFreq:    "中",
		AvgSentenceLen: "綿長",
		Tone:           "詩意",
		Summary:        "意象濃厚，句子拉長",
	}
	if *got != *want {
		t.Fatalf("unexpected analysis: got %#v want %#v", got, want)
	}
}

func TestAnalyzeStyleRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"response\":\"not-json\",\"done\":true}\n"))
	}))
	defer srv.Close()

	c := New(srv.URL, "mock")
	if _, err := c.AnalyzeStyle(context.Background(), "一段文字"); err == nil {
		t.Fatal("expected malformed JSON error")
	} else if !errors.Is(err, ErrStyleParseFailure) {
		t.Fatalf("expected ErrStyleParseFailure, got %v", err)
	}
}
