# Issue 12 工作計畫：可選本機驗證模式

## 目標

在不影響單機預設使用體驗的前提下，新增可選的本機 / 區網密碼保護模式。

## 範圍與決策

- 保留預設 `open mode`，未設定驗證環境變數時維持現況
- 新增 `password mode`，用單一密碼保護所有頁面與 API
- 使用 cookie session，避免每次請求都重送密碼
- 設定來源以環境變數為主，不做多帳號資料模型
- 未登入的頁面導向 `/login`，未登入的 API 回傳 `401`

## 實作步驟

1. 在 `internal/config` 新增 auth 環境變數解析與啟用判斷
2. 在 `internal/server` 新增 auth manager、middleware、login/logout handler
3. 調整 router，讓既有頁面與 API 統一走保護中介層
4. 新增 `login` 頁面與登入/登出操作入口
5. 補測試，驗證 open mode、未登入阻擋、登入後放行與登出失效

## 環境變數與驗證

- `AUTH_MODE=open|password`
- `AUTH_PASSWORD=...`
- `AUTH_COOKIE_SECURE=true|false`
- `go test ./...`
