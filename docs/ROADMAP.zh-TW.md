# 產品藍圖

[English](ROADMAP.md) | 繁體中文

> 我們不是在打造 AI 寫作工具。
> 我們在打造一個記住你整本小說的系統。

---

## ✅ 已完成

以下功能已合併進 `main`。

| 功能 | Issue / PR |
|---|---|
| 伏筆追蹤器 — 偵測、確認/忽略、未回收清單、過期提醒 | #45 / #67 |
| Scene-aware RAG — 時間軸邊界檢索，不洩漏未來情節 | #44 / #65 |
| 自動敘事記憶 — 每章提取事件、關係、世界觀變化、伏筆 | #46 |
| Story Health 評分 — 3 次中位數、Confidence、變異數、結構化評分表 | #47 / #73 |
| 章節摘要記憶層 — `SummarizeChapter` + `chapter_summary` 索引 | #79 |
| LLMStreamer 介面 — Provider 抽象、Mock 測試 | #74 / #75 |

---

## 🔥 Phase 0 — First Wow Experience

> 目標：讓使用者在幾秒內說出「這個 AI 真的看懂我的故事」。

### Story Health Analyzer ✅
- 一鍵分析（章節或貼上文字）
- 穩定分數（3 次中位數）+ Confidence 指標
- 具體問題與亮點

### Story Health Share Card — [#77](https://github.com/easonchiang07-ship-it/novel-assistant/issues/77)
- 生成可分享 PNG
- 分數 + Confidence + 關鍵問題（精簡版）+ 品牌標識
- 支援改稿前 / 後對比

### Demo Experience — [#80](https://github.com/easonchiang07-ship-it/novel-assistant/issues/80)
- 內建範例小說，開箱即用
- 預先生成的分析結果，不需要 Ollama 也能看到
- 零設定即可體驗（Demo Mode）

**成功條件：** 使用者想截圖分享。GitHub Star 有機地成長。

---

## 🖥️ Phase 2 — 易用性與首次啟動

> 目標：非技術使用者在 5 分鐘內開始使用。

### First-run Experience — [#78](https://github.com/easonchiang07-ship-it/novel-assistant/issues/78)
- 偵測 Ollama 是否安裝
- 引導安裝與下載模型
- 依硬體分級推薦模型（Entry / Standard / Advanced / Pro）
- 先展示價值，再等待安裝完成（Demo Mode 作為入口）

### Focus Chat Mode — [#81](https://github.com/easonchiang07-ship-it/novel-assistant/issues/81)
- 左側：上下文感知的 AI 對話
- 右側：稿件編輯器
- 一鍵將生成內容插入正式稿
- 透明顯示 RAG 參考來源（「本次參考了第 1–3 章 + 角色設定」）

**成功條件：** 不需要看文件。第一次使用者能無摩擦地上手。

---

## 🧱 Phase 3 — 可擴展架構

> 目標：支援大規模寫作，且不需要未來全部重寫。
> ⚠️ 只建地基 — 避免過早優化。

### Scene-based 資料模型 ✅（部分完成）
- `Document` 已有 `SceneIndex` 欄位
- `parseScenes()` 與 scene 層級 chunk 索引已就位
- 待完成：將 `scene_id` 傳遞至 tracker 與敘事記憶寫入

### 章節摘要層 ✅
- 索引時呼叫 `SummarizeChapter()`，每章生成 100 字摘要
- `QueryChapterSummaries(beforeChapter)` 可供檢索使用
- 摘要嵌入向量並以 `chapter_summary` 文件類型儲存

### Retriever 抽象介面 — [#82](https://github.com/easonchiang07-ship-it/novel-assistant/issues/82)
```go
type Retriever interface {
    Retrieve(ctx context.Context, req RetrievalRequest) ([]ContextChunk, error)
}
```
- 讓 handler 不再直接依賴 `*vectorstore.Store`
- 未來換成 hybrid search 或 rerank 時，上層功能不需要重寫

**本 Phase 明確不做：**
- 完整 hybrid search（BM25 + vector）
- Rerank
- State graph
- Incremental indexing

**成功條件：** 未來不需要重寫。可擴展至 50 萬字以上。

---

## ☁️ Phase 4 — 雲端版本

> 目標：打造一個不同的產品，而不是更強的本地版。

- 大規模敘事引擎：支援 100 萬–1000 萬字，分層記憶，多階段檢索
- 多 Agent 編輯系統：邏輯編輯、情節編輯、文筆編輯，結構化反饋
- Series / Show Bible：角色、世界觀、時間軸、伏筆、弧線追蹤
- 協作：多人編輯、寫作室工作流、雲端同步

**成功條件：** 使用者願意付費。與開源版本有清楚的差異。

---

## 🧠 Phase 5 — 敘事作業系統

- 讀者模擬
- 風格鎖定
- 故事規劃系統
- 出版流水線

---

## 產品哲學

- 不跟生成品質比 — 那是模型的工作。
- 比的是敘事記憶與一致性。
- 優化的是信任感，而不是新鮮感。

---

## 一句話

Most AI tools help you write more.
We help you write better — by remembering everything.

多數 AI 工具幫你寫更多。
我們幫你寫更好 — 因為我們記住了一切。
