package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"novel-assistant/internal/checker"
	"novel-assistant/internal/config"
	"novel-assistant/internal/embedder"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/projectsettings"
	"novel-assistant/internal/reviewhistory"
	"novel-assistant/internal/reviewrules"
	"novel-assistant/internal/tracker"
	"novel-assistant/internal/vectorstore"

	"github.com/gin-gonic/gin"
)

func TestE2EChapterReviewRewriteWritebackAndHistoryExport(t *testing.T) {
	t.Parallel()

	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float64{0.1, 0.2, 0.3}})
		case "/api/generate":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"response\":\"ok\",\"done\":true}\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "characters", "林昊.md"), "# 角色：林昊\n- 個性：冷靜\n- 說話風格：短句\n")
	mustWriteFile(t, filepath.Join(dir, "worldbuilding", "城市規則.md"), "# 城市規則\n- 夜晚才會顯影\n")
	mustWriteFile(t, filepath.Join(dir, "style", "主線敘事.md"), "# 風格：主線敘事\n- 敘事視角：第三人稱有限視角\n")

	s := newE2ETestServer(t, dir, ollama.URL)
	if err := s.Ingest(context.Background()); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	app := httptest.NewServer(s.router)
	defer app.Close()

	chapterResp := performJSONRequest(t, app.URL, "POST", "/api/chapters", map[string]any{
		"name":    "第01章",
		"content": "林昊站在夜港塔下。",
	})
	if chapterResp.StatusCode != http.StatusOK {
		t.Fatalf("save chapter failed: %s", string(chapterResp.Body))
	}

	getResp := performRequest(t, app.URL, "GET", "/api/chapters/%E7%AC%AC01%E7%AB%A0.md", nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("load chapter failed: %s", string(getResp.Body))
	}

	checkResp := performJSONRequest(t, app.URL, "POST", "/check/stream", map[string]any{
		"chapter":       "林昊站在夜港塔下。",
		"checks":        []string{"behavior"},
		"chapter_file":  "第01章.md",
		"chapter_title": "第01章",
	})
	if checkResp.StatusCode != http.StatusOK || !strings.Contains(string(checkResp.Body), "ok") {
		t.Fatalf("check stream failed: %s", string(checkResp.Body))
	}

	rewriteResp := performJSONRequest(t, app.URL, "POST", "/rewrite/stream", map[string]any{
		"chapter":       "林昊站在夜港塔下。",
		"mode":          "conservative",
		"chapter_file":  "第01章.md",
		"chapter_title": "第01章",
	})
	if rewriteResp.StatusCode != http.StatusOK || !strings.Contains(string(rewriteResp.Body), "ok") {
		t.Fatalf("rewrite stream failed: %s", string(rewriteResp.Body))
	}

	writebackResp := performJSONRequest(t, app.URL, "POST", "/api/writeback/timeline", map[string]any{
		"chapter":      1,
		"scene":        "夜港塔",
		"description":  "林昊抵達現場",
		"characters":   []string{"林昊"},
		"consequences": "後續調查展開",
	})
	if writebackResp.StatusCode != http.StatusOK {
		t.Fatalf("timeline writeback failed: %s", string(writebackResp.Body))
	}

	exportResp := performJSONRequest(t, app.URL, "POST", "/api/history/export", map[string]any{})
	if exportResp.StatusCode != http.StatusOK || !strings.Contains(string(exportResp.Body), "審查歷史匯出") {
		t.Fatalf("history export failed: %s", string(exportResp.Body))
	}
}

func newE2ETestServer(t *testing.T, dataDir, ollamaURL string) *Server {
	t.Helper()

	cfg := &config.Config{
		OllamaURL:  ollamaURL,
		LLMModel:   "mock-llm",
		EmbedModel: "mock-embed",
		DataDir:    dataDir,
		Port:       "8080",
	}

	gin.SetMode(gin.TestMode)
	s := &Server{
		cfg:           cfg,
		project:       projectsettings.New(filepath.Join(dataDir, "project_settings.json"), projectsettings.Settings{OllamaURL: cfg.OllamaURL, LLMModel: cfg.LLMModel, EmbedModel: cfg.EmbedModel, Port: cfg.Port, DataDir: cfg.DataDir}),
		profiles:      profile.NewManager(dataDir),
		store:         vectorstore.New(filepath.Join(dataDir, "store.json")),
		embedder:      embedder.New(cfg.OllamaURL, cfg.EmbedModel),
		checker:       checker.New(cfg.OllamaURL, cfg.LLMModel),
		rules:         reviewrules.New(filepath.Join(dataDir, "review_rules.json")),
		history:       reviewhistory.New(filepath.Join(dataDir, "reviews.json")),
		relationships: tracker.NewRelationshipTracker(filepath.Join(dataDir, "relationships.json")),
		timeline:      tracker.NewTimelineTracker(filepath.Join(dataDir, "timeline.json")),
		foreshadow:    tracker.NewForeshadowTracker(filepath.Join(dataDir, "foreshadow.json")),
	}
	if err := s.profiles.Load(); err != nil {
		t.Fatalf("load profiles: %v", err)
	}
	s.router = gin.New()
	s.setupRoutes()
	return s
}

type httpResult struct {
	StatusCode int
	Body       []byte
}

func performJSONRequest(t *testing.T, baseURL, method, path string, payload any) httpResult {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return performRequest(t, baseURL, method, path, bytes.NewReader(body))
}

func performRequest(t *testing.T, baseURL, method, path string, body *bytes.Reader) httpResult {
	t.Helper()

	var reader io.Reader
	if body == nil {
		reader = http.NoBody
	} else {
		reader = body
	}

	req, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return httpResult{StatusCode: resp.StatusCode, Body: data}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
