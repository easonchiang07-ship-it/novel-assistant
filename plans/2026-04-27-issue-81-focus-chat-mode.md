# Issue #81 實作計畫：Focus Chat Mode

> 狀態：待實作
> Issue：[#81 Focus Chat Mode — split-screen AI chat + manuscript editor](https://github.com/easonchiang07-ship-it/novel-assistant/issues/81)

## 架構決策

- 新路由 `/focus`（GET）和 `/focus/stream`（POST），獨立於現有 `/chat/stream`（角色對談室）
- Focus chat 是「寫作夥伴模式」：context 自動注入，不需使用者指定
- Context 來源：`buildReferenceContext`（傳入 message 作為 query）+ `QueryChapterSummaries` + `foreshadow.GetPending()` + 前端送來的當前稿件內容（`ChapterContent`）
- SSE 格式與現有 `check/stream` 相同（`streamEvent` + `msgChan` + inline SSE loop），支援 `chunk` / `sources` 事件
- 右側編輯器用 `<textarea>`，儲存直接呼叫現有 `POST /api/chapters` endpoint
- 「插入稿件」按鈕在每條 AI 回覆下方，點擊後用 `setRangeText` 插入 textarea 游標位置

## 資料流

```
GET /focus
  → 讀取 listChapterFiles() 供右側選擇章節
  → 渲染 focus.html

POST /focus/stream { message, history[], chapter_file?, chapter_content? }
  → buildReferenceContext(ctx, message, chapter_file, defaultOpts) → []vectorProfile
  → store.QueryChapterSummaries(beforeChapter) → chapter summaries
  → foreshadow.GetPending() → pending hooks
  → 若 chapter_content 非空，加入 contextBlock【當前稿件】區段
  → 組 contextBlock 字串
  → SSE: sources event（vectorProfile 摘要）
  → checker.FocusChatStream(ctx, contextBlock, historyText, message, cw)
  → SSE: chunk events（串流 AI 回覆）
```

## 待實作 Checklist

- [ ] **Task 1** `internal/checker/checker.go`：新增 `FocusChatStream`
- [ ] **Task 2** `internal/server/handlers.go`：新增型別 + `handleFocusPage` + `handleFocusStream`
- [ ] **Task 3** `internal/server/server.go`：路由註冊
- [ ] **Task 4** `web/templates/focus.html`：新增分割畫面模板
- [ ] **Task 5** `web/templates/_nav.html`：新增導航連結
- [ ] **Task 6** e2e 測試（`internal/server/e2e_test.go`）
- [ ] **Task 7** `go build ./...` + `go test ./...`

---

## Task 1：`internal/checker/checker.go`

在 `ChatWithCharacterStream` 之後加入：

```go
func (c *Checker) FocusChatStream(ctx context.Context, contextBlock, history, userMessage string, w io.Writer) error {
	prompt := fmt.Sprintf(`【參考資料】
%s

【對話歷史】
%s

【使用者訊息】
%s`, contextBlock, history, userMessage)

	system := `你是小說創作助理。請根據提供的角色設定、世界觀、章節摘要與伏筆清單，協助作者發想、討論或生成段落。
規則：
- 直接針對使用者訊息回覆，不重複引用資料來源
- 若生成段落，請確保與設定一致
- 回覆使用繁體中文`

	return c.llm.Stream(ctx, system, prompt, w)
}
```

---

## Task 2：`internal/server/handlers.go`

### 2-A 新增型別（加在現有 `chatRequest` 附近）

```go
type focusMessage struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content string `json:"content"`
}

type focusRequest struct {
	Message        string         `json:"message"`
	History        []focusMessage `json:"history"`
	ChapterFile    string         `json:"chapter_file"`    // optional，用於 beforeChapter 篩選
	ChapterContent string         `json:"chapter_content"` // optional，右側 textarea 當前內容
}
```

### 2-B handleFocusPage

```go
func (s *Server) handleFocusPage(c *gin.Context) {
	chapters, err := s.listChapterFiles()
	if err != nil {
		log.Printf("focus: list chapters: %v", err)
	}
	c.HTML(http.StatusOK, "focus.html", gin.H{
		"Title":    "Focus Chat Mode",
		"Chapters": chapters,
	})
}
```

### 2-C handleFocusStream

```go
func (s *Server) handleFocusStream(c *gin.Context) {
	var req focusRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "訊息不可為空"})
		return
	}

	ctx, cancel := context.WithCancel(c.Request.Context())
	msgChan := make(chan streamEvent, 256)

	go func() {
		defer cancel()
		defer close(msgChan)

		// 1. 建立 RAG 參考（傳入 message 作為 embedding query）
		defaultOpts := retrievalOptions{}
		references, refErr := s.buildReferenceContext(ctx, req.Message, req.ChapterFile, mergeRetrieval(s.rules.PresetFor("rewrite"), defaultOpts))
		if refErr != nil {
			msgChan <- streamEvent{Event: "chunk", Text: fmt.Sprintf("\n> RAG 載入失敗，使用基礎模式：%s\n", refErr.Error())}
		} else {
			msgChan <- streamEvent{Event: "sources", Sources: summarizeReferences(references)}
		}

		// 2. 章節摘要
		beforeChapter := resolveBeforeChapter(req.ChapterFile, defaultOpts)
		summaryDocs := s.store.QueryChapterSummaries(beforeChapter)
		var summaryLines []string
		for _, d := range summaryDocs {
			summaryLines = append(summaryLines, fmt.Sprintf("- 第%d章：%s", d.ChapterIndex, d.Content))
		}

		// 3. 待解決伏筆
		pendingHooks := s.foreshadow.GetPending()
		var hookLines []string
		for _, h := range pendingHooks {
			hookLines = append(hookLines, fmt.Sprintf("- %s（偵測：第%d章）", h.Description, h.ChapterIndex))
		}

		// 4. 組 contextBlock
		var parts []string
		if len(references) > 0 {
			parts = append(parts, "【角色／世界觀／風格參考】\n"+joinProfiles(references))
		}
		if len(summaryLines) > 0 {
			parts = append(parts, "【章節摘要】\n"+strings.Join(summaryLines, "\n"))
		}
		if len(hookLines) > 0 {
			parts = append(parts, "【待解決伏筆】\n"+strings.Join(hookLines, "\n"))
		}
		if strings.TrimSpace(req.ChapterContent) != "" {
			parts = append(parts, "【當前稿件】\n"+req.ChapterContent)
		}
		contextBlock := strings.Join(parts, "\n\n")

		// 5. 對話歷史轉字串
		var histLines []string
		for _, h := range req.History {
			role := "使用者"
			if h.Role == "assistant" {
				role = "AI助理"
			}
			histLines = append(histLines, role+"："+h.Content)
		}
		historyText := strings.Join(histLines, "\n")

		// 6. 串流 AI 回覆
		cw := &chanWriter{ch: msgChan}
		if err := s.checker.FocusChatStream(ctx, contextBlock, historyText, req.Message, cw); err != nil {
			if ctx.Err() == nil {
				msgChan <- streamEvent{Event: "chunk", Text: fmt.Sprintf("\n> 錯誤：%s\n", err.Error())}
			}
		}
	}()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	for msg := range msgChan {
		select {
		case <-ctx.Done():
			return
		default:
		}
		switch msg.Event {
		case "sources":
			c.SSEvent("sources", gin.H{"items": msg.Sources})
		case "chunk":
			c.SSEvent("chunk", gin.H{"text": msg.Text})
		}
		c.Writer.Flush()
	}
}
```

> **注意**：`chanWriter` 的 `transcript` 欄位為 optional pointer，Focus 模式不需要審查記錄，傳 `nil` 或直接用 `&chanWriter{ch: msgChan}` 即可。若 `chanWriter` 強制要求 transcript，改為 `&chanWriter{ch: msgChan, transcript: &strings.Builder{}}`。

---

## Task 3：`internal/server/server.go`

在 `setupRoutes()` 的 `protected.GET("/evaluate", ...)` 附近加入：

```go
protected.GET("/focus", s.handleFocusPage)
protected.POST("/focus/stream", s.handleFocusStream)
```

---

## Task 4：`web/templates/focus.html`

新建檔案：

```html
{{define "focus.html"}}
<!DOCTYPE html>
<html lang="zh-TW">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Focus Chat Mode — 小說助手</title>
  <link rel="stylesheet" href="/static/style.css">
  <style>
    .focus-layout {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 16px;
      height: calc(100vh - 160px);
      min-height: 500px;
    }
    .focus-panel { display: flex; flex-direction: column; overflow: hidden; }
    .focus-panel .card { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
    .focus-panel h2 { margin-bottom: 10px; font-size: 1rem; }

    /* 左側：對話 */
    .chat-messages {
      flex: 1; overflow-y: auto; padding: 12px;
      background: #0f0d14; border: 1px solid #2d2540;
      border-radius: 8px; margin-bottom: 10px;
    }
    .chat-bubble { margin-bottom: 10px; max-width: 85%; }
    .chat-bubble.user { margin-left: auto; text-align: right; }
    .chat-bubble.assistant { margin-right: auto; }
    .bubble-inner {
      display: inline-block; padding: 9px 13px; border-radius: 10px;
      font-size: 0.88rem; line-height: 1.65; white-space: pre-wrap;
    }
    .user .bubble-inner { background: #3b82f6; color: #fff; border-bottom-right-radius: 3px; }
    .assistant .bubble-inner { background: #1a1628; border: 1px solid #2d2540; color: #e2e8f0; border-bottom-left-radius: 3px; }
    .bubble-meta { font-size: 0.72rem; color: #64748b; margin-bottom: 3px; }
    .insert-btn {
      display: inline-block; margin-top: 5px; padding: 3px 10px;
      background: transparent; border: 1px solid #3b82f6; color: #3b82f6;
      border-radius: 5px; font-size: 0.75rem; cursor: pointer;
    }
    .insert-btn:hover { background: #3b82f615; }
    .sources-bar {
      font-size: 0.75rem; color: #64748b; padding: 4px 0;
      border-top: 1px solid #2d2540; margin-top: 4px;
    }
    .chat-input-row { display: flex; gap: 8px; }
    .chat-input-row textarea { flex: 1; min-height: 50px; max-height: 110px; resize: vertical; }

    /* 右側：稿件 */
    .editor-toolbar {
      display: flex; gap: 8px; align-items: center;
      margin-bottom: 10px; flex-wrap: wrap;
    }
    .editor-toolbar input[type="text"] { flex: 1; min-width: 160px; }
    #manuscript-editor { flex: 1; resize: none; width: 100%; font-family: monospace; font-size: 0.9rem; }
    .save-status { font-size: 0.75rem; color: #64748b; }
  </style>
</head>
<body>
<div class="layout">
  {{template "nav" .}}
  <main class="main-content">
    <div class="page-header" style="margin-bottom:12px">
      <h1>Focus Chat Mode</h1>
      <p>左側與 AI 寫作夥伴對話，右側直接編輯稿件。點「插入稿件」可將 AI 回覆貼入游標位置。</p>
    </div>

    <div class="focus-layout">
      <!-- 左側：AI 對話 -->
      <div class="focus-panel">
        <div class="card" style="padding:14px">
          <h2>AI 寫作夥伴</h2>
          <div id="sources-bar" class="sources-bar" style="display:none"></div>
          <div class="chat-messages" id="chat-messages">
            <p style="color:#94a3b8;text-align:center;font-size:0.85rem">開始輸入，AI 將自動參考角色設定、章節摘要與伏筆清單</p>
          </div>
          <div class="chat-input-row">
            <textarea id="chat-input" placeholder="輸入你的問題或創作需求…" onkeydown="handleKey(event)"></textarea>
            <button class="btn" id="send-btn" type="button" onclick="sendMessage()">送出</button>
          </div>
          <div style="margin-top:6px;font-size:0.75rem;color:#64748b">Enter 送出，Shift+Enter 換行</div>
        </div>
      </div>

      <!-- 右側：稿件編輯器 -->
      <div class="focus-panel">
        <div class="card" style="padding:14px">
          <h2>稿件編輯器</h2>
          <div class="editor-toolbar">
            <select id="chapter-select" onchange="loadChapter()">
              <option value="">— 新稿件 —</option>
              {{range .Chapters}}
              <option value="{{.Name}}">{{.Title}}</option>
              {{end}}
            </select>
            <input type="text" id="chapter-name" placeholder="章節檔名（例：第二章-主角覺醒.md）">
            <button class="btn btn-ghost btn-sm" type="button" onclick="saveChapter()">儲存</button>
            <span class="save-status" id="save-status"></span>
          </div>
          <textarea id="manuscript-editor" placeholder="在此撰寫或貼入章節內容…"></textarea>
        </div>
      </div>
    </div>
  </main>
</div>

<script>
const chatHistory = [];
let isStreaming = false;

// ── 對話 ────────────────────────────────────────────────────

function escHTML(t) {
  return String(t||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

function appendBubble(role, text) {
  const box = document.getElementById('chat-messages');
  box.querySelector('p')?.remove();
  const wrap = document.createElement('div');
  wrap.className = `chat-bubble ${role}`;
  const label = role === 'user' ? '作者' : 'AI助理';
  wrap.innerHTML = `<div class="bubble-meta">${escHTML(label)}</div><div class="bubble-inner">${escHTML(text)}</div>`;
  box.appendChild(wrap);
  box.scrollTop = box.scrollHeight;
  return wrap.querySelector('.bubble-inner');
}

function addInsertButton(bubbleWrap, text) {
  const btn = document.createElement('button');
  btn.className = 'insert-btn';
  btn.textContent = '▼ 插入稿件';
  btn.onclick = () => insertAtCursor(text);
  bubbleWrap.appendChild(btn);
}

function showSources(items) {
  const bar = document.getElementById('sources-bar');
  if (!items || items.length === 0) { bar.style.display = 'none'; return; }
  const names = items.map(i => i.name || i.id || '').filter(Boolean).join('、');
  bar.textContent = `本次參考：${names}`;
  bar.style.display = 'block';
}

async function sendMessage() {
  if (isStreaming) return;
  const input = document.getElementById('chat-input');
  const message = input.value.trim();
  if (!message) return;
  input.value = '';

  appendBubble('user', message);
  const chapterFile = document.getElementById('chapter-select').value;
  const chapterContent = document.getElementById('manuscript-editor').value;

  isStreaming = true;
  document.getElementById('send-btn').disabled = true;

  const bubbleWrap = (() => {
    const box = document.getElementById('chat-messages');
    const wrap = document.createElement('div');
    wrap.className = 'chat-bubble assistant';
    wrap.innerHTML = `<div class="bubble-meta">AI助理</div><div class="bubble-inner" id="streaming-bubble"></div>`;
    box.appendChild(wrap);
    box.scrollTop = box.scrollHeight;
    return wrap;
  })();
  const bubbleEl = document.getElementById('streaming-bubble');
  let replyText = '';

  try {
    const resp = await fetch('/focus/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message, history: chatHistory, chapter_file: chapterFile, chapter_content: chapterContent })
    });
    if (!resp.ok) throw new Error(await resp.text());

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      const lines = buf.split('\n');
      buf = lines.pop(); // 保留不完整的最後一行

      let eventType = '';
      for (const line of lines) {
        if (line.startsWith('event:')) {
          eventType = line.slice(6).trim();
        } else if (line.startsWith('data:')) {
          const raw = line.slice(5).trim();
          try {
            const payload = JSON.parse(raw);
            if (eventType === 'chunk') {
              replyText += payload.text || '';
              bubbleEl.textContent = replyText;
              bubbleEl.removeAttribute('id'); // 避免重複 id
              bubbleEl.id = 'streaming-bubble';
              document.getElementById('chat-messages').scrollTop = 9999;
            } else if (eventType === 'sources') {
              showSources(payload.items);
            }
          } catch (_) {}
          eventType = '';
        }
      }
    }

    chatHistory.push({ role: 'user', content: message });
    chatHistory.push({ role: 'assistant', content: replyText });
    addInsertButton(bubbleWrap, replyText);
  } catch (e) {
    bubbleEl.textContent = '⚠️ 發生錯誤：' + e.message;
  } finally {
    isStreaming = false;
    document.getElementById('send-btn').disabled = false;
  }
}

function handleKey(e) {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(); }
}

// ── 稿件 ────────────────────────────────────────────────────

function insertAtCursor(text) {
  const ta = document.getElementById('manuscript-editor');
  const start = ta.selectionStart;
  const end = ta.selectionEnd;
  ta.setRangeText(text, start, end, 'end');
  ta.focus();
}

async function loadChapter() {
  const name = document.getElementById('chapter-select').value;
  if (!name) return;
  document.getElementById('chapter-name').value = name;
  try {
    const resp = await fetch('/api/chapters/' + encodeURIComponent(name));
    const data = await resp.json();
    document.getElementById('manuscript-editor').value = data.content || '';
  } catch (e) {
    alert('載入失敗：' + e.message);
  }
}

async function saveChapter() {
  const name = document.getElementById('chapter-name').value.trim();
  const content = document.getElementById('manuscript-editor').value;
  if (!name) { alert('請輸入章節檔名'); return; }
  const status = document.getElementById('save-status');
  status.textContent = '儲存中…';
  try {
    const resp = await fetch('/api/chapters', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, content })
    });
    const data = await resp.json();
    if (!resp.ok) throw new Error(data.error || '儲存失敗');
    status.textContent = `已儲存：${data.title || name}`;
    setTimeout(() => { status.textContent = ''; }, 3000);
  } catch (e) {
    status.textContent = '錯誤：' + e.message;
  }
}
</script>
</body>
</html>
{{end}}
```

---

## Task 6：`internal/server/e2e_test.go`

參照現有 e2e 測試模式（mock LLM streamer），新增以下測試案例：

```
TestFocusPage
  - GET /focus → 200，body 含 "Focus Chat Mode"

TestFocusStreamEmptyMessage
  - POST /focus/stream body={"message":""} → 400

TestFocusStreamBasic
  - POST /focus/stream body={"message":"test"} → 200
  - SSE 回應包含至少一個 event: chunk
  - SSE 回應包含 event: sources（即使 sources 為空陣列）

TestFocusStreamWithChapterContent
  - POST /focus/stream body={"message":"繼續寫","chapter_content":"第一段已完成"}
  - mock LLM 收到的 prompt 應包含「當前稿件」字串
  - （在 mock streamer 裡 assert system/prompt 參數）
```

---

## Task 5：`web/templates/_nav.html`

在現有導航清單中，找到 `/chat` 連結的 `<li>` 之後加入：

```html
<li><a href="/focus" {{if eq .Title "Focus Chat Mode"}}class="active"{{end}}>Focus Chat</a></li>
```

---

## Acceptance Criteria 對應

| Issue 完成條件 | 對應 Task |
|---|---|
| `/focus` 路由可訪問，左右分割版面 | 3、4 |
| AI 對話能串流回應，且自動注入 context | 1、2 |
| 顯示 RAG 參考來源（「本次參考：第N章、角色A」） | 2、4 |
| 「插入稿件」按鈕正常運作 | 4 |
| 右側可儲存為章節檔案 | 4 |
| `go build ./...` 與 `go test ./...` 通過 | 6 |

## 實作注意事項

1. **`chanWriter` transcript 欄位**：若編譯錯誤，改用 `&chanWriter{ch: msgChan, transcript: &strings.Builder{}}`。

2. **`QueryChapterSummaries` 回傳的 `Content`**：文件類型為 `chapter_summary`，`Content` 是章節摘要文字，`ChapterIndex` 是章節序號。若欄位名稱不符，以實際 `vectorstore.Document` 結構為準。

3. **`foreshadow.GetPending()` 回傳的 `PendingHook`**：使用 `h.Description`（描述）與 `h.ChapterIndex`（偵測章節序號）。欄位定義以 `tracker/foreshadow.go` 的 `PendingHook` struct 為準。

4. **SSE chunk 格式**：前端期待 `event: chunk\ndata: {"text":"..."}\n\n`。確認現有 `c.SSEvent("chunk", gin.H{"text": msg.Text})` 輸出格式一致。

5. **右側 `<textarea>` 高度**：用 `flex: 1` 撐滿剩餘空間，需要外層 `.focus-panel .card` 設定 `display:flex; flex-direction:column`（已含在 CSS 中）。
