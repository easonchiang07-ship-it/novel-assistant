# Issue 13 工作計畫：Retrieval Diagnostics Page

目標：提供專門頁面，讓使用者不用看 server log 也能檢查 index 健康度與 retrieval 行為。

範圍：
- 顯示向量索引總量與各類型 counts
- 顯示最近一次 reindex 狀態
- 顯示最近幾次 retrieval trace
- 顯示 retrieval latency summary

做法：
1. 在 server 內新增 diagnostics 狀態，保存最近一次 reindex 與 recent traces
2. 在 check / rewrite 的 retrieval 載入點記錄 trace 與 latency
3. 新增 `/diagnostics` 頁面與導覽入口
4. 補測試並驗證 `go test ./...`
