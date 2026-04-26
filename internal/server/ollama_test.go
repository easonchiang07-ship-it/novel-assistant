package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ─── unit: normalize helpers ─────────────────────────────────────────────────

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
		{"gemma3:27b", "gemma3:27b"},
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
		// Entry alias: any alias matches any alias in reported
		{"llama3.2", true},
		{"llama3.2:latest", true},
		{"llama3.2:3b", true},
		// General :latest strip
		{"nomic-embed-text", true},
		{"nomic-embed-text:latest", true},
		// Exact (after strip)
		{"llama3.1:8b", true},
		// Not present
		{"gemma3:27b", false},
		{"llama3.3:70b", false},
	}
	for _, c := range cases {
		if got := modelReady(c.configured, reported); got != c.want {
			t.Errorf("modelReady(%q, reported) = %v, want %v", c.configured, got, c.want)
		}
	}
}

// ─── integration: handleOllamaStatus ─────────────────────────────────────────

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
	// Point to a server that immediately rejects connections
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection immediately to simulate Ollama being down
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

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp ollamaStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Running {
		t.Error("expected running=false when Ollama is down")
	}
	if resp.LLMReady || resp.EmbedReady {
		t.Error("expected llm_ready=false and embed_ready=false when Ollama is down")
	}
}

func TestHandleOllamaStatus_AllReady(t *testing.T) {
	t.Parallel()
	body := `{"models":[{"name":"llama3.2:latest"},{"name":"nomic-embed-text:latest"}]}`
	mock := ollamaMockServer(t, 200, body)
	defer mock.Close()

	s := newTestServerWithOllama(t, mock.URL, "llama3.2", "nomic-embed-text")
	rec := newTestRequest(t, s, "GET", "/api/ollama/status", nil)

	var resp ollamaStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if !resp.Running {
		t.Error("expected running=true")
	}
	if !resp.LLMReady {
		t.Error("expected llm_ready=true for llama3.2 matched against llama3.2:latest")
	}
	if !resp.EmbedReady {
		t.Error("expected embed_ready=true for nomic-embed-text matched against nomic-embed-text:latest")
	}
}

func TestHandleOllamaStatus_MissingLLM(t *testing.T) {
	t.Parallel()
	body := `{"models":[{"name":"nomic-embed-text:latest"}]}`
	mock := ollamaMockServer(t, 200, body)
	defer mock.Close()

	s := newTestServerWithOllama(t, mock.URL, "llama3.1:8b", "nomic-embed-text")
	rec := newTestRequest(t, s, "GET", "/api/ollama/status", nil)

	var resp ollamaStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if !resp.Running {
		t.Error("expected running=true")
	}
	if resp.LLMReady {
		t.Error("expected llm_ready=false")
	}
	if !resp.EmbedReady {
		t.Error("expected embed_ready=true")
	}
}

func TestHandleOllamaStatus_MissingEmbed(t *testing.T) {
	t.Parallel()
	body := `{"models":[{"name":"llama3.2:3b"}]}`
	mock := ollamaMockServer(t, 200, body)
	defer mock.Close()

	s := newTestServerWithOllama(t, mock.URL, "llama3.2", "nomic-embed-text")
	rec := newTestRequest(t, s, "GET", "/api/ollama/status", nil)

	var resp ollamaStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if !resp.LLMReady {
		t.Error("expected llm_ready=true (llama3.2 entry alias matches llama3.2:3b)")
	}
	if resp.EmbedReady {
		t.Error("expected embed_ready=false")
	}
}

func TestHandleOllamaStatus_LatestNormalize(t *testing.T) {
	t.Parallel()
	// Ollama reports llama3.1:8b:latest (unusual but possible)
	body := `{"models":[{"name":"llama3.1:8b"},{"name":"nomic-embed-text"}]}`
	mock := ollamaMockServer(t, 200, body)
	defer mock.Close()

	s := newTestServerWithOllama(t, mock.URL, "llama3.1:8b", "nomic-embed-text:latest")
	rec := newTestRequest(t, s, "GET", "/api/ollama/status", nil)

	var resp ollamaStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if !resp.LLMReady {
		t.Error("expected llm_ready=true")
	}
	if !resp.EmbedReady {
		t.Error("expected embed_ready=true for nomic-embed-text:latest matched against nomic-embed-text")
	}
}

