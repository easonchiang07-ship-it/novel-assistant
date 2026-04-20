# Multi-Layer Review Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 為章節審查新增 `pipeline` 模式，依序執行結構、角色、世界觀、語言四層獨立 review，同時保留 `single` 模式的既有行為不變。

**Architecture:** 在 `internal/server/review_layers.go` 集中管理 layer 定義、prompt 模板與 pipeline 執行流程；`handleCheckStream` 只負責分流到 `single` 與 `pipeline`。前端 `check.html` 新增模式切換與 layer SSE 呈現，但不在第一版提供部分 layer 勾選。

**Tech Stack:** Go、Gin、既有 SSE stream flow、既有 checker streaming API、HTML template + vanilla JavaScript、Go test。

---

## File Map

- Modify: `internal/server/handlers.go`
  - 擴充既有 `checkRequest` struct，保留現有欄位並新增 `layer_mode` / `layers`
  - 在 `handleCheckStream` 做 `single` / `pipeline` 分流
  - 新增 `streamEvent` 的 layer 欄位支援
- Create: `internal/server/review_layers.go`
  - 定義 `reviewLayer`
  - 實作 `defaultReviewLayers()`
  - 實作 `resolveReviewLayers(req checkRequest)`
  - 實作 `runPipelineReview(...)`
- Modify: `internal/server/handlers_test.go`
  - 補 unit tests：layer definitions、layer resolution、single mode 不走 pipeline
- Modify: `internal/server/e2e_test.go`
  - 補 regression test：`single` 模式不出現 layer events
  - 補 pipeline success ordering test
  - 補 pipeline failure path test
- Modify: `web/templates/check.html`
  - 新增審稿模式切換 UI
  - pipeline 模式隱藏 `checks`
  - SSE 處理 `layer_start` / `layer_end`
- Modify: `internal/server/templates_test.go`
  - 補 template 靜態驗證：模式切換 UI 與 layer event handler

## Task 1: 建立 Review Layer Definition 與 Resolution

**Files:**
- Create: `internal/server/review_layers.go`
- Modify: `internal/server/handlers_test.go`

- [ ] **Step 1: 先寫 review layer unit test**

在 `internal/server/handlers_test.go` 新增測試：

```go
func TestDefaultReviewLayersReturnsFourLayersInOrder(t *testing.T) {
	t.Parallel()

	layers := defaultReviewLayers()
	if len(layers) != 4 {
		t.Fatalf("expected 4 layers, got %d", len(layers))
	}

	want := []struct {
		name  string
		label string
	}{
		{name: "structure", label: "結構層"},
		{name: "character", label: "角色層"},
		{name: "world_logic", label: "世界觀層"},
		{name: "language", label: "語言層"},
	}

	for i, layer := range layers {
		if layer.Name != want[i].name || layer.Label != want[i].label || !layer.Enabled {
			t.Fatalf("unexpected layer at %d: %#v", i, layer)
		}
		if strings.TrimSpace(layer.Prompt) == "" {
			t.Fatalf("expected prompt for layer %s", layer.Name)
		}
	}
}

func TestResolveReviewLayersPipelineReturnsAllEnabled(t *testing.T) {
	t.Parallel()

	req := checkRequest{
		Chapter:   "章節內容",
		LayerMode: "pipeline",
	}

	layers := resolveReviewLayers(req)
	if len(layers) != 4 {
		t.Fatalf("expected 4 layers in pipeline mode, got %#v", layers)
	}
}

func TestResolveReviewLayersSingleReturnsNil(t *testing.T) {
	t.Parallel()

	req := checkRequest{
		Chapter:   "章節內容",
		LayerMode: "single",
	}

	layers := resolveReviewLayers(req)
	if layers != nil {
		t.Fatalf("expected nil layers in single mode, got %#v", layers)
	}
}
```

- [ ] **Step 2: 跑測試確認先紅燈**

Run:

