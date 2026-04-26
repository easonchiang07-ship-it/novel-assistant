package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeModel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"nomic-embed-text", "nomic-embed-text"},
		{"nomic-embed-text:latest", "nomic-embed-text"},
		{"llama3.1:8b", "llama3.1:8b"},
		{"llama3.2:latest", "llama3.2"},
	}
	for _, c := range cases {
		if got := normalizeModel(c.in); got != c.want {
			t.Errorf("normalizeModel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPullTarget(t *testing.T) {
	cases := []struct{ in, want string }{
		{"llama3.2", entryPullTarget},
		{"llama3.2:latest", entryPullTarget},
		{"llama3.2:3b", entryPullTarget},
		{"llama3.1:8b", "llama3.1:8b"},
		{"nomic-embed-text", "nomic-embed-text"},
	}
	for _, c := range cases {
		if got := pullTarget(c.in); got != c.want {
			t.Errorf("pullTarget(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestModelReady(t *testing.T) {
	reported := []string{"llama3.2:latest", "nomic-embed-text:latest", "llama3.1:8b"}
	cases := []struct {
		configured string
		want       bool
	}{
		{"llama3.2", true},
		{"llama3.2:latest", true},
		{"llama3.2:3b", true},
		{"nomic-embed-text", true},
		{"nomic-embed-text:latest", true},
		{"llama3.1:8b", true},
		{"gemma3:27b", false},
		{"llama3.3:70b", false},
	}
	for _, c := range cases {
		if got := modelReady(c.configured, reported); got != c.want {
			t.Errorf("modelReady(%q, reported) = %v, want %v", c.configured, got, c.want)
		}
	}
}

func ollamaMockServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
}

func TestHandleOllamaStatus_Down(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "", http.StatusServiceUnavailable)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()
	s := newTestServerWithOllama(t, srv.URL, "llama3.2", "nomic-embed-text")
	rec := newTestRequest(t, s, "GET", "/api/ollama/status", nil)
	var resp ollamaStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Running {
		t.Error("expected running=false when Ollama is down")
	}
}

func TestHandleOllamaStatus_AllReady(t *testing.T) {
	t.Parallel()
	mock := ollamaMockServer(t, 200, `{"models":[{"name":"llama3.2:latest"},{"name":"nomic-embed-text:latest"}]}`)
	defer mock.Close()
	s := newTestServerWithOllama(t, mock.URL, "llama3.2", "nomic-embed-text")
	rec := newTestRequest(t, s, "GET", "/api/ollama/status", nil)
	var resp ollamaStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.Running || !resp.LLMReady || !resp.EmbedReady {
		t.Errorf("expected all ready, got running=%v llm=%v embed=%v", resp.Running, resp.LLMReady, resp.EmbedReady)
	}
}

func TestHandleOllamaStatus_MissingLLM(t *testing.T) {
	t.Parallel()
	mock := ollamaMockServer(t, 200, `{"models":[{"name":"nomic-embed-text:latest"}]}`)
	defer mock.Close()
	s := newTestServerWithOllama(t, mock.URL, "llama3.1:8b", "nomic-embed-text")
	rec := newTestRequest(t, s, "GET", "/api/ollama/status", nil)
	var resp ollamaStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.LLMReady {
		t.Error("expected llm_ready=false")
	}
	if !resp.EmbedReady {
		t.Error("expected embed_ready=true")
	}
}

func TestHandleOllamaStatus_MissingEmbed(t *testing.T) {
	t.Parallel()
	mock := ollamaMockServer(t, 200, `{"models":[{"name":"llama3.2:3b"}]}`)
	defer mock.Close()
	s := newTestServerWithOllama(t, mock.URL, "llama3.2", "nomic-embed-text")
	rec := newTestRequest(t, s, "GET", "/api/ollama/status", nil)
	var resp ollamaStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.LLMReady {
		t.Error("expected llm_ready=true (entry alias)")
	}
	if resp.EmbedReady {
		t.Error("expected embed_ready=false")
	}
}

func TestHandleOllamaStatus_LatestNormalize(t *testing.T) {
	t.Parallel()
	mock := ollamaMockServer(t, 200, `{"models":[{"name":"llama3.1:8b"},{"name":"nomic-embed-text"}]}`)
	defer mock.Close()
	s := newTestServerWithOllama(t, mock.URL, "llama3.1:8b", "nomic-embed-text:latest")
	rec := newTestRequest(t, s, "GET", "/api/ollama/status", nil)
	var resp ollamaStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.LLMReady || !resp.EmbedReady {
		t.Errorf("expected both ready after :latest normalize, got llm=%v embed=%v", resp.LLMReady, resp.EmbedReady)
	}
}

func TestHandleOllamaStatus_EntryAlias(t *testing.T) {
	t.Parallel()
	mock := ollamaMockServer(t, 200, `{"models":[{"name":"llama3.2:latest"},{"name":"nomic-embed-text"}]}`)
	defer mock.Close()
	s := newTestServerWithOllama(t, mock.URL, "llama3.2:3b", "nomic-embed-text")
	rec := newTestRequest(t, s, "GET", "/api/ollama/status", nil)
	var resp ollamaStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.LLMReady {
		t.Error("expected llm_ready=true: llama3.2:3b alias matches llama3.2:latest")
	}
	if resp.LLMPullModel != entryPullTarget {
		t.Errorf("expected llm_pull_model=%q, got %q", entryPullTarget, resp.LLMPullModel)
	}
}

func newTestServerWithOllama(t *testing.T, ollamaURL, llmModel, embedModel string) *Server {
	t.Helper()
	dir := t.TempDir()
	s := newE2ETestServer(t, dir, ollamaURL)
	s.cfg.LLMModel = llmModel
	s.cfg.EmbedModel = embedModel
	return s
}

func newTestRequest(t *testing.T, s *Server, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	s.router.ServeHTTP(rec, req)
	return rec
}
