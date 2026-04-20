package checker

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestCheckBehaviorStreamUsesPronounGuidance(t *testing.T) {
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
	if err := c.CheckBehaviorStream(context.Background(), "角色設定", "章節內容", &out); err != nil {
		t.Fatalf("expected stream success, got error: %v", err)
	}
	if !strings.Contains(captured.Prompt, "他 / 她") {
		t.Fatalf("expected pronoun guidance in prompt, got %q", captured.Prompt)
	}
}

func TestCheckBehaviorWithSystemStreamPrependsWorldState(t *testing.T) {
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
	if err := c.CheckBehaviorWithSystemStream(context.Background(), "【當前世界狀態】\n- 林昊：已失去傳家寶劍", "角色設定", "章節內容", &out); err != nil {
		t.Fatalf("expected stream success, got error: %v", err)
	}
	if !strings.Contains(captured.System, "【當前世界狀態】") {
		t.Fatalf("expected system prefix in request, got %q", captured.System)
	}
}

func TestGenerateWorldStateChangesParsesJSONArray(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"response\":\"[{\\\"entity\\\":\\\"林昊\\\",\\\"change_type\\\":\\\"status\\\",\\\"description\\\":\\\"已失去傳家寶劍\\\"}]\",\"done\":true}\n"))
	}))
	defer srv.Close()

	c := New(srv.URL, "mock")
	changes, err := c.GenerateWorldStateChanges(context.Background(), "章節內容")
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}
	if len(changes) != 1 || changes[0].Entity != "林昊" {
		t.Fatalf("unexpected changes: %#v", changes)
	}
}

func TestSplitTextWithOverlapEdgeCases(t *testing.T) {
	t.Parallel()

	if got := splitTextWithOverlap("", 100, 10); len(got) != 0 {
		t.Fatalf("expected nil for empty text, got %v", got)
	}
	exactText := strings.Repeat("a", 100)
	if got := splitTextWithOverlap(exactText, 100, 10); len(got) != 1 || got[0] != exactText {
		t.Fatalf("expected single chunk for text == limit, got %v", got)
	}
	if got := splitTextWithOverlap("hello", 100, 200); len(got) != 1 {
		t.Fatalf("expected single chunk when overlap >= limit, got %v", got)
	}
	twoChunkText := strings.Repeat("字", 150)
	chunks := splitTextWithOverlap(twoChunkText, 100, 20)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if len([]rune(chunks[0])) != 100 {
		t.Fatalf("expected first chunk to be exactly limit, got %d runes", len([]rune(chunks[0])))
	}
}

func TestCheckBehaviorStreamChunksLongTextAndMergesResponses(t *testing.T) {
	t.Parallel()

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"response\":\"1. 行為一致性：符合\\n\",\"done\":false}\n"))
		_, _ = w.Write([]byte("{\"response\":\"2. 具體問題：無明顯衝突\\n\",\"done\":true}\n"))
	}))
	defer srv.Close()

	c := New(srv.URL, "mock")
	var out bytes.Buffer
	longText := strings.Repeat("他在夜裡持續觀察四周。", behaviorChunkRuneLimit/4+50)
	if err := c.CheckBehaviorStream(context.Background(), "角色設定", longText, &out); err != nil {
		t.Fatalf("expected chunked stream success, got error: %v", err)
	}
	if callCount < 2 {
		t.Fatalf("expected multiple chunk calls, got %d", callCount)
	}
	if strings.Count(out.String(), "1. 行為一致性：符合") != 1 {
		t.Fatalf("expected merged output to dedupe repeated lines, got %q", out.String())
	}
}
