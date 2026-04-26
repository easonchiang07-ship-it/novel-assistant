# Novel Assistant vs 現有產品對比

The table below compares **capability depth**, not just feature presence.

| 能力 | NovelCrafter | Obsidian + Smart Connections | Sudowrite | **Novel Assistant** |
|---|---|---|---|---|
| **Story Memory** | Codex（手動建立） | 筆記（靜態，不感知故事時序） | 無 | **Auto Memory — 寫完場景自動更新角色狀態、事件、關係** |
| **Foreshadow Tracking** | ❌ | ❌ | ❌ | **主動偵測 + 追蹤 open/hinted/resolved + 提醒** |
| **Narrative Timeline Awareness** | ❌ | ❌ | ❌ | **Scene-aware RAG — 只檢索當前場景以前的內容，防止劇透** |
| **Consistency Checking** | 部分（Codex reference 偵測） | ❌ | ❌ | **系統級：角色行為、對白風格、世界觀、時間軸** |
| **Story Health Scoring** | ❌ | ❌ | 部分（單次感性評論） | **多維度 + 3 run 中位數 + Confidence 指標** |
| **Local-first / 零雲端** | ❌ 雲端 | ✅ | ❌ 雲端 | **完整本地 — Ollama 推理 + 本地向量庫** |
| **資料隱私** | 低（雲端儲存） | 高 | 低 | **完整本地 — 稿件永不離開你的裝置** |
| **訂閱費用** | $10/月 | 免費 + 付費外掛 | $10–$30/月 | **免費開源（MIT）** |
| **中文長篇原生支援** | △ 英文優先 | △ 通用 | △ 英文優先 | **原生設計，繁體中文優先** |
| **對白風格審查（per 角色）** | ❌ | ❌ | ❌ | **✅ 每個角色獨立風格設定** |
| **Diff 改稿對比** | ❌ | ❌ | ✅ | **✅ 含歷史紀錄 + 版本回溯** |
| **審查歷史 + 匯出** | ❌ | ❌ | ❌ | **✅** |

## 核心差異

其他工具知道你的**筆記**。Novel Assistant 知道你的**故事**。

- **NovelCrafter** 最接近，但 Codex 是手動維護的靜態資料庫，不懂故事時序，沒有伏筆追蹤，且閉源付費。
- **Obsidian + Smart Connections** 做到 local-first semantic search，但把所有筆記視為同等重要的文本，不理解「第三章的伏筆」和「第十章的回收」之間的敘事關係。
- **Sudowrite** 生成體驗最好，但完全依賴雲端，有內容審查，且不保留任何長期故事記憶。

## 一句話定位

The only local-first AI writing system that tracks your foreshadowing, detects inconsistencies, and remembers your entire story — without sending a word to the cloud.

唯一會追蹤伏筆、抓出劇情矛盾、並記住整本小說的本地 AI 寫作系統。