```bash
go test ./internal/server -run "TestDefaultReviewLayersReturnsFourLayersInOrder|TestResolveReviewLayersPipelineReturnsAllEnabled|TestResolveReviewLayersSingleReturnsNil" -count=1
```

Expected:

- build fail，因為 `defaultReviewLayers` / `resolveReviewLayers` 尚未存在

- [ ] **Step 3: 新增 `review_layers.go` 最小實作**

在 `internal/server/review_layers.go` 建立：

```go
package server

type reviewLayer struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Prompt  string `json:"prompt"`
	Enabled bool   `json:"enabled"`
}

func defaultReviewLayers() []reviewLayer {
	return []reviewLayer{
		{
			Name:    "structure",
			Label:   "結構層",
			Prompt:  "你是專業小說結構編輯。只分析以下章節的「敘事節奏、開場鉤子、張力起伏、段落長短」。不要評論角色或語言風格。條列式給出具體問題與改善建議。",
			Enabled: true,
		},
		{
			Name:    "character",
			Label:   "角色層",
			Prompt:  "你是角色塑造專家。只分析以下章節的「角色行為是否符合其人設、對白語氣是否一致」。結合提供的角色資料做判斷。不要評論結構或語言。",
			Enabled: true,
		},
		{
			Name:    "world_logic",
			Label:   "世界觀層",
			Prompt:  "你是世界觀邏輯審查員。只分析以下章節的「設定自洽性、時間線合理性、道具與地點邏輯」。若有提供追蹤器資料，優先以此判斷。不要評論其他層面。",
			Enabled: true,
		},
		{
			Name:    "language",
			Label:   "語言層",
			Prompt:  "你是文字風格編輯。只分析以下章節的「句式多樣性、重複用語、感官描寫密度、語言流暢度」。不要評論劇情或角色。",
			Enabled: true,
		},
	}
}

func resolveReviewLayers(req checkRequest) []reviewLayer {
	if req.LayerMode != "pipeline" {
		return nil
	}
	layers := defaultReviewLayers()
	out := make([]reviewLayer, 0, len(layers))
	for _, layer := range layers {
		if layer.Enabled {
			out = append(out, layer)
		}
	}
	return out
}
```

- [ ] **Step 4: 重新跑 unit test 確認變綠**

Run:

```bash
go test ./internal/server -run "TestDefaultReviewLayersReturnsFourLayersInOrder|TestResolveReviewLayersPipelineReturnsAllEnabled|TestResolveReviewLayersSingleReturnsNil" -count=1
```

Expected:

- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/review_layers.go internal/server/handlers_test.go
git commit -m "feat: define review pipeline layers"
```

## Task 2: 擴充 Request/SSE 結構並保住 Single Mode 回歸邊界

**Files:**
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/handlers_test.go`

- [ ] **Step 1: 先寫 single mode regression 測試**

在 `internal/server/handlers_test.go` 新增：

```go
func TestResolveReviewLayersTreatsEmptyModeAsSingle(t *testing.T) {
	t.Parallel()

	req := checkRequest{
		Chapter: "章節內容",
	}

	if layers := resolveReviewLayers(req); layers != nil {
		t.Fatalf("expected empty mode to behave like single, got %#v", layers)
	}
}
```

- [ ] **Step 2: 跑測試確認需求被釘住**

Run:

```bash
go test ./internal/server -run "TestResolveReviewLayersTreatsEmptyModeAsSingle" -count=1
```

Expected:

- PASS 或 FAIL，視 Step 3 是否需要調整；若已 PASS，保留該測試作 regression 錨點

- [ ] **Step 3: 擴充既有 `checkRequest` 與 `streamEvent`**

在 `internal/server/handlers.go` 更新既有型別，保留現有欄位，只新增欄位：

