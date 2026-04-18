# Novel Assistant

本地隱私的 AI 小說寫作輔助工具，使用 Go + Ollama 構建，無需雲端服務。

## 功能

- **角色一致性審查**：上傳章節，AI 即時分析角色行為是否符合設定，並提供蛻變方案
- **對白風格審查**：檢查角色說話方式是否符合人設
- **關係圖追蹤**：記錄角色間的動態關係與觸發事件
- **時間軸管理**：記錄重要事件，防止前後劇情矛盾
- **伏筆追蹤**：管理已埋下的伏筆，追蹤回收狀態
- **場景式審查流程**：未手選角色時，系統會先從章節內容自動抓取出場角色，再聚焦審查
- **本地 RAG 參考上下文**：重新索引後，角色與世界觀會成為本地知識庫，審查時自動帶入相關上下文
- **Markdown 報告匯出**：將審查結果匯出為可存檔的 .md 報告

## 環境需求

- Go 1.21+
- [Ollama](https://ollama.com/) 本地執行

## 快速開始

```bash
# 1. 安裝 Ollama 模型
ollama pull llama3.2
ollama pull nomic-embed-text

# 2. 安裝 Go 依賴
go mod tidy

# 3. 啟動
go run ./cmd
```

開啟瀏覽器前往 http://localhost:8080

## 角色設定格式

在 `data/characters/` 新增 `.md` 檔案：

```markdown
# 角色：角色名稱
- 個性：...
- 核心恐懼：...
- 行為模式：...
- 弱點：...
- 成長限制：...
- 說話風格：...
```

修改後，在網頁左側點「重新索引」即可更新。

## 整合方向

這個專案現在採用以下組合思路：

- **novelWriter / Manuskript 取向**：角色、世界觀、關係圖、時間軸、伏筆都以寫作工作流為核心，讓你先整理故事結構，再進行章節修稿。
- **AnythingLLM 取向**：角色與世界觀檔案會被索引成可檢索的本地知識庫，審查章節時自動帶入相關參考，避免每次都只靠單輪提示詞硬猜。

建議的使用節奏是：

1. 在 `data/characters/` 與 `data/worldbuilding/` 維護穩定設定
2. 按「重新索引」建立本地知識庫
3. 以單一章節或單一場景進行一致性審查
4. 把新產生的劇情變化補記到關係圖、時間軸與伏筆追蹤

## 專案結構

```
novel-assistant/
├── cmd/main.go              # 入口點
├── internal/
│   ├── config/              # 設定
│   ├── profile/             # 角色設定管理
│   ├── embedder/            # Ollama Embedding
│   ├── vectorstore/         # 本地向量儲存
│   ├── checker/             # 一致性審查邏輯
│   ├── tracker/             # 關係/時間軸/伏筆追蹤
│   ├── exporter/            # Markdown 匯出
│   └── server/              # HTTP 服務
├── web/
│   ├── templates/           # HTML 模板
│   └── static/              # CSS
└── data/
    ├── characters/          # 角色設定 .md
    └── worldbuilding/       # 世界觀設定 .md
```
