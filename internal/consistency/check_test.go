package consistency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckParsesJSONArray(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"response\":\"[{\\\"severity\\\":\\\"error\\\",\\\"description\\\":\\\"主角試圖使用傳家寶劍，但該道具已賣出\\\",\\\"reference\\\":\\\"第3章\\\"}]\",\"done\":true}\n"))
	}))
	defer srv.Close()

	c := New(srv.URL, "mock")
	conflicts, err := c.Check(context.Background(), "主角拔出傳家寶劍", "第3章：傳家寶劍已賣出")
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0].Severity != "error" {
		t.Fatalf("unexpected conflicts: %#v", conflicts)
	}
}

func TestCheckReturnsEmptyList(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"response\":\"[]\",\"done\":true}\n"))
	}))
	defer srv.Close()

	c := New(srv.URL, "mock")
	conflicts, err := c.Check(context.Background(), "主角走進夜港塔", "第1章：主角抵達港口")
	if err != nil {
		t.Fatalf("expected empty parse success, got error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %#v", conflicts)
	}
}