func TestHandleOllamaStatus_EntryAlias(t *testing.T) {
	t.Parallel()
	// Configured as llama3.2:3b; Ollama reports llama3.2:latest
	body := `{"models":[{"name":"llama3.2:latest"},{"name":"nomic-embed-text"}]}`
	mock := ollamaMockServer(t, 200, body)
	defer mock.Close()

	s := newTestServerWithOllama(t, mock.URL, "llama3.2:3b", "nomic-embed-text")
	rec := newTestRequest(t, s, "GET", "/api/ollama/status", nil)

	var resp ollamaStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if !resp.LLMReady {
		t.Error("expected llm_ready=true: llama3.2:3b entry alias should match llama3.2:latest")
	}
	if resp.LLMPullModel != entryPullTarget {
		t.Errorf("expected llm_pull_model=%q, got %q", entryPullTarget, resp.LLMPullModel)
	}
}

// ─── integration: handleOllamaPull ───────────────────────────────────────────

func TestHandleOllamaPull_ForwardsProgress(t *testing.T) {
	t.Parallel()
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/pull" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"status":"pulling manifest","completed":0,"total":100}`)
			fmt.Fprintln(w, `{"status":"success"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer mock.Close()

	s := newTestServerWithOllama(t, mock.URL, "llama3.2", "nomic-embed-text")
	body := strings.NewReader(`{"model":"llama3.2:3b"}`)
	req := httptest.NewRequest("POST", "/api/ollama/pull", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	resp := rec.Body.String()
	if !strings.Contains(resp, "event: progress") {
		t.Errorf("expected progress event, got: %s", resp)
	}
	if !strings.Contains(resp, "event: done") {
		t.Errorf("expected done event, got: %s", resp)
	}
}

func TestHandleOllamaPull_ForwardsError(t *testing.T) {
	t.Parallel()
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/pull" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"error":"model 'nosuchmodel' not found"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer mock.Close()

	s := newTestServerWithOllama(t, mock.URL, "llama3.2", "nomic-embed-text")
	body := strings.NewReader(`{"model":"nosuchmodel"}`)
	req := httptest.NewRequest("POST", "/api/ollama/pull", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	resp := rec.Body.String()
	if !strings.Contains(resp, "event: error") {
		t.Errorf("expected error event, got: %s", resp)
	}
	if strings.Contains(resp, "event: done") {
		t.Errorf("should not emit done after error, got: %s", resp)
	}
}

func TestHandleOllamaPull_DuplicateReturns409(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	unblock := make(chan struct{})
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/pull" {
			close(started)
			<-unblock
			fmt.Fprintln(w, `{"status":"success"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer mock.Close()
	defer close(unblock)

	s := newTestServerWithOllama(t, mock.URL, "llama3.2", "nomic-embed-text")

	go func() {
		body := strings.NewReader(`{"model":"slowmodel"}`)
		req := httptest.NewRequest("POST", "/api/ollama/pull", body)
		req.Header.Set("Content-Type", "application/json")
		s.router.ServeHTTP(httptest.NewRecorder(), req)
	}()

	<-started

	body := strings.NewReader(`{"model":"slowmodel"}`)
	req := httptest.NewRequest("POST", "/api/ollama/pull", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestHandleOllamaPull_ReleasesLockAfterCompletion(t *testing.T) {
	t.Parallel()

	// Verify the inflight lock is released via defer after the handler finishes,
	// regardless of whether the client cancelled or the stream completed normally.
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/pull" {
			// Write headers then signal done; scanner will see EOF and handler will return.
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			fmt.Fprintln(w, `{"status":"success"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer mock.Close()

	s := newTestServerWithOllama(t, mock.URL, "llama3.2", "nomic-embed-text")
	body := strings.NewReader(`{"model":"lockmodel"}`)
	req := httptest.NewRequest("POST", "/api/ollama/pull", body)
	req.Header.Set("Content-Type", "application/json")
	s.router.ServeHTTP(httptest.NewRecorder(), req)

	inflightMu.Lock()
	_, locked := inflight["lockmodel"]
	inflightMu.Unlock()

	if locked {
		t.Error("inflight lock not released after handler completion")
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

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