```go
type streamEvent struct {
	Event     string
	Text      string
	Layer     string
	Label     string
	Sources   []referenceSummary
	Retrieval any
	Gaps      *retrievalGaps
	Conflicts []consistency.Conflict
}

type checkRequest struct {
	Chapter            string                      `json:"chapter"`
	Checks             []string                    `json:"checks"`
	Characters         []string                    `json:"characters"`
	Styles             []string                    `json:"styles"`
	Retrieval          retrievalOptions            `json:"retrieval"`
	RetrievalOverrides map[string]retrievalOptions `json:"retrieval_overrides"`
	ChapterFile        string                      `json:"chapter_file"`
	ChapterTitle       string                      `json:"chapter_title"`
	SceneTitle         string                      `json:"scene_title,omitempty"`
	Scene              string                      `json:"scene,omitempty"`
	LayerMode          string                      `json:"layer_mode"`
	Layers             []string                    `json:"layers,omitempty"`
}
```

同檔案中補一個 helper：

```go
func normalizedLayerMode(raw string) string {
	mode := strings.TrimSpace(raw)
	if mode == "" {
		return "single"
	}
	return mode
}
```

- [ ] **Step 4: 確保 SSE writer 支援 layer event**

在 `handleCheckStream` 的 SSE stream switch 加入：

```go
if msg.Event == "layer_start" {
	c.SSEvent("layer_start", gin.H{"layer": msg.Layer, "label": msg.Label})
	return true
}
if msg.Event == "layer_end" {
	c.SSEvent("layer_end", gin.H{"layer": msg.Layer})
	return true
}
```

- [ ] **Step 5: 跑 handlers 測試**

Run:

```bash
go test ./internal/server -run "TestResolveReviewLayersTreatsEmptyModeAsSingle|TestDefaultReviewLayersReturnsFourLayersInOrder|TestResolveReviewLayersPipelineReturnsAllEnabled|TestResolveReviewLayersSingleReturnsNil" -count=1
```

Expected:

- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/handlers.go internal/server/handlers_test.go
git commit -m "feat: add layer mode request and sse events"
```

## Task 3: 實作 Pipeline Runner 與後端分流

**Files:**
- Modify: `internal/server/review_layers.go`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/e2e_test.go`

- [ ] **Step 1: 先寫 pipeline 成功路徑 e2e 測試**

在 `internal/server/e2e_test.go` 新增：

```go
func TestCheckStreamPipelineEmitsOrderedLayerEvents(t *testing.T) {
	t.Parallel()

	callCount := 0
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float64{0.1, 0.2, 0.3}})
		case "/api/generate":
			callCount++
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
	mustWriteFile(t, filepath.Join(dir, "style", "主線敘事.md"), "# 風格：主線敘事\n- 語氣：克制\n")

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
	if callCount != 4 {
		t.Fatalf("expected 4 generate calls, got %d", callCount)
	}
}
```

- [ ] **Step 2: 寫 single regression e2e 測試**

在同檔追加：

```go
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
```

- [ ] **Step 3: 跑測試確認先失敗**

Run:

```bash
go test ./internal/server -run "TestCheckStreamPipelineEmitsOrderedLayerEvents|TestCheckStreamSingleDoesNotEmitLayerEvents" -count=1
```

Expected:

- `pipeline` 測試 FAIL，因為目前尚未實作 layer event 與 pipeline path

- [ ] **Step 4: 在 `review_layers.go` 實作 pipeline runner**

新增 helper：

