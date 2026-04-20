# Issue 19 工作計畫：Character Pronoun Resolution

目標：改善角色行為審查對 `他/她` 代名詞的處理，降低只用代名詞時漏檢角色的機率。

這輪範圍先聚焦 Phase 1：
- 在 behavior prompt 補上代名詞歸因指引
- 在角色自動選擇時，加入代名詞候選擴充

做法：
1. 補一個角色代名詞推斷 helper，從角色設定推測 `他/她`
2. 在 `resolveCharacters` 內，把代名詞候選角色合併進待審清單
3. 在 `CheckBehaviorStream` prompt 加入代名詞歸因說明
4. 補測試並驗證 `go test ./...`
