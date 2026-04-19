# 貢獻指南

[English](CONTRIBUTING.md) | 繁體中文

感謝你幫助改善 Novel Assistant。

## 開始前

- 直接用編輯器開啟 `novel-assistant` 資料夾。
- 使用 Go `1.21+`。
- 若要端對端驗證嵌入或審查流程，請在本地安裝 Ollama。

## 開發工作流程

1. 盡可能從 GitHub Issue 出發。
2. 為該 Issue 建立專屬分支，不要直接 commit 到 `master`。
3. 保持改動聚焦。盡可能將重構與功能開發分開。
4. 開 Pull Request 前，先執行格式化、測試與建置檢查。
5. 當行為、設定或公開工作流程有所更動時，同步更新文件。
6. 透過審查過的 Pull Request 合併，再關閉 Issue。

詳細規範請見 [docs/DEVELOPMENT_WORKFLOW.md](docs/DEVELOPMENT_WORKFLOW.md)。

建議的分支命名：

- `feature/issue-<number>-short-name`
- `fix/issue-<number>-short-name`
- `docs/issue-<number>-short-name`
- `chore/issue-<number>-short-name`

## 常用指令

Windows PowerShell：

```powershell
./scripts/dev.ps1 fmt
./scripts/dev.ps1 test
./scripts/dev.ps1 build
./scripts/dev.ps1 run
```

任何已安裝 Go 的 shell：

```bash
gofmt -w ./cmd ./internal
go test ./...
go build ./...
go run ./cmd
```

## Pull Request 要求

- 連結正在關閉或推進的 Issue。
- 說明使用者面臨的問題。
- 摘要說明選擇的解決方案與取捨。
- 說明任何遷移、資料安全或向下相容性的注意事項。
- 在實際可行的情況下，為新的解析、驗證或審查行為加入測試覆蓋。
- 若 UI 有可見的變更，請附上截圖或短片。

## 範圍指引

適合新手的貢獻：

- 角色、風格或世界觀檔案的解析器改進
- 更好的驗證與錯誤訊息
- 文件與上線體驗改進
- 審查流程與追蹤器的額外測試

較大型的改動通常應先開 Issue：

- 重大資料格式變更
- 替換 Ollama 整合行為
- 新的工作流程頁面或儲存模型
- 認證、同步或雲端功能