```go
func (s *Server) runPipelineReview(
	ctx context.Context,
	req checkRequest,
	msgChan chan<- streamEvent,
	transcript *strings.Builder,
	worldStatePrefix string,
) error {
	layers := resolveReviewLayers(req)
	if len(layers) == 0 {
		return nil
	}

	charsToCheck := s.resolveCharacters(req)
	behaviorOpts := mergeRetrieval(s.rules.PresetFor("behavior"), req.retrievalOverrideFor("behavior"))
	dialogueOpts := mergeRetrieval(s.rules.PresetFor("dialogue"), req.retrievalOverrideFor("dialogue"))
	worldOpts := mergeRetrieval(s.rules.PresetFor("world"), req.retrievalOverrideFor("world"))

	behaviorRefs, err := s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, behaviorOpts)
	if err != nil {
		return err
	}
	dialogueRefs, err := s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, dialogueOpts)
	if err != nil {
		return err
	}
	worldRefs, err := s.buildReferenceContext(ctx, req.Chapter, req.ChapterFile, worldOpts)
	if err != nil {
		return err
	}

	allRefs := mergeReferenceLists(behaviorRefs, dialogueRefs, worldRefs)
	if len(allRefs) > 0 {
		msgChan <- streamEvent{Event: "sources", Sources: summarizeReferences(allRefs)}
	}

	for _, layer := range layers {
		msgChan <- streamEvent{Event: "layer_start", Layer: layer.Name, Label: layer.Label}

		cw := &chanWriter{ch: msgChan, transcript: transcript}
		var layerErr error
		switch layer.Name {
		case "structure":
			prompt := layer.Prompt + "\n\n【章節內容】\n" + req.Chapter
			layerErr = s.checker.RewriteChapterWithSystemStream(ctx, worldStatePrefix, prompt, cw)
		case "character":
			var profiles []string
			for _, ch := range charsToCheck {
				profiles = append(profiles, ch.RawContent)
			}
			prompt := layer.Prompt + "\n\n【角色資料】\n" + strings.Join(profiles, "\n\n") + "\n\n【補充參考資料】\n" + joinProfiles(mergeReferenceLists(behaviorRefs, dialogueRefs)) + "\n\n【章節內容】\n" + req.Chapter
			layerErr = s.checker.RewriteChapterWithSystemStream(ctx, worldStatePrefix, prompt, cw)
		case "world_logic":
			prompt := layer.Prompt + "\n\n【補充參考資料】\n" + joinProfiles(worldRefs) + "\n\n【章節內容】\n" + req.Chapter
			layerErr = s.checker.RewriteChapterWithSystemStream(ctx, worldStatePrefix, prompt, cw)
		case "language":
			prompt := layer.Prompt + "\n\n【補充參考資料】\n" + joinProfiles(filterReferencesByType(allRefs, "style")) + "\n\n【章節內容】\n" + req.Chapter
			layerErr = s.checker.RewriteChapterWithSystemStream(ctx, worldStatePrefix, prompt, cw)
		}

		if layerErr != nil {
			if ctx.Err() == nil {
				text := fmt.Sprintf("\n> 錯誤：%s\n", layerErr.Error())
				transcript.WriteString(text)
				msgChan <- streamEvent{Event: "chunk", Text: text}
			}
			return layerErr
		}

		msgChan <- streamEvent{Event: "layer_end", Layer: layer.Name}
		msgChan <- streamEvent{Event: "chunk", Text: "\n"}
		transcript.WriteString("\n")
	}

	return nil
}
```

注意：

- 第一版沿用既有 checker stream API，避免為這張票再抽新 checker abstraction
- `checkRequest` 是擴充既有 struct，不可刪掉現有欄位
- `structure` 層這裡先沿用通用 streaming 入口來發 token；若要避免「rewrite」語義命名不精準，可在後續小重構處理，不在本票擴 scope

- [ ] **Step 5: 在 `handleCheckStream` 增加分流**

在 goroutine 中，於既有 single flow 前加入：

```go
	mode := normalizedLayerMode(req.LayerMode)
	if mode == "pipeline" {
		worldStatePrefix := s.worldStateSystemPrefix(req.ChapterFile)
		if err := s.runPipelineReview(ctx, req, msgChan, &transcript, worldStatePrefix); err != nil {
			return
		}

		completion := "\n\n---\n審查完成\n"
		msgChan <- streamEvent{Event: "chunk", Text: completion}
		transcript.WriteString(completion)
		return
	}
```

保留現有 single mode 流程原樣，不挪動既有行為。

- [ ] **Step 6: 跑 pipeline/single e2e 測試**

Run:

