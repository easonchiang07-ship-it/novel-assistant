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
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"novel-assistant/internal/checker"
	"novel-assistant/internal/config"
	"novel-assistant/internal/consistency"
	"novel-assistant/internal/embedder"
	"novel-assistant/internal/profile"
	"novel-assistant/internal/projectsettings"
	"novel-assistant/internal/reviewhistory"
	"novel-assistant/internal/reviewrules"
	"novel-assistant/internal/tracker"
	"novel-assistant/internal/vectorstore"
	"novel-assistant/internal/worldstate"

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
	if !strings.Contains(string(checkResp.Body), "event:retrieval") {
		t.Fatalf("expected retrieval metadata event in stream, got %s", string(checkResp.Body))
	}
	if !strings.Contains(string(checkResp.Body), "event:gaps") {
		t.Fatalf("expected retrieval gaps event in stream, got %s", string(checkResp.Body))
	}
	if !strings.Contains(string(checkResp.Body), "\"index_ready\":true") {
		t.Fatalf("expected indexed gap payload in stream, got %s", string(checkResp.Body))
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

func TestIngestStoresMultipleChapterChunks(t *testing.T) {
	t.Parallel()

	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float64{0.1, 0.2, 0.3}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "chapters", "第01章.md"), "## Scene 1: Opening\nA\n\n## Scene 2: Rain\nB")

	s := newE2ETestServer(t, dir, ollama.URL)
	if err := s.Ingest(context.Background()); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "store.json"))
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if strings.Count(string(raw), "\"type\": \"chapter\"") <= 1 {
		t.Fatalf("expected multiple chapter documents in store.json, got %s", string(raw))
	}
}

