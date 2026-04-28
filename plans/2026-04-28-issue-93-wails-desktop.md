狀態：Phase 1 & 2 已完成，Phase 3 待實作
Issue：[#93 Wails desktop packaging — single executable, no Docker required](https://github.com/easonchiang07-ship-it/novel-assistant/issues/93)

## 架構決策

### Phase 1：最小桌面殼

**方案：gin 作為 Wails AssetServer.Handler**

- 不另開 TCP port；WebView 的請求直接走 gin.Engine.ServeHTTP
- `webfs.go`（project root）用 `//go:embed web` 把 `web/` 打進 binary
- `internal/server.NewEmbedded(cfg, webFS)` 用 `template.ParseFS` 取代 `LoadHTMLGlob`，用 `StaticFS` 取代 `Static`
- `server.Handler()` 暴露 `s.router` 給 Wails
- 所有 desktop 程式碼加 `//go:build desktop`，避免影響 CI（`go test ./...` 不含此 tag）

**Build 指令**
```bash
CGO_ENABLED=1 go build -tags desktop -o novel-assistant-desktop.exe ./cmd/desktop/
```

**為何不用 `wails build`**
`wails build` 是給有 npm frontend 的專案用。我們的前端是 gin server-side templates，只需 `go build` + embedded FS。

### Phase 2（待實作）：First-run UX
- 首次啟動偵測 Ollama（呼叫 `/api/ollama/status`）
- 工作目錄改為 `os.UserHomeDir()/NovelAssistant/`

### Phase 3（待實作）：高頻 API 遷移到 Wails bind
- 章節載入/儲存、重新索引、評分

## 資料夾結構

```
novel-assistant/
  webfs.go                    ← //go:build desktop, 嵌入 web/
  desktop/
    app.go                    ← //go:build desktop, App struct
  cmd/desktop/
    main.go                   ← //go:build desktop, wails.Run 入口
  internal/server/
    server.go                 ← 新增 NewEmbedded / Handler / setupRouter
```

## 相依性注意事項

`github.com/wailsapp/wails/v2` 只被 `//go:build desktop` 檔案引用，
`go mod tidy`（不帶 `-tags desktop`）會把它清掉。
不要在這個 repo 跑無 tag 的 `go mod tidy`。
加依賴用 `go get`，`go.sum` 保持原樣。
