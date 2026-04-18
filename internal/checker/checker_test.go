package checker

import (
	"bytes"
	"context"
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