func TestCheckStreamSourcesExposeChunkMetadata(t *testing.T) {
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
	mustWriteFile(t, filepath.Join(dir, "chapters", "第02章.md"), "## Scene 1: Rain Dock\n林昊在雨夜碼頭停下腳步。")

	s := newE2ETestServer(t, dir, ollama.URL)
	if err := s.Ingest(context.Background()); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	app := httptest.NewServer(s.router)
	defer app.Close()

	resp := performJSONRequest(t, app.URL, "POST", "/check/stream", map[string]any{
		"chapter": "林昊再次想起雨夜碼頭的腳步聲。",
		"checks":  []string{"behavior"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("check stream failed: %s", string(resp.Body))
	}

	items := decodeSSEItems(t, string(resp.Body))
	if len(items) == 0 {
		t.Fatalf("expected sources payload in stream, got %s", string(resp.Body))
	}
	found := false
	for _, item := range items {
		if item.ChapterFile == "第02章.md" && item.ChapterIndex == 2 && item.SceneIndex == 1 && item.ChunkType == "scene" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected chunk metadata in sources payload, got %#v", items)
	}
}

func TestCheckStreamMarksGapsAsUnindexedWhenStoreIsEmpty(t *testing.T) {
	t.Parallel()

	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
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

	s := newE2ETestServer(t, dir, ollama.URL)
	app := httptest.NewServer(s.router)
	defer app.Close()

	checkResp := performJSONRequest(t, app.URL, "POST", "/check/stream", map[string]any{
		"chapter": "林昊走進夜港塔下。影潮契約已經啟動。",
		"checks":  []string{"behavior"},
	})
	if checkResp.StatusCode != http.StatusOK {
		t.Fatalf("check stream failed: %s", string(checkResp.Body))
	}
	if !strings.Contains(string(checkResp.Body), "event:gaps") {
		t.Fatalf("expected retrieval gaps event in stream, got %s", string(checkResp.Body))
	}
	if !strings.Contains(string(checkResp.Body), "\"index_ready\":false") {
		t.Fatalf("expected unindexed gap payload in stream, got %s", string(checkResp.Body))
	}
}

func TestCheckStreamPipelineEmitsOrderedLayerEvents(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float64{0.1, 0.2, 0.3}})
		case "/api/generate":
			callCount.Add(1)
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

	s := newE2ETestServer(t, dir, ollama.URL)
	if err := s.Ingest(context.Background()); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	app := httptest.NewServer(s.router)
	defer app.Close()

	resp := performJSONRequest(t, app.URL, "POST", "/check/stream", map[string]any{
		"chapter":    "林昊站在夜港塔下。",
		"layer_mode": "pipeline",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pipeline stream failed: %s", string(resp.Body))
	}

	body := string(resp.Body)
	order := []string{
		"event:layer_start\ndata:{\"label\":\"結構層\",\"layer\":\"structure\"}",
		"event:layer_end\ndata:{\"layer\":\"structure\"}",
		"event:layer_start\ndata:{\"label\":\"角色層\",\"layer\":\"character\"}",
		"event:layer_end\ndata:{\"layer\":\"character\"}",
		"event:layer_start\ndata:{\"label\":\"世界觀層\",\"layer\":\"world_logic\"}",
		"event:layer_end\ndata:{\"layer\":\"world_logic\"}",
		"event:layer_start\ndata:{\"label\":\"語言層\",\"layer\":\"language\"}",
		"event:layer_end\ndata:{\"layer\":\"language\"}",
	}

	last := -1
	for _, marker := range order {
		idx := strings.Index(body, marker)
		if idx < 0 {
			t.Fatalf("expected marker %q in stream, got %s", marker, body)
		}
		if idx <= last {
			t.Fatalf("expected ordered markers, got %s", body)
		}
		last = idx
	}
	if n := callCount.Load(); n != 4 {
		t.Fatalf("expected 4 generate calls, got %d", n)
	}
}

func TestCheckStreamPipelineAbortsOnFirstLayerFailure(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float64{0.1, 0.2, 0.3}})
		case "/api/generate":
			n := callCount.Add(1)
			if n == 1 {
				// First layer (structure) fails: return 500 to trigger layer error.
				http.Error(w, "model overloaded", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"response\":\"ok\",\"done\":true}\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "characters", "林昊.md"), "# 角色：林昊\n- 個性：冷靜\n")
	mustWriteFile(t, filepath.Join(dir, "worldbuilding", "城市規則.md"), "# 城市規則\n- 夜晚才會顯影\n")

	s := newE2ETestServer(t, dir, ollama.URL)
	if err := s.Ingest(context.Background()); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	app := httptest.NewServer(s.router)
	defer app.Close()

	resp := performJSONRequest(t, app.URL, "POST", "/check/stream", map[string]any{
		"chapter":    "林昊站在夜港塔下。",
		"layer_mode": "pipeline",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pipeline stream failed with status %d", resp.StatusCode)
	}

	body := string(resp.Body)

	// structure layer_start must appear (pipeline started)
	if !strings.Contains(body, "event:layer_start\ndata:{\"label\":\"結構層\",\"layer\":\"structure\"}") {
		t.Fatalf("expected structure layer_start in stream, got %s", body)
	}
	// failed layer must NOT have layer_end
	if strings.Contains(body, "event:layer_end\ndata:{\"layer\":\"structure\"}") {
		t.Fatalf("failed layer must not emit layer_end, got %s", body)
	}
	// subsequent layers must not appear
	if strings.Contains(body, "event:layer_start\ndata:{\"label\":\"角色層\",\"layer\":\"character\"}") {
		t.Fatalf("pipeline should abort after first failure, got character layer in %s", body)
	}
	// only one generate call (structure layer)
	if n := callCount.Load(); n != 1 {
		t.Fatalf("expected exactly 1 generate call before abort, got %d", n)
	}
}

func TestCheckStreamSingleDoesNotEmitLayerEvents(t *testing.T) {
	t.Parallel()

	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
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

	s := newE2ETestServer(t, dir, ollama.URL)
	app := httptest.NewServer(s.router)
	defer app.Close()

	resp := performJSONRequest(t, app.URL, "POST", "/check/stream", map[string]any{
		"chapter":    "林昊站在夜港塔下。",
		"checks":     []string{"behavior"},
		"layer_mode": "single",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("single stream failed: %s", string(resp.Body))
	}
	if strings.Contains(string(resp.Body), "event:layer_start") || strings.Contains(string(resp.Body), "event:layer_end") {
		t.Fatalf("single mode should not emit layer events, got %s", string(resp.Body))
	}
}

func TestCreateWorldstateSnapshotAndList(t *testing.T) {
	t.Parallel()

	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"response\":\"[{\\\"entity\\\":\\\"林昊\\\",\\\"change_type\\\":\\\"status\\\",\\\"description\\\":\\\"已失去傳家寶劍\\\"}]\",\"done\":true}\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "chapters", "第02章.md"), "林昊把傳家寶劍賣了出去。")

	s := newE2ETestServer(t, dir, ollama.URL)
	app := httptest.NewServer(s.router)
	defer app.Close()

	snapshotResp := performRequest(t, app.URL, "POST", "/api/chapters/%E7%AC%AC02%E7%AB%A0.md/snapshot", nil)
	if snapshotResp.StatusCode != http.StatusOK || !strings.Contains(string(snapshotResp.Body), "已產生章節狀態快照") {
		t.Fatalf("snapshot endpoint failed: %s", string(snapshotResp.Body))
	}

	listResp := performRequest(t, app.URL, "GET", "/api/worldstate", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("worldstate list failed: %s", string(listResp.Body))
	}

	var payload struct {
		Items []struct {
			ChapterFile string `json:"chapter_file"`
			Changes     []struct {
				Description string `json:"description"`
			} `json:"changes"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body, &payload); err != nil {
		t.Fatalf("decode worldstate list: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 snapshot in list payload, got %#v", payload.Items)
	}
	if payload.Items[0].ChapterFile != "第02章.md" {
		t.Fatalf("expected chapter_file 第02章.md, got %#v", payload.Items[0])
	}
	if len(payload.Items[0].Changes) != 1 || payload.Items[0].Changes[0].Description != "已失去傳家寶劍" {
		t.Fatalf("expected snapshot change in list payload, got %#v", payload.Items[0].Changes)
	}
}

func TestCheckAndRewriteUseLatestWorldstateInSystemPrompt(t *testing.T) {
	t.Parallel()

	var (
		mu      sync.Mutex
		systems []string
	)
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			var req struct {
				System string `json:"system"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode generate request: %v", err)
			}
			mu.Lock()
			systems = append(systems, req.System)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"response\":\"ok\",\"done\":true}\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "characters", "林昊.md"), "# 角色：林昊\n- 個性：冷靜\n- 說話風格：短句\n")

	s := newE2ETestServer(t, dir, ollama.URL)
	s.worldstate.Upsert(&worldstate.Snapshot{
		ChapterFile:  "第01章.md",
		ChapterIndex: 1,
		Changes: []worldstate.Change{
			{Entity: "林昊", ChangeType: "status", Description: "已失去傳家寶劍"},
		},
	})
	app := httptest.NewServer(s.router)
	defer app.Close()

	checkResp := performJSONRequest(t, app.URL, "POST", "/check/stream", map[string]any{
		"chapter":      "林昊走進夜港塔下。",
		"checks":       []string{"behavior"},
		"chapter_file": "第02章.md",
	})
	if checkResp.StatusCode != http.StatusOK {
		t.Fatalf("check stream failed: %s", string(checkResp.Body))
	}

	rewriteResp := performJSONRequest(t, app.URL, "POST", "/rewrite/stream", map[string]any{
		"chapter":      "林昊走進夜港塔下。",
		"mode":         "conservative",
		"chapter_file": "第02章.md",
	})
	if rewriteResp.StatusCode != http.StatusOK {
		t.Fatalf("rewrite stream failed: %s", string(rewriteResp.Body))
	}

	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(systems, "\n---\n")
	if !strings.Contains(joined, "【當前世界狀態（截至第 1 章）】") || !strings.Contains(joined, "已失去傳家寶劍") {
		t.Fatalf("expected world state in generate system prompts, got %s", joined)
	}
}

func TestCheckStreamEmitsConflictBeforeMainGeneration(t *testing.T) {
	t.Parallel()

	callCount := 0
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float64{0.1, 0.2, 0.3}})
		case "/api/generate":
			callCount++
			w.Header().Set("Content-Type", "application/json")
			if callCount == 1 {
				_, _ = w.Write([]byte("{\"response\":\"[{\\\"severity\\\":\\\"error\\\",\\\"description\\\":\\\"主角試圖使用傳家寶劍，但該道具已賣出\\\",\\\"reference\\\":\\\"城市規則\\\"}]\",\"done\":true}\n"))
				return
			}
			_, _ = w.Write([]byte("{\"response\":\"ok\",\"done\":true}\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "characters", "林昊.md"), "# 角色：林昊\n- 個性：冷靜\n- 說話風格：短句\n")
	mustWriteFile(t, filepath.Join(dir, "worldbuilding", "城市規則.md"), "# 城市規則\n- 傳家寶劍已賣出\n")

	s := newE2ETestServer(t, dir, ollama.URL)
	if err := s.Ingest(context.Background()); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	app := httptest.NewServer(s.router)
	defer app.Close()

	resp := performJSONRequest(t, app.URL, "POST", "/check/stream", map[string]any{
		"chapter": "林昊拔出傳家寶劍。",
		"checks":  []string{"behavior"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("check stream failed: %s", string(resp.Body))
	}

	body := string(resp.Body)
	sourcesAt := strings.Index(body, "event:sources")
	conflictAt := strings.Index(body, "event:conflict")
	chunkAt := strings.Index(body, "event:chunk")
	if sourcesAt < 0 || conflictAt < 0 {
		t.Fatalf("expected sources/conflict events, got %s", body)
	}
	if sourcesAt >= conflictAt {
		t.Fatalf("expected sources -> conflict ordering, got %s", body)
	}
	if chunkAt >= 0 && conflictAt >= chunkAt {
		t.Fatalf("expected conflict before chunk when chunk is present, got %s", body)
	}
}

func TestRewriteStreamEmitsConflictEvent(t *testing.T) {
	t.Parallel()

	callCount := 0
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float64{0.1, 0.2, 0.3}})
		case "/api/generate":
			callCount++
			w.Header().Set("Content-Type", "application/json")
			if callCount == 1 {
				_, _ = w.Write([]byte("{\"response\":\"[{\\\"severity\\\":\\\"warning\\\",\\\"description\\\":\\\"場景提及已毀的裝置\\\",\\\"reference\\\":\\\"城市規則\\\"}]\",\"done\":true}\n"))
				return
			}
			_, _ = w.Write([]byte("{\"response\":\"rewrite ok\",\"done\":true}\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "characters", "林昊.md"), "# 角色：林昊\n- 個性：冷靜\n- 說話風格：短句\n")
	mustWriteFile(t, filepath.Join(dir, "worldbuilding", "城市規則.md"), "# 城市規則\n- 裝置已毀\n")

	s := newE2ETestServer(t, dir, ollama.URL)
	if err := s.Ingest(context.Background()); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	app := httptest.NewServer(s.router)
	defer app.Close()

	resp := performJSONRequest(t, app.URL, "POST", "/rewrite/stream", map[string]any{
		"chapter": "他重新啟動那台裝置。",
		"mode":    "conservative",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rewrite stream failed: %s", string(resp.Body))
	}
	if !strings.Contains(string(resp.Body), "event:conflict") {
		t.Fatalf("expected conflict event in rewrite stream, got %s", string(resp.Body))
	}
}

func TestE2ESceneEditsPersistAcrossSequentialSaves(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newE2ETestServer(t, dir, "http://127.0.0.1:0")
	app := httptest.NewServer(s.router)
	defer app.Close()

	original := `序章前言

## Scene 1: Opening
Lin Hao opened the door.

## Scene 2: Rain
Zhang Lei stood in the rain.`

	saveResp := performJSONRequest(t, app.URL, "POST", "/api/chapters", map[string]any{
		"name":    "第01章",
		"content": original,
	})
	if saveResp.StatusCode != http.StatusOK {
		t.Fatalf("initial chapter save failed: %s", string(saveResp.Body))
	}

	loadResp := performRequest(t, app.URL, "GET", "/api/chapters/%E7%AC%AC01%E7%AB%A0.md", nil)
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load chapter failed: %s", string(loadResp.Body))
	}

	var loaded struct {
		Content string  `json:"content"`
		Scenes  []Scene `json:"scenes"`
	}
	if err := json.Unmarshal(loadResp.Body, &loaded); err != nil {
		t.Fatalf("decode chapter response: %v", err)
	}
	if len(loaded.Scenes) != 2 {
		t.Fatalf("expected 2 scenes, got %d", len(loaded.Scenes))
	}

	scene1Saved := reconstructChapterForSceneEdit(t, loaded.Content, loaded.Scenes, "Scene 1: Opening", "Lin Hao stepped inside.")
	saveResp = performJSONRequest(t, app.URL, "POST", "/api/chapters", map[string]any{
		"name":    "第01章.md",
		"content": scene1Saved,
	})
	if saveResp.StatusCode != http.StatusOK {
		t.Fatalf("save after scene 1 edit failed: %s", string(saveResp.Body))
	}

	loaded.Content = scene1Saved
	loaded.Scenes[0].Content = "Lin Hao stepped inside."

	scene2Saved := reconstructChapterForSceneEdit(t, loaded.Content, loaded.Scenes, "Scene 2: Rain", "Zhang Lei vanished into the rain.")
	saveResp = performJSONRequest(t, app.URL, "POST", "/api/chapters", map[string]any{
		"name":    "第01章.md",
		"content": scene2Saved,
	})
	if saveResp.StatusCode != http.StatusOK {
		t.Fatalf("save after scene 2 edit failed: %s", string(saveResp.Body))
	}

	reloadResp := performRequest(t, app.URL, "GET", "/api/chapters/%E7%AC%AC01%E7%AB%A0.md", nil)
	if reloadResp.StatusCode != http.StatusOK {
		t.Fatalf("reload chapter failed: %s", string(reloadResp.Body))
	}

	var reloaded struct {
		Content string  `json:"content"`
		Scenes  []Scene `json:"scenes"`
	}
	if err := json.Unmarshal(reloadResp.Body, &reloaded); err != nil {
		t.Fatalf("decode reloaded chapter response: %v", err)
	}
	if len(reloaded.Scenes) != 2 {
		t.Fatalf("expected 2 scenes after reload, got %d", len(reloaded.Scenes))
	}
	if reloaded.Scenes[0].Content != "Lin Hao stepped inside." {
		t.Fatalf("scene 1 content regressed after second save: %q", reloaded.Scenes[0].Content)
	}
	if reloaded.Scenes[1].Content != "Zhang Lei vanished into the rain." {
		t.Fatalf("scene 2 content not saved: %q", reloaded.Scenes[1].Content)
	}
	if !strings.Contains(reloaded.Content, "Lin Hao stepped inside.") || !strings.Contains(reloaded.Content, "Zhang Lei vanished into the rain.") {
		t.Fatalf("reloaded chapter content missing edited scenes: %q", reloaded.Content)
	}
	if !strings.HasPrefix(reloaded.Content, "序章前言") {
		t.Fatalf("expected chapter preamble to be preserved, got %q", reloaded.Content)
	}
}

func TestGetSettingsReturnsRetrievalDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newE2ETestServer(t, dir, "http://127.0.0.1:0")
	app := httptest.NewServer(s.router)
	defer app.Close()

	resp := performRequest(t, app.URL, "GET", "/api/settings", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get settings failed: %s", string(resp.Body))
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	if got := payload["retrieval_top_k"]; got != float64(4) {
		t.Fatalf("expected retrieval_top_k=4, got %#v", got)
	}
	sources, ok := payload["retrieval_sources"].([]any)
	if !ok || len(sources) != 4 {
		t.Fatalf("expected retrieval_sources in response, got %#v", payload["retrieval_sources"])
	}
	presets, ok := payload["presets"].(map[string]any)
	if !ok {
		t.Fatalf("expected presets in response, got %#v", payload["presets"])
	}
	dialogue, ok := presets["dialogue"].(map[string]any)
	if !ok {
		t.Fatalf("expected dialogue preset, got %#v", presets["dialogue"])
	}
	if got := dialogue["top_k"]; got != float64(3) {
		t.Fatalf("expected dialogue top_k=3, got %#v", got)
	}
}

func TestE2EStyleAnalyzeApplyAndRewritePreset(t *testing.T) {
	t.Parallel()

	var prompts []string
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float64{0.1, 0.2, 0.3}})
		case "/api/generate":
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode generate request: %v", err)
			}
			prompt, _ := req["prompt"].(string)
			prompts = append(prompts, prompt)
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.Contains(prompt, "分析以下文字的寫作風格"):
				_, _ = w.Write([]byte("{\"response\":\"{\\\"dialogue_ratio\\\":\\\"中\\\",\\\"sensory_freq\\\":\\\"高\\\",\\\"avg_sentence_len\\\":\\\"中等\\\",\\\"tone\\\":\\\"冷靜\\\",\\\"summary\\\":\\\"冷靜中帶細節\\\"}\",\"done\":true}\n"))
			default:
				_, _ = w.Write([]byte("{\"response\":\"ok\",\"done\":true}\n"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "style", "主線敘事.md"), "# 風格：主線敘事\n- 語氣：克制\n")

	s := newE2ETestServer(t, dir, ollama.URL)
	app := httptest.NewServer(s.router)
	defer app.Close()

	analyzeResp := performJSONRequest(t, app.URL, "POST", "/api/styles/analyze", map[string]any{
		"text": "他看著門，沒有說話。",
	})
	if analyzeResp.StatusCode != http.StatusOK {
		t.Fatalf("style analyze failed: %s", string(analyzeResp.Body))
	}
	if !strings.Contains(string(analyzeResp.Body), "\"dialogue_ratio\":\"中\"") {
		t.Fatalf("expected analyze response payload, got %s", string(analyzeResp.Body))
	}

	applyResp := performJSONRequest(t, app.URL, "POST", "/api/styles/%E4%B8%BB%E7%B7%9A%E6%95%98%E4%BA%8B/analysis", map[string]any{
		"dialogue_ratio":   "中",
		"sensory_freq":     "高",
		"avg_sentence_len": "中等",
		"tone":             "冷靜",
		"summary":          "冷靜中帶細節",
	})
	if applyResp.StatusCode != http.StatusOK {
		t.Fatalf("apply style analysis failed: %s", string(applyResp.Body))
	}
	data, err := os.ReadFile(filepath.Join(dir, "style", ".analysis", "主線敘事.json"))
	if err != nil {
		t.Fatalf("expected analysis sidecar to be written: %v", err)
	}
	if !strings.Contains(string(data), "冷靜中帶細節") {
		t.Fatalf("expected sidecar to contain applied summary, got %s", string(data))
	}

	rewriteResp := performJSONRequest(t, app.URL, "POST", "/rewrite/stream", map[string]any{
		"chapter":       "林昊站在夜港塔下。",
		"mode":          "conservative",
		"style_preset":  "cold_hard",
		"chapter_title": "第01章",
	})
	if rewriteResp.StatusCode != http.StatusOK || !strings.Contains(string(rewriteResp.Body), "ok") {
		t.Fatalf("rewrite stream with preset failed: %s", string(rewriteResp.Body))
	}

	foundPreset := false
	for _, prompt := range prompts {
		if strings.Contains(prompt, "【風格約束：冷硬派】") {
			foundPreset = true
			break
		}
	}
	if !foundPreset {
		t.Fatalf("expected rewrite prompt to include cold_hard preset, prompts=%v", prompts)
	}
}

func TestE2EStyleAnalyzeReturnsBadRequestOnParseFailure(t *testing.T) {
	t.Parallel()

	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"response\":\"not-json\",\"done\":true}\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	dir := t.TempDir()
	s := newE2ETestServer(t, dir, ollama.URL)
	app := httptest.NewServer(s.router)
	defer app.Close()

	resp := performJSONRequest(t, app.URL, "POST", "/api/styles/analyze", map[string]any{
		"text": "他看著門，沒有說話。",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 on parse failure, got %d body=%s", resp.StatusCode, string(resp.Body))
	}
}

func TestHandleExportManuscriptHTMLFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := newE2ETestServer(t, dir, "http://127.0.0.1:0")
	if _, err := s.saveChapterFile("第01章", "內容正文"); err != nil {
		t.Fatalf("save chapter: %v", err)
	}

	app := httptest.NewServer(s.router)
	defer app.Close()

	resp := performJSONRequest(t, app.URL, "POST", "/api/manuscript/export", map[string]any{
		"format": "html",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(resp.Body))
	}

	body := string(resp.Body)
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Fatalf("expected HTML document in response, got %s", body)
	}
	if !strings.Contains(body, "內容正文") {
		t.Fatalf("expected chapter content in HTML output, got %s", body)
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
		auth:          newAuthManager(cfg),
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
		worldstate:    worldstate.New(filepath.Join(dataDir, "worldstate.json")),
	}
	s.consistency = consistency.New(s.checker)
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

type sourceEventItem struct {
	ChapterFile  string `json:"chapter_file"`
	ChapterIndex int    `json:"chapter_index"`
	SceneIndex   int    `json:"scene_index"`
	ChunkType    string `json:"chunk_type"`
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

func decodeSSEItems(t *testing.T, body string) []sourceEventItem {
	t.Helper()

	type sourceEvent struct {
		Items []sourceEventItem `json:"items"`
	}

	blocks := strings.Split(body, "\n\n")
	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		if len(lines) < 2 || strings.TrimSpace(lines[0]) != "event:sources" {
			continue
		}
		var payloadLines []string
		for _, line := range lines[1:] {
			if strings.HasPrefix(line, "data:") {
				payloadLines = append(payloadLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if len(payloadLines) == 0 {
			continue
		}

		var payload sourceEvent
		if err := json.Unmarshal([]byte(strings.Join(payloadLines, "\n")), &payload); err != nil {
			t.Fatalf("decode SSE sources payload: %v body=%s", err, body)
		}
		return payload.Items
	}

	return nil
}

func reconstructChapterForSceneEdit(t *testing.T, fullContent string, scenes []Scene, sceneTitle, editedContent string) string {
	t.Helper()

	firstMarker := regexp.MustCompile(`(?m)^## Scene \d+`).FindStringIndex(fullContent)
	preamble := ""
	if firstMarker != nil && firstMarker[0] > 0 {
		preamble = strings.TrimRight(fullContent[:firstMarker[0]], "\r\n\t ")
	}

	parts := make([]string, 0, len(scenes)+1)
	if preamble != "" {
		parts = append(parts, preamble)
	}

	found := false
	for _, scene := range scenes {
		content := scene.Content
		if scene.Title == sceneTitle {
			content = editedContent
			found = true
		}
		parts = append(parts, "## "+scene.Title+"\n"+content)
	}
	if !found {
		t.Fatalf("scene %q not found in %v", sceneTitle, scenes)
	}

	return strings.Join(parts, "\n\n")
}

func TestBuildReferenceContextExcludesFutureChapters(t *testing.T) {
	t.Parallel()

	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float64{1, 0, 0}})
		case "/api/generate":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"response\":\"ok\",\"done\":true}\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	dir := t.TempDir()
	// Past chapter: index 2 — should be included when reviewing chapter 3
	mustWriteFile(t, filepath.Join(dir, "chapters", "第02章.md"), "# 第02章\n林昊初次見到城主。")
	// Future chapter: index 5 — must be excluded when reviewing chapter 3
	mustWriteFile(t, filepath.Join(dir, "chapters", "第05章.md"), "# 第05章\n林昊離開了城市。")
	// Character — not filtered by chapter index
	mustWriteFile(t, filepath.Join(dir, "characters", "林昊.md"), "# 角色：林昊\n- 個性：冷靜\n")

	s := newE2ETestServer(t, dir, ollama.URL)
	if err := s.Ingest(context.Background()); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	app := httptest.NewServer(s.router)
	defer app.Close()

	resp := performJSONRequest(t, app.URL, "POST", "/check/stream", map[string]any{
		"chapter":      "林昊在第三章站著。",
		"chapter_file": "第03章.md",
		"checks":       []string{"behavior"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("check stream failed: %s", string(resp.Body))
	}

	body := string(resp.Body)

	// Sources event should include chapter 第02章.md but not 第05章.md
	if strings.Contains(body, "第05章") {
		t.Fatalf("future chapter 第05章 must not appear in retrieval sources, got: %s", body)
	}
	if !strings.Contains(body, "第02章") {
		t.Fatalf("past chapter 第02章 should appear in retrieval sources, got: %s", body)
	}
}
