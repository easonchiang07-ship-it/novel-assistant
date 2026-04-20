# Issue #36 Spec: Multi-Layer Review Pipeline

Date: 2026-04-20
Issue: #36

## Summary

將目前單一路徑的章節審查流程擴充為雙模式：

- `single`：維持既有 `handleCheckStream` 行為，不做語義改動
- `pipeline`：依序執行四個獨立審稿層，讓結構、角色、世界觀、語言四個面向分段輸出

本設計只新增多層審稿 pipeline，不移除既有單層模式、不加入評分系統、不提供使用者自訂 prompt、不做並行執行。

## Goals

- 保留現有 `single` 模式，確保 backward compatible
- 新增 `pipeline` 模式，固定依序執行四層 review
- 每層使用獨立 prompt 與獨立標題分段輸出
- 後端透過 SSE 發送 `layer_start` / `layer_end`
- 前端在結果區正確插入層標題與層間分隔
- 補齊 regression、pipeline、failure path 測試

## Non-Goals

- 不移除既有 `checks` 機制
- 不做 layer score / rating
- 不允許使用者編輯 layer prompt 模板
- 不支援並行跑多層
- 不新增 `layer_error` SSE event
- 不在第一版前端提供 layer 勾選 UI

## Request Model

`checkRequest` 擴充以下欄位：

```go
type checkRequest struct {
    Chapter   string   `json:"chapter"`
    Scene     string   `json:"scene,omitempty"`
    LayerMode string   `json:"layer_mode"` // "single" | "pipeline"
    Layers    []string `json:"layers,omitempty"`
}
```

實際語義如下：

- `layer_mode == ""` 視為 `single`
- `layer_mode == "single"`：完全沿用目前邏輯
- `layer_mode == "pipeline"`：忽略 `checks`
- `layers` 欄位先保留於 request schema，但第一版前後端都不提供部分啟用能力
- `pipeline` 模式下，空 `layers` 代表固定跑四層全開

其他既有欄位如 `characters`、`styles`、`chapter_file`、`chapter_title`、`scene_title` 仍保留原意。

## Architecture

### New File

新增 `internal/server/review_layers.go`，集中管理：

- `reviewLayer` struct
- `defaultReviewLayers() []reviewLayer`
- `resolveReviewLayers(req checkRequest) []reviewLayer`
- `runPipelineReview(...)`

避免把 layer prompt 與 pipeline control flow 寫進 `handlers.go`。

### reviewLayer

```go
type reviewLayer struct {
    Name    string `json:"name"`
    Label   string `json:"label"`
    Prompt  string `json:"prompt"`
    Enabled bool   `json:"enabled"`
}
```

說明：

- `Name`：穩定 key，供後端與 SSE payload 使用
- `Label`：人類可讀名稱，由後端定義並直接發給前端
- `Prompt`：該層的 prompt 模板
- `Enabled`：保留結構一致性；第一版 `pipeline` 固定四層皆為 `true`

### Layer Definitions

`defaultReviewLayers()` 固定回傳四層，順序不可變：

1. `structure` / `結構層`
2. `character` / `角色層`
3. `world_logic` / `世界觀層`
4. `language` / `語言層`

這個順序同時作為：

- pipeline 執行順序
- SSE 顯示順序
- 測試驗收基準

## Prompt Ownership

所有 layer prompt 模板皆由 `internal/server/review_layers.go` 管理，不放在 handler 中。

四層提示語意如下：

- `structure`
  - 只分析敘事節奏、開場鉤子、張力起伏、段落長短
  - 不評論角色或語言風格
- `character`
  - 只分析角色行為是否符合人設、對白語氣是否一致
  - 可結合角色資料與對話相關 reference
  - 不評論結構或語言
- `world_logic`
  - 只分析設定自洽、時間線、道具與地點邏輯
  - 可結合世界觀、追蹤器、章節 reference
  - 不評論其他層面
- `language`
  - 只分析句式多樣性、重複用語、描寫密度、語言流暢度
  - 可結合 style reference
  - 不評論劇情或角色

## Data Sources Per Layer

### structure

- 只吃章節內容
- 不依賴角色、世界觀、style reference

### character

- 吃章節內容
- 吃角色資料
- 吃現有 character/dialogue retrieval context
- 角色選擇沿用現有 request 語義：
  - 若 request 指定 `characters`，只使用指定角色
  - 若未指定，走既有 `resolveCharacters(req)` 自動辨識與 fallback 邏輯

### world_logic

- 吃章節內容
- 吃世界觀資料與相關 retrieval context
- 若有 tracker/worldstate/context，可沿用既有 world review 可取得的資料

