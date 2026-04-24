# Issue #47 實作計畫：Story Health Scoring

狀態：進行中

## 架構決策

- `checker.EvaluateChapter` 平行跑 3 次 LLM，各回傳 1–5 整數分，系統取中位數
- SSE 串流傳送進度事件（`progress`）與最終結果（`result`），保持 UX 一致性
- Handler 接手計算 weighted base score + penalty/bonus，LLM 不決定最終分數
- version comparison：request 帶可選 `compare_chapter`，同樣跑 3-run，前端並排顯示
- `EvaluationReasons` 以 map[string]string 儲存，prompt 要求每維度附一句中文說明

## 資料流

```
POST /evaluate/stream
  → checker.EvaluateChapter (3 goroutines, median)
      → SSE progress 1/3, 2/3, 3/3
  → computeAdjustments (stale hooks / consistency conflicts)
  → SSE result: FinalEvaluation
```

## 待實作 Checklist

- [ ] **Task 1** `internal/checker/evaluate.go`：型別定義 + `EvaluateChapter`
  - `EvaluationScores`, `EvaluationReasons`, `EvaluationResponse`
  - `StabilityResult`（MedianScores, Variance, SuccessRuns, Confidence）
  - `EvaluateChapter(ctx, chapter, systemPrefix string, onProgress func(run,total int)) (*StabilityResult, error)`
  - 平行 3 goroutine + WaitGroup；每次解析失敗不計入 median；variance = 換算分 max-min

- [ ] **Task 2** `internal/checker/evaluate_test.go`：單元測試
  - 正常 3 次回傳 → 驗證中位數
  - 1 次解析失敗 → success_runs=2，仍能計算
  - Confidence 邊界（variance≤3→High, ≤7→Medium, else→Low）

- [ ] **Task 3** `internal/server/handlers.go`：型別 + handler
  - `ScoreAdjustment`, `FinalEvaluation` struct
  - `computeWeightedScore(scores EvaluationScores) int`
  - `computeAdjustments(chapterIndex, staleThreshold int, worldContext, chapter string) ([]ScoreAdjustment, int)`
  - `handleEvaluateStream`：SSE 端點，支援 `compare_chapter`

- [ ] **Task 4** `internal/server/server.go`：註冊路由
  - `GET /evaluate` → 頁面
  - `POST /evaluate/stream` → SSE

- [ ] **Task 5** `web/templates/evaluate.html`：前端
  - 表單（chapter、chapter_index、compare_chapter 選填）
  - SSE 進度顯示「評估第 N/3 次…」
  - 結果：總分 + Confidence badge + 五維明細 + 調整項 + 版本比較

- [ ] **Task 6** `internal/server/e2e_test.go`：e2e 測試
  - mock 3 LLM 回應 → 驗證 SSE `result` 事件含正確分數
  - penalty 測試：植入 stale hook → 驗證 delta=-5

- [ ] **Task 7** gofmt + `go test ./...`

## Acceptance Criteria 對應

| Criteria | Task |
|---|---|
| LLM 評分端點，嚴格 JSON schema | 1 |
| 3-run 平行 + 中位數 | 1, 2 |
| 確定性加權換算 100 分 | 3 |
| Hook / 衝突 penalty | 3 |
| Confidence | 1, 3 |
| 進度 UX | 3, 5 |
| 基本 UI | 5 |
| 版本比較 | 3, 5 |
