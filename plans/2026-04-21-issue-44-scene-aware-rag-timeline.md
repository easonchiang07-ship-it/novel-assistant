# Issue #44 實作計畫：Scene-aware RAG timeline-bounded context retrieval

狀態：進行中

## 背景

現有 `QueryFilteredScored` 只過濾 type，沒有依 `chapter_index` 排除未來章節向量。
`chapter_index` 已正確寫入 vectorstore（`extractChapterIndex` 解析檔名），只差 retrieval 端過濾。

## 架構決策

- 只對 `type == "chapter"` 的 doc 套用 `before_chapter` 上界過濾；角色/世界觀/風格向量不受影響
- `before_chapter` 由後端從 `chapterFile` 自動計算（`extractChapterIndex`），不需要前端手動傳遞
- `retrievalOptions` 加入 `BeforeChapter int` 欄位供未來 override 保留彈性，但預設行為是自動填入
- `retrievalSummary` 加入 `BeforeChapter int` 欄位，前端顯示「僅含第 N 章以前」

## 待實作 Checklist

- [ ] **Task 1**：`vectorstore.Store` 新增 `QueryFilteredBeforeChapter` 方法
  - 簽名：`QueryFilteredBeforeChapter(queryVec []float64, topK int, types []string, threshold float64, beforeChapter int) []ScoredDocument`
  - 只對 `type == "chapter"` 的 doc 做 `ChapterIndex >= beforeChapter` 排除（`beforeChapter <= 0` 代表不過濾）
  - 補單元測試：`beforeChapter=0` 不過濾、`beforeChapter=3` 排除 idx≥3 的 chapter doc、非 chapter doc 不受影響

- [ ] **Task 2**：`retrievalOptions` 加 `BeforeChapter int`；`retrievalSummary` 加 `BeforeChapter int`
  - `mergeRetrieval` 傳遞 `BeforeChapter`（如果 override 有設才覆蓋）
  - `summarizeRetrieval` 傳入 `beforeChapter` 寫入 summary

- [ ] **Task 3**：`buildReferenceContext` 自動從 `chapterFile` 計算 `before_chapter` 並使用新 query 方法
  - `before_chapter = extractChapterIndex(chapterFile)`；若 `chapterFile == ""` 或 `before_chapter == 0`，不過濾
  - 若 `opts.BeforeChapter > 0`，以 opts 值優先（override 路徑）
  - 呼叫 `QueryFilteredBeforeChapter` 取代 `QueryFilteredScored`

- [ ] **Task 4**：前端 `check.html` `renderAppliedRetrieval` 顯示時間範圍
  - 若 `cfg.before_chapter > 0`，在 retrieval 摘要行尾加上 ` / 限第 N 章以前`

- [ ] **Task 5**：補 e2e 測試
  - 情境：兩個 chapter 向量（index 2 和 index 5），`chapterFile` = index 3 的章節
  - 驗證：retrieval 結果只含 index 2，不含 index 5

- [ ] **Task 6**：`gofmt -l` 無輸出 + `go test ./...` 全過

## 驗證方式

```
go test ./internal/vectorstore/... -v -run "TestQueryFilteredBeforeChapter"
go test ./internal/server/... -v -run "TestBuildReferenceContextExcludesFutureChapters"
go test ./... -timeout 120s
gofmt -l internal/vectorstore/ internal/server/
```
