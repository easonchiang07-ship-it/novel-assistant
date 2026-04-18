package embedder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbedReturnsErrorOnNonOKStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"missing model"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	emb := New(srv.URL, "missing")
	_, err := emb.Embed(context.Background(), "chapter text")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}

func TestEmbedReturnsErrorOnEmptyVector(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"embedding":[]}`))
	}))
	defer srv.Close()

	emb := New(srv.URL, "mock")
	_, err := emb.Embed(context.Background(), "chapter text")
	if err == nil {
		t.Fatal("expected error for empty embedding")
	}
}
