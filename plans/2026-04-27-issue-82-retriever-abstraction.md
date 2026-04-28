# Issue #82 實作計畫：Retriever Abstraction Interface

> 狀態：已完成
> Issue：[#82 Retriever abstraction interface — decouple handlers from vectorstore.Store](https://github.com/easonchiang07-ship-it/novel-assistant/issues/82)
> 注意：Issue 標記為 Phase 3 地基，Phase 0 / Phase 2 完成前不需要實作。

## 架構決策

- 新建 `internal/retriever/` package，定義 `Retriever` interface 與 `VectorRetriever` 實作
- `VectorRetriever` 在 `Retrieve()` 內部負責 embed + query，handler 只傳字串 query
- `Retriever` 內定義 `Embedder` interface（`Embed(ctx, text) ([]float64, error)`），讓 VectorRetriever 可以在不啟動 Ollama 的情況下單元測試
- `buildReferenceContext` 的 `s.store.QueryFilteredBeforeChapter` 替換為 `s.retriever.Retrieve`
- `s.store.Len() == 0` 的早退邏輯移入 `VectorRetriever.Retrieve`，handler 不再直接存取 store
- `server.go` 在 `setProjectState` 與 `applyProjectSettings` 兩個位置同步更新 `s.retriever`
- `settings.go` 的 `handleSaveSettings` 在呼叫 `applyProjectSettings()` 後會再次重新指派 `s.embedder`，因此必須在最後一次 `s.embedder = ...` 之後再重建 `s.retriever`，否則 retriever 持有的是已被替換的舊 embedder

## 呼叫路徑（重構前 vs 後）

```
重構前：
buildReferenceContext
  → s.embedder.Embed(ctx, chapter)     [*embedder.OllamaEmbedder]
  → s.store.QueryFilteredBeforeChapter [*vectorstore.Store]

重構後：
buildReferenceContext
  → s.retriever.Retrieve(ctx, Request{Query: chapter, ...})
      → Embedder.Embed(ctx, query)
      → store.QueryFilteredBeforeChapter(vec, ...)
```

## 檔案位置

| 動作 | 路徑 |
|---|---|
| 新增 | `internal/retriever/retriever.go` |
| 新增 | `internal/retriever/vector.go` |
| 新增 | `internal/retriever/vector_test.go` |
| 修改 | `internal/server/server.go` |
| 修改 | `internal/server/handlers.go` |
| 修改 | `internal/server/handlers_test.go` |
| 修改 | `internal/server/settings.go` |

## 待實作 Checklist

- [ ] **Task 1** `internal/retriever/retriever.go`：介面定義
- [ ] **Task 2** `internal/retriever/vector.go`：VectorRetriever 實作
- [ ] **Task 3** `internal/retriever/vector_test.go`：單元測試
- [ ] **Task 4** `internal/server/server.go`：Server struct + 初始化 + 同步更新
- [ ] **Task 4-F** `internal/server/handlers_test.go`：修正 `TestBuildReferenceContextReturnsNilWhenStoreIsEmpty`
- [ ] **Task 4-G** `internal/server/settings.go`：`handleSaveSettings` 末尾同步更新 retriever
- [ ] **Task 5** `internal/server/handlers.go`：替換 buildReferenceContext
- [ ] **Task 6** `go build ./...` + `go test ./...`

---

## Task 1：`internal/retriever/retriever.go`

```go
package retriever

import (
	"context"
	"novel-assistant/internal/vectorstore"
)

// Embedder 是 embed 呼叫的抽象，讓 VectorRetriever 可以在測試中被替換。
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

// Request 封裝一次檢索所需的所有參數。
type Request struct {
	Query         string
	Types         []string
	TopK          int
	Threshold     float64
	BeforeChapter int
}

// Chunk 是一次檢索命中的文件，帶有相似度分數。
type Chunk struct {
	vectorstore.Document
	Score float64
}

// Retriever 是向量檢索的抽象介面。
type Retriever interface {
	Retrieve(ctx context.Context, req Request) ([]Chunk, error)
}
```

---

## Task 2：`internal/retriever/vector.go`

```go
package retriever

import (
	"context"
	"novel-assistant/internal/vectorstore"
)

// VectorStorer 是 VectorRetriever 所需的 store 方法子集。
type VectorStorer interface {
	Len() int
	QueryFilteredBeforeChapter(queryVec []float64, topK int, types []string, threshold float64, beforeChapter int) []vectorstore.ScoredDocument
}

// VectorRetriever 將現有 QueryFilteredBeforeChapter 邏輯包裝為 Retriever 介面。
type VectorRetriever struct {
	embedder Embedder
	store    VectorStorer
}

// NewVector 建立 VectorRetriever。emb 與 store 不得為 nil。
func NewVector(emb Embedder, store VectorStorer) *VectorRetriever {
	return &VectorRetriever{embedder: emb, store: store}
}

// Retrieve 行為等價於原 buildReferenceContext 中的 embed + QueryFilteredBeforeChapter。
// store 為空時直接回傳 nil, nil（與原邏輯一致）。
func (r *VectorRetriever) Retrieve(ctx context.Context, req Request) ([]Chunk, error) {
	if r.store.Len() == 0 {
		return nil, nil
	}
	vec, err := r.embedder.Embed(ctx, req.Query)
	if err != nil {
		return nil, err
	}
	docs := r.store.QueryFilteredBeforeChapter(vec, req.TopK, req.Types, req.Threshold, req.BeforeChapter)
	out := make([]Chunk, len(docs))
	for i, d := range docs {
		out[i] = Chunk{Document: d.Document, Score: d.Score}
	}
	return out, nil
}
```

---

## Task 3：`internal/retriever/vector_test.go`

```go
package retriever_test

import (
	"context"
	"testing"

	"novel-assistant/internal/retriever"
	"novel-assistant/internal/vectorstore"
)

// stubEmbedder 直接回傳固定向量。
type stubEmbedder struct{ vec []float64 }

func (s *stubEmbedder) Embed(_ context.Context, _ string) ([]float64, error) {
	return s.vec, nil
}

// stubStore 僅回傳固定文件，不記錄參數（用於基本行為測試）。
type stubStore struct {
	docs []vectorstore.ScoredDocument
}

func (s *stubStore) Len() int { return len(s.docs) }
func (s *stubStore) QueryFilteredBeforeChapter(_ []float64, topK int, _ []string, _ float64, _ int) []vectorstore.ScoredDocument {
	if topK < len(s.docs) {
		return s.docs[:topK]
	}
	return s.docs
}

// recordingStore 記錄 QueryFilteredBeforeChapter 收到的所有參數，用於驗證轉送正確性。
type recordingStore struct {
	docs         []vectorstore.ScoredDocument
	gotTopK      int
	gotTypes     []string
	gotThreshold float64
	gotBefore    int
}

func (s *recordingStore) Len() int { return len(s.docs) }
func (s *recordingStore) QueryFilteredBeforeChapter(_ []float64, topK int, types []string, threshold float64, beforeChapter int) []vectorstore.ScoredDocument {
	s.gotTopK = topK
	s.gotTypes = types
	s.gotThreshold = threshold
	s.gotBefore = beforeChapter
	if topK < len(s.docs) {
		return s.docs[:topK]
	}
	return s.docs
}

func TestVectorRetrieverEmptyStore(t *testing.T) {
	r := retriever.NewVector(&stubEmbedder{vec: []float64{1}}, &stubStore{})
	chunks, err := r.Retrieve(context.Background(), retriever.Request{Query: "test", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty store, got %d", len(chunks))
	}
}

func TestVectorRetrieverReturnsChunks(t *testing.T) {
	doc := vectorstore.ScoredDocument{
		Document: vectorstore.Document{ID: "char_林昊", Type: "character", Content: "主角"},
		Score:    0.9,
	}
	store := &stubStore{docs: []vectorstore.ScoredDocument{doc}}
	r := retriever.NewVector(&stubEmbedder{vec: []float64{1}}, store)

	chunks, err := r.Retrieve(context.Background(), retriever.Request{Query: "林昊", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].ID != "char_林昊" {
		t.Errorf("expected ID=char_林昊, got %s", chunks[0].ID)
	}
	if chunks[0].Score != 0.9 {
		t.Errorf("expected Score=0.9, got %f", chunks[0].Score)
	}
}

// TestVectorRetrieverPassesRequestParams 驗證 Types / Threshold / BeforeChapter / TopK
// 全部被原樣轉送給 QueryFilteredBeforeChapter，這是「行為等價」的核心保證。
func TestVectorRetrieverPassesRequestParams(t *testing.T) {
	doc := vectorstore.ScoredDocument{
		Document: vectorstore.Document{ID: "char_X", Type: "character"},
		Score:    0.5,
	}
	store := &recordingStore{docs: []vectorstore.ScoredDocument{doc, doc, doc, doc, doc}}
	r := retriever.NewVector(&stubEmbedder{vec: []float64{1}}, store)

	req := retriever.Request{
		Query:         "test",
		Types:         []string{"character", "world"},
		TopK:          2,
		Threshold:     0.3,
		BeforeChapter: 4,
	}
	chunks, err := r.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks (topK=2), got %d", len(chunks))
	}
	if store.gotTopK != 2 {
		t.Errorf("topK not forwarded: got %d, want 2", store.gotTopK)
	}
	if len(store.gotTypes) != 2 || store.gotTypes[0] != "character" || store.gotTypes[1] != "world" {
		t.Errorf("types not forwarded: got %v", store.gotTypes)
	}
	if store.gotThreshold != 0.3 {
		t.Errorf("threshold not forwarded: got %f, want 0.3", store.gotThreshold)
	}
	if store.gotBefore != 4 {
		t.Errorf("beforeChapter not forwarded: got %d, want 4", store.gotBefore)
	}
}
```

---

## Task 4：`internal/server/server.go`

### 4-A Server struct 加欄位

在 `Server` struct 的 `consistency` 之後加入：

```go
retriever      retriever.Retriever
```

import 同時加入：

```go
"novel-assistant/internal/retriever"
```

### 4-B New() 初始化

在 `s.consistency = consistency.New(s.checker)` 之後（`setProjectState` 之前）加入：

```go
// retriever 在 setProjectState 中會根據最新 store 建立，此處先用零值初始化。
// New() 最後呼叫 s.setProjectState(st) 時會正確設定。
```

### 4-C setProjectState 同步更新

在 `s.worldstate = st.worldstate` 之後加入：

```go
s.retriever = retriever.NewVector(s.embedder, s.store)
```

完整段落如下：

```go
func (s *Server) setProjectState(st *projectState) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.state = st
	s.profiles = st.profiles
	s.store = st.store
	s.project = st.project
	s.rules = st.rules
	s.history = st.history
	s.relationships = st.relationships
	s.timeline = st.timeline
	s.foreshadow = st.foreshadow
	s.worldstate = st.worldstate
	s.cfg.DataDir = st.dataDir
	s.retriever = retriever.NewVector(s.embedder, s.store) // ← 新增
}
```

### 4-D applyProjectSettings 同步更新

在 `s.embedder = embedder.New(...)` 之後加入：

```go
if st := s.currentState(); st != nil {
    s.retriever = retriever.NewVector(s.embedder, st.store)
}
```

完整段落如下：

```go
s.embedder = embedder.New(s.cfg.OllamaURL, s.cfg.EmbedModel)
s.checker = checker.New(s.cfg.OllamaURL, s.cfg.LLMModel)
s.consistency = consistency.New(s.checker)
if st := s.currentState(); st != nil {         // ← 新增
    s.retriever = retriever.NewVector(s.embedder, st.store) // ← 新增
}                                               // ← 新增
```

### 4-E newE2ETestServer 測試 helper（`internal/server/e2e_test.go`）

在 `s.consistency = consistency.New(s.checker)` 之後加入：

```go
s.retriever = retriever.NewVector(s.embedder, s.store)
```

---

## Task 4-F：`internal/server/handlers_test.go`

`buildReferenceContext` 重構後不再呼叫 `s.store.Len()`，改呼叫 `s.rules.Get()` 和 `s.retriever.Retrieve()`。原本的 `TestBuildReferenceContextReturnsNilWhenStoreIsEmpty` 只初始化了 `s.store`，重構後會 panic（nil rules / nil retriever）。

**重寫此測試**，改用完整初始化的 retriever 和 rules：

```go
func TestBuildReferenceContextReturnsNilWhenStoreIsEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// reviewrules.New 未 Load 時 Get() 回傳零值預設，不會 panic
	rules := reviewrules.New(filepath.Join(dir, "rules.json"))
	// vectorstore.New 未 Load 時 Len() == 0，Retrieve 會直接回傳 nil,nil
	store := vectorstore.New(filepath.Join(dir, "store.json"))
	// embedder URL 不可達，但 store 為空所以不會被呼叫
	emb := embedder.New("http://127.0.0.1:1", "mock")

	s := &Server{
		rules:     rules,
		retriever: retriever.NewVector(emb, store),
	}

	refs, err := s.buildReferenceContext(context.Background(), "chapter", "", retrievalOptions{})
	if err != nil {
		t.Fatalf("unexpected error with empty store: %v", err)
	}
	if refs != nil {
		t.Fatalf("expected nil refs for empty store, got %#v", refs)
	}
}
```

需在 `handlers_test.go` import 加入：
```
"novel-assistant/internal/embedder"
"novel-assistant/internal/retriever"
"novel-assistant/internal/vectorstore"
```
（若已存在則略過。）

---

## Task 4-G：`internal/server/settings.go`

`handleSaveSettings` 在 `applyProjectSettings()` 之後直接重新指派 `s.embedder` 和 `s.checker`：

```go
s.applyProjectSettings()
s.embedder = embedder.New(s.cfg.OllamaURL, s.cfg.EmbedModel)  // ← 替換了 applyProjectSettings 裡建立的 embedder
s.checker = checker.New(s.cfg.OllamaURL, s.cfg.LLMModel)
```

若照計畫在 `applyProjectSettings` 裡重建 retriever，此時 retriever 已綁定舊 embedder，不變式被破壞。

在 `s.checker = checker.New(...)` 之後加入：

```go
if st := s.currentState(); st != nil {
    s.retriever = retriever.NewVector(s.embedder, st.store)
}
```

完整段落（settings.go handleSaveSettings 末尾）：

```go
s.applyProjectSettings()
s.embedder = embedder.New(s.cfg.OllamaURL, s.cfg.EmbedModel)
s.checker = checker.New(s.cfg.OllamaURL, s.cfg.LLMModel)
if st := s.currentState(); st != nil {                          // ← 新增
    s.retriever = retriever.NewVector(s.embedder, st.store)     // ← 新增
}                                                               // ← 新增
```

---

## Task 5：`internal/server/handlers.go`

### 替換 buildReferenceContext

**移除：**
- `s.store.Len() == 0` 早退（已移入 VectorRetriever）
- `s.embedder.Embed(ctx, chapter)` 呼叫
- `s.store.QueryFilteredBeforeChapter(...)` 呼叫

**替換成：**

```go
func (s *Server) buildReferenceContext(ctx context.Context, chapter, chapterFile string, opts retrievalOptions) ([]vectorProfile, error) {
	rules := s.rules.Get()
	topK := opts.TopK
	if topK < 1 {
		topK = rules.RetrievalTopK
	}
	sources := opts.Sources
	if len(sources) == 0 {
		sources = rules.RetrievalSources
	}
	threshold := opts.Threshold
	if threshold < 0 || threshold > 1 {
		threshold = rules.RetrievalThreshold
	}

	beforeChapter := resolveBeforeChapter(chapterFile, opts)
	chunks, err := s.retriever.Retrieve(ctx, retriever.Request{
		Query:         chapter,
		Types:         sources,
		TopK:          topK,
		Threshold:     threshold,
		BeforeChapter: beforeChapter,
	})
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, nil
	}

	results := make([]vectorProfile, 0, len(chunks))
	for _, c := range chunks {
		if c.Type == "chapter" && strings.TrimSpace(chapterFile) != "" && c.ChapterFile == chapterFile {
			continue
		}
		name := strings.TrimPrefix(c.ID, "char_")
		name = strings.TrimPrefix(name, "world_")
		name = strings.TrimPrefix(name, "style_")
		name = strings.TrimPrefix(name, "chapter_")
		reason, snippet := referenceMatchDetail(chapter, name, c.Content)
		results = append(results, vectorProfile{
			Name:         name,
			Type:         c.Type,
			Content:      c.Content,
			Score:        c.Score,
			MatchReason:  reason,
			ChapterMatch: snippet,
			ChapterFile:  c.ChapterFile,
			ChapterIndex: c.ChapterIndex,
			SceneIndex:   c.SceneIndex,
			ChunkType:    c.ChunkType,
		})
	}
	return results, nil
}
```

import 加入 `"novel-assistant/internal/retriever"`。

---

## Acceptance Criteria 對應

| Issue 完成條件 | 對應 Task |
|---|---|
| `internal/retriever/` package 存在，定義 `Retriever` interface | 1 |
| `VectorRetriever` 行為等價於現有 `QueryFilteredBeforeChapter` | 2、3 |
| handler 不再直接呼叫 `store.QueryFilteredBeforeChapter` | 5 |
| `go build ./...` 與 `go test ./...` 通過 | 6 |
| `internal/server/` 對外 API 行為不變 | 5（現有 e2e tests 保護） |

## 實作注意事項

1. **`VectorStorer` interface vs `*vectorstore.Store`**：Task 2 定義了 `VectorStorer` interface 讓 `VectorRetriever` 可以接受 stub。`*vectorstore.Store` 隱式實作此 interface（它有 `Len()` 和 `QueryFilteredBeforeChapter()`），不需要改 vectorstore package。

2. **`setProjectState` 中 `s.embedder` 的時機**：`setProjectState` 持有 `stateMu` lock，而 `s.embedder` 在 lock 外被更新（`applyProjectSettings` 不持 lock）。目前架構如此，本票不做 thread-safety 改善，沿用現有模式即可。

3. **`s.store.Len() == 0` 判斷移位**：原本 `buildReferenceContext` 最前面的 `if s.store.Len() == 0 { return nil, nil }` 移入 `VectorRetriever.Retrieve`。`buildReferenceContext` 改為在 `len(chunks) == 0` 時回傳 `nil, nil`，行為等價。

4. **`s.store.QueryChapterSummaries` 不在本票範圍**：`handleFocusStream` 中的 `s.store.QueryChapterSummaries(beforeChapter)` 不屬於 `QueryFilteredBeforeChapter` call site，保持不變。

5. **`handleSaveSettings` 的 embedder 重複指派（P2 根因）**：`settings.go` 的 `handleSaveSettings` 在 `applyProjectSettings()` 之後又直接重新 `s.embedder = embedder.New(...)`，導致 retriever 持有過期的 embedder 實例。Task 4-G 修正此路徑；Task 4-D 的 `applyProjectSettings` 更新雖然仍保留（邏輯對稱），但 handleSaveSettings 使用時必須以 Task 4-G 的最終更新為準。

6. **`TestBuildReferenceContextReturnsNilWhenStoreIsEmpty` 初始化缺口（P1 根因）**：重構後 `buildReferenceContext` 的第一行改為 `s.rules.Get()`（nil panic），不再是 `s.store.Len()`。Task 4-F 的修法是把 `s.rules` 和 `s.retriever` 都初始化為空值版本（unloaded rules + empty vectorstore）；因為 store 為空，embed 不會被呼叫，所以 embedder URL 指向不可達位址也不影響測試。
