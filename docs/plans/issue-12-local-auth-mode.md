# Issue 12 工作計畫：可選本機驗證模式

目標：在不影響單機預設使用體驗的前提下，新增可選的本機 / 區網密碼保護模式。

決策：
- 預設維持 `open mode`
- 啟用 `password mode` 時，以單一密碼保護所有頁面與 API
- 使用 cookie session，不做多帳號資料模型
- 未登入頁面導向 `/login`，API 回 `401`

步驟：
1. 在 `internal/config` 新增 auth 環境變數解析與啟用判斷
2. 在 `internal/server` 新增 auth manager、middleware、login/logout handler
3. 新增 `login` 頁面與登出入口
4. 補測試並驗證 `go test ./...`

環境變數：`AUTH_MODE`、`AUTH_PASSWORD`、`AUTH_COOKIE_SECURE`
