# Issue #82 實作計畫：Retriever Abstraction Interface

> 狀態：待實作
> Issue：[#82 Retriever abstraction interface — decouple handlers from vectorstore.Store](https://github.com/easonchiang07-ship-it/novel-assistant/issues/82)
> 注意：Issue 標記為 Phase 3 地基，Phase 0 / Phase 2 完成前不需要實作。

## 架構決策

- 新建 `internal/retriever/` package，定義 `Retriever` interface 與 `VectorRetriever` 實作
- `VectorRetriever` 在 `Retrieve()` 內部負責 embed + query，handler 只傳字串 query
- `Retriever` 內定義 `Embedder` interface（`Embed(ctx, text) ([]float64, error)`），讓 VectorRetriever 可以在不啟動 Ollama 的情況下單元測試
- `buildReferenceContext` 的 `s.store.QueryFilteredBeforeChapter` 替換為 `s.retriever.Retrieve`
- `s.store.Len() == 0` 的早退邏輯移入 `VectorRetriever.Retrieve`，handler 不再直接存取 store
- `server.go` 在 `setProjectState` 與 `applyProjectSettings` 兩個位置同步更新 `s.retriever`

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

## 待實作 Checklist

- [ ] **Task 1** `internal/retriever/retriever.go`：介面定義
- [ ] **Task 2** `internal/retriever/vector.go`：VectorRetriever 實作
- [ ] **Task 3** `internal/retriever/vector_test.go`：單元測試
- [ ] **Task 4** `internal/server/server.go`：Server struct + 初始化 + 同步更新
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

// stubStore 模擬 VectorStorer。
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

func TestVectorRetrieverTopKRespected(t *testing.T) {
	docs := make([]vectorstore.ScoredDocument, 5)
	for i := range docs {
		docs[i] = vectorstore.ScoredDocument{
			Document: vectorstore.Document{ID: "doc", Type: "character"},
			Score:    float64(i),
		}
	}
	store := &stubStore{docs: docs}
	r := retriever.NewVector(&stubEmbedder{vec: []float64{1}}, store)

	chunks, err := r.Retrieve(context.Background(), retriever.Request{Query: "x", TopK: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 3 {
		t.Errorf("expected 3 chunks (topK=3), got %d", len(chunks))
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