### language

- 吃章節內容
- 可吃 style reference 與必要 context
- prompt 必須明確限制只評論語言層面

## Server Flow

### Single Mode

`layer_mode == ""` 或 `"single"` 時：

- 完全走現有 `handleCheckStream` 流程
- 不發送 `layer_start` / `layer_end`
- 現有 `checks`、`sources`、`retrieval`、`gaps`、`conflict` 行為維持不變

這是強制 regression 邊界，不能順手重構造成語義偏差。

### Pipeline Mode

`layer_mode == "pipeline"` 時：

- 忽略 `checks`
- 解析固定四層 review layers
- 先建立一次共用 retrieval/reference context
- 依固定順序逐層執行
- 每層開始前送 `layer_start`
- 該層 token 仍沿用既有 `chunk` SSE 事件
- 每層完成後送 `layer_end`

`sources`、`retrieval`、`gaps`、`conflict` 等事件維持既有 schema，不新增 layer-specific schema。第一版僅在整體 review 流程前段發送一次，不在每層重送。

## SSE Contract

新增兩種事件：

### `layer_start`

```json
{"layer":"structure","label":"結構層"}
```

### `layer_end`

```json
{"layer":"structure"}
```

說明：

- `layer` 是穩定 key
- `label` 由後端定義，前端不自行做 key-to-label mapping
- 若未來要做多語系，只需要改後端 layer 定義

其餘 `chunk`、`sources`、`retrieval`、`gaps`、`conflict` 事件格式維持現狀。

## Error Handling

pipeline 模式下採「失敗即中止」：

- 某一層失敗時，中止整個 pipeline
- 不繼續執行後續 layer
- 沿用目前 `chunk` 錯誤訊息輸出方式回報失敗
- 不新增 `layer_error` event
- 失敗層不發送 `layer_end`

這可保持與現有 `handleCheckStream` 的失敗語義一致，避免向使用者交付半套且可信度不明的審稿結果。

## Frontend Design

目標檔案：`web/templates/check.html`

### Review Mode Toggle

新增「審稿模式」切換：

- `single`：預設
- `pipeline`

### UI Behavior

#### single

- 顯示目前既有 `checks` UI
- 行為完全維持現狀

#### pipeline

- 隱藏既有 `checks` 區塊
- 不提供 layer 勾選 UI
- 顯示 helper text：
  - `將固定依序執行：結構層、角色層、世界觀層、語言層`

### SSE Rendering

前端收到 `layer_start` 時：

- 在結果區 append 一個分隔標題
- 格式：`── 結構層 ──`

前端收到 `layer_end` 時：

- append 一個空行作為層間間距

前端收到 `chunk` 時：

- 沿用目前串流 append 行為

single 模式下，即使前端已有 `layer_start` / `layer_end` handler，也不應收到此事件。

## Testing Strategy

### 1. Backend Unit Tests

- `defaultReviewLayers()` 固定回傳四層，順序為 structure -> character -> world_logic -> language
- `resolveReviewLayers()` 在 `pipeline` 模式下回傳四層全開
- `single` 模式不走 pipeline runner

### 2. Handler / E2E Tests

- regression test：
  - `layer_mode: "single"` 行為與目前相同
  - stream 中不出現 `layer_start` / `layer_end`
- pipeline test：
  - `layer_mode: "pipeline"` 時，依序輸出四層
  - 事件順序必須符合：
    - `layer_start structure`
    - `chunk`
    - `layer_end structure`
    - `layer_start character`
    - `...`
- failure path：
  - 模擬某一層 LLM 失敗
  - stream 應輸出錯誤 `chunk`
  - pipeline 停在該層
  - 失敗層後續的 layer event 不可出現

### 3. Template / Frontend Tests

- `check.html` 存在審稿模式切換 UI
- pipeline 模式會隱藏 `checks` 區塊
- 前端 SSE 處理存在 `layer_start` / `layer_end` 分支
- `layer_start` 會插入對應標題文字，如 `── 結構層 ──`

## Acceptance Criteria

- `layer_mode: "single"` 行為與現在完全相同
- `layer_mode: "pipeline"` 依序輸出四層 review
- 每層有明確 `layer_start` / `layer_end` 分隔
- 前端在 pipeline 模式可正確顯示四層標題與層間分隔
- `go test ./...` 全通過
- `gofmt -l` 無輸出

## Scope Notes

本票刻意限制在：

- 後端新增 pipeline review path
- review layer 定義集中化
- 前端新增模式切換與分段顯示
- 對應測試

不把 prompt template 系統化、layer score、partial layer selection、parallel execution 混進本票。