```bash
go test ./internal/server -run "TestCheckStreamPipelineEmitsOrderedLayerEvents|TestCheckStreamSingleDoesNotEmitLayerEvents" -count=1
```

Expected:

- PASS

- [ ] **Step 7: Commit**

```bash
git add internal/server/review_layers.go internal/server/handlers.go internal/server/e2e_test.go
git commit -m "feat: add pipeline review streaming"
```

## Task 4: 補齊 Pipeline Failure Path

**Files:**
- Modify: `internal/server/e2e_test.go`
- Modify: `internal/server/review_layers.go`（僅在必要時）

- [ ] **Step 1: 先寫 failure path e2e 測試**

在 `internal/server/e2e_test.go` 新增：

```go
func TestCheckStreamPipelineStopsAfterLayerFailure(t *testing.T) {
	t.Parallel()

	callCount := 0
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			callCount++
			w.Header().Set("Content-Type", "application/json")
			if callCount == 2 {
				http.Error(w, "boom", http.StatusInternalServerError)
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

	s := newE2ETestServer(t, dir, ollama.URL)
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
	if !strings.Contains(body, "event:layer_start") || !strings.Contains(body, "\"layer\":\"character\"") {
		t.Fatalf("expected pipeline to reach failing layer, got %s", body)
	}
	if strings.Contains(body, "event:layer_end\ndata:{\"layer\":\"character\"}") {
		t.Fatalf("failing layer must not emit layer_end, got %s", body)
	}
	if strings.Contains(body, "\"layer\":\"world_logic\"") || strings.Contains(body, "\"layer\":\"language\"") {
		t.Fatalf("pipeline should stop after failing layer, got %s", body)
	}
	if !strings.Contains(body, "> 錯誤：") {
		t.Fatalf("expected streamed error chunk, got %s", body)
	}
}
```

- [ ] **Step 2: 跑測試確認先紅燈**

Run:

```bash
go test ./internal/server -run "TestCheckStreamPipelineStopsAfterLayerFailure" -count=1
```

Expected:

- FAIL，直到 failure path 語義符合 spec

- [ ] **Step 3: 修正 pipeline runner 直到失敗即中止**

確認 `runPipelineReview(...)` 遵守：

- 某層失敗時回傳 error
- 失敗層不送 `layer_end`
- 後續層不執行
- 仍透過 `chunk` 送出錯誤訊息

若 `Step 1` 測試已綠，這步只需保留程式碼不再更動。

- [ ] **Step 4: 跑 failure path 測試**

Run:

```bash
go test ./internal/server -run "TestCheckStreamPipelineStopsAfterLayerFailure" -count=1
```

Expected:

- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/e2e_test.go internal/server/review_layers.go
git commit -m "test: cover pipeline failure path"
```

## Task 5: 前端加上模式切換與 Layer 顯示

**Files:**
- Modify: `web/templates/check.html`
- Modify: `internal/server/templates_test.go`

- [ ] **Step 1: 先寫 template 靜態驗證**

在 `internal/server/templates_test.go` 新增：

```go
func TestCheckTemplateSupportsPipelineReviewMode(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../web/templates/check.html")
	if err != nil {
		t.Fatalf("read check template: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "id=\"review-mode\"") {
		t.Fatal("expected review mode selector in check template")
	}
	if !strings.Contains(text, "layer_start") || !strings.Contains(text, "layer_end") {
		t.Fatal("expected layer event handlers in check template")
	}
	if !strings.Contains(text, "將固定依序執行：結構層、角色層、世界觀層、語言層") {
		t.Fatal("expected pipeline helper text in check template")
	}
}
```

- [ ] **Step 2: 跑 template 測試確認先失敗**

Run:

```bash
go test ./internal/server -run "TestCheckTemplateSupportsPipelineReviewMode" -count=1
```

Expected:

- FAIL，因為前端尚未有 review mode UI 與 layer handlers

- [ ] **Step 3: 在 `check.html` 新增模式切換與 UI 顯示邏輯**

新增一段表單：

```html
<div class="form-group">
  <label for="review-mode">審稿模式</label>
  <select id="review-mode" onchange="syncReviewModeUI()">
    <option value="single">單層</option>
    <option value="pipeline">多層 Pipeline</option>
  </select>
  <div id="pipeline-mode-hint" class="helper-text" style="display:none;margin-top:8px">
    將固定依序執行：結構層、角色層、世界觀層、語言層
  </div>
</div>
```

新增 JS helper：

```js
function syncReviewModeUI() {
  const mode = document.getElementById('review-mode').value;
  const checksGroup = document.querySelector('.form-group:has([name="checks"])');
  const hint = document.getElementById('pipeline-mode-hint');
  if (checksGroup) {
    checksGroup.style.display = mode === 'pipeline' ? 'none' : 'block';
  }
  if (hint) {
    hint.style.display = mode === 'pipeline' ? 'block' : 'none';
  }
}
```

並在 `DOMContentLoaded` 末尾呼叫一次：

```js
syncReviewModeUI();
```

注意：若 `:has()` 相容性是顧慮，改成直接給 checks 容器一個固定 `id="checks-group"`，由 JS 直接抓 ID；優先用固定 ID，避免 selector 相容性問題。

- [ ] **Step 4: 在 SSE parser 新增 layer event 顯示**

在 `runCheck()` 的 stream loop 與 buffer flush 分支中加入：

```js
function appendLayerHeading(label) {
  const resultBox = document.getElementById('result-box');
  if (resultBox.textContent && !resultBox.textContent.endsWith('\n')) {
    resultBox.textContent += '\n';
  }
  resultBox.textContent += '── ' + label + ' ──\n';
  resultBox.scrollTop = resultBox.scrollHeight;
}
```

處理事件：

```js
if (parsed.eventName === 'layer_start') {
  appendLayerHeading(parsed.payload.label || parsed.payload.layer || 'Layer');
} else if (parsed.eventName === 'layer_end') {
  appendResult('\n');
}
```

送 request 時加入：

```js
layer_mode: document.getElementById('review-mode').value,
```

- [ ] **Step 5: 跑 template test**

Run:

```bash
go test ./internal/server -run "TestCheckTemplateSupportsPipelineReviewMode" -count=1
```

Expected:

- PASS

- [ ] **Step 6: Commit**

```bash
git add web/templates/check.html internal/server/templates_test.go
git commit -m "feat: add pipeline review mode ui"
```

## Task 6: 全面驗證與整理

**Files:**
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/review_layers.go`
- Modify: `internal/server/handlers_test.go`
- Modify: `internal/server/e2e_test.go`
- Modify: `internal/server/templates_test.go`
- Modify: `web/templates/check.html`

- [ ] **Step 1: 格式化 Go 檔案**

Run:

```bash
gofmt -w internal/server/handlers.go internal/server/review_layers.go internal/server/handlers_test.go internal/server/e2e_test.go internal/server/templates_test.go
```

Expected:

- command succeeds with no error output

- [ ] **Step 2: 跑 server focused tests**

Run:

```bash
go test ./internal/server -count=1
```

Expected:

- PASS

- [ ] **Step 3: 跑全測試**

Run:

```bash
go test ./... -count=1
```

Expected:

- PASS

- [ ] **Step 4: 確認 gofmt 無漏**

Run:

```bash
gofmt -l internal/server/handlers.go internal/server/review_layers.go internal/server/handlers_test.go internal/server/e2e_test.go internal/server/templates_test.go
```

Expected:

- no output

- [ ] **Step 5: 檢查變更範圍**

Run:

```bash
git status --short
```

Expected:

- 只包含本計畫列出的檔案

- [ ] **Step 6: Commit**

```bash
git add internal/server/handlers.go internal/server/review_layers.go internal/server/handlers_test.go internal/server/e2e_test.go internal/server/templates_test.go web/templates/check.html
git commit -m "feat: add multi-layer review pipeline"
```
