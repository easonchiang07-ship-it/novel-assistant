# Novel Assistant — Codex 實作任務清單

> 建立時間：2026-04-19  
> 用途：交給 Codex 執行的 7 個新功能實作規格  
> 請勿與 FEATURE_SPEC.md（產品 roadmap）混淆

---

## 專案架構速覽

```
novel-assistant/
├── cmd/                        # 進入點
├── internal/
│   ├── checker/checker.go      # 所有 Ollama 呼叫（streaming）
│   ├── server/
│   │   ├── server.go           # 路由設定、Server struct
│   │   └── handlers.go         # HTTP handlers
│   ├── tracker/
│   │   ├── relationship.go     # RelationshipTracker（JSON 持久化）
│   │   └── timeline.go         # TimelineTracker（JSON 持久化）
│   └── profile/manager.go      # 讀取 characters/worldbuilding/style .md 檔
└── web/
    ├── templates/              # Go html/template
    │   ├── _nav.html           # 共用側邊欄
    │   ├── check.html          # 一致性審查頁
    │   ├── relationships.html  # 關係圖頁
    │   └── timeline.html       # 時間軸頁
    └── static/style.css
```

**關鍵約定：**
- Ollama 串流：`checker.go` 的 `stream()` 函式，所有新 AI 功能沿用此模式
- SSE 輸出：前端用 `fetch` + `ReadableStream` 讀取，參考 `check.html` 的 `runCheck()`
- 所有回應語言：繁體中文
- 模板傳入資料：`gin.H{...}` 在 handler 裡組好，模板用 `{{.Field}}` 取用
- JSON 資料傳模板：server.go 已有 `jsonJS` template func，用 `{{jsonJS .Field}}` 輸出 JS-safe JSON
- 現有 checker 方法簽名：`func (c *Checker) XxxStream(ctx context.Context, ..., w io.Writer) error`

---

## Task 1：關聯圖譜視覺化

**純前端，無後端改動。**

### 修改：`web/templates/relationships.html`

**Step 1** — 在 `<head>` 加 vis.js CDN：

```html
<script src="https://cdn.jsdelivr.net/npm/vis-network@9/standalone/umd/vis-network.min.js"></script>
```

**Step 2** — 在 `.page-header` 之後、`.grid-2` 之前插入容器：

```html
<div class="card" style="margin-bottom:24px">
  <h2>關係圖譜</h2>
  <div id="rel-graph" style="height:400px;border:1px solid #e2e8f0;border-radius:8px;background:#f8fafc"></div>
</div>
```

**Step 3** — 在頁面 `<script>` 末尾加入：

```javascript
(function initGraph() {
  const rels = {{jsonJS .Relationships}};
  if (!rels || rels.length === 0) return;

  const colorMap = {
    '信任': '#22c55e', '猜疑': '#f59e0b', '敵對': '#ef4444',
    '愛情': '#ec4899', '家人': '#8b5cf6', '中立': '#94a3b8'
  };

  const nodeSet = new Map();
  rels.forEach(r => {
    if (!nodeSet.has(r.from)) nodeSet.set(r.from, {
      id: r.from, label: r.from, shape: 'ellipse',
      color: { background: '#dbeafe', border: '#3b82f6' }, font: { size: 14 }
    });
    if (!nodeSet.has(r.to)) nodeSet.set(r.to, {
      id: r.to, label: r.to, shape: 'ellipse',
      color: { background: '#dbeafe', border: '#3b82f6' }, font: { size: 14 }
    });
  });

  const edges = rels.map((r, i) => ({
    id: i, from: r.from, to: r.to, label: r.status,
    color: { color: colorMap[r.status] || '#94a3b8' },
    font: { size: 12, color: colorMap[r.status] || '#94a3b8', align: 'middle' },
    arrows: { to: { enabled: true, scaleFactor: 0.8 } },
    smooth: { type: 'dynamic' }
  }));

  new vis.Network(
    document.getElementById('rel-graph'),
    { nodes: new vis.DataSet([...nodeSet.values()]), edges: new vis.DataSet(edges) },
    {
      physics: { solver: 'forceAtlas2Based', stabilization: { iterations: 150 } },
      interaction: { hover: true, tooltipDelay: 200 },
      layout: { improvedLayout: true }
    }
  );
})();
```

---

## Task 2：時間線圖表化

**純前端，無後端改動。**

### 修改：`web/templates/timeline.html`

**Step 1** — 在 `<head>` 加 Chart.js CDN：

```html
<script src="https://cdn.jsdelivr.net/npm/chart.js@4/dist/chart.umd.min.js"></script>
```

**Step 2** — 在 `.page-header` 之後、`.grid-2` 之前插入容器：

```html
<div class="card" style="margin-bottom:24px">
  <h2>事件分佈圖</h2>
  <p style="color:#64748b;font-size:0.85rem">X 軸為章節編號，hover 查看事件名稱</p>
  <canvas id="timeline-chart" height="100"></canvas>
</div>
```

**Step 3** — 在頁面 `<script>` 末尾加入：

```javascript
(function initTimelineChart() {
  const events = {{jsonJS .Events}};
  if (!events || events.length === 0) return;

  const chapterCount = {};
  const dataPoints = events.map(ev => {
    chapterCount[ev.chapter] = (chapterCount[ev.chapter] || 0) + 1;
    return { x: ev.chapter, y: chapterCount[ev.chapter], label: ev.scene, desc: ev.description };
  });

  new Chart(document.getElementById('timeline-chart'), {
    type: 'scatter',
    data: {
      datasets: [{
        label: '事件', data: dataPoints,
        backgroundColor: '#3b82f6', pointRadius: 8, pointHoverRadius: 11
      }]
    },
    options: {
      responsive: true,
      plugins: {
        legend: { display: false },
        tooltip: {
          callbacks: {
            label: ctx => {
              const p = ctx.raw;
              return [`📌 ${p.label}`, p.desc ? p.desc.slice(0, 60) + (p.desc.length > 60 ? '…' : '') : ''];
            }
          }
        }
      },
      scales: {
        x: {
          title: { display: true, text: '章節' },
          min: 0, max: Math.max(...events.map(e => e.chapter), 1) + 1,
          ticks: { stepSize: 1, precision: 0 }
        },
        y: {
          title: { display: true, text: '同章事件序' },
          min: 0, ticks: { stepSize: 1, precision: 0 }
        }
      }
    }
  });
})();
```

---

## Task 3：自動標籤點擊跳轉

**章節預覽中，已知角色名／世界觀名自動變成可點擊連結。**

### 修改：`internal/server/handlers.go`

在 `handleCheckPage` 函式中，補上角色名與世界觀名的清單，傳給模板：

```go
// 在現有的 gin.H{...} 裡加入這兩個欄位
"KnownCharacterNames": func() []string {
    names := make([]string, 0, len(s.profiles.Characters))
    for _, ch := range s.profiles.Characters {
        names = append(names, ch.Name)
    }
    return names
}(),
"KnownWorldNames": func() []string {
    names := make([]string, 0, len(s.profiles.Worlds))
    for _, w := range s.profiles.Worlds {
        names = append(names, w.Name)
    }
    return names
}(),
```

### 修改：`web/templates/check.html`

**Step 1** — 在 `<head>` 裡（其他 `<script>` 之前）注入名稱清單：

```html
<script>
const KNOWN_NAMES = {
  characters: {{jsonJS .KnownCharacterNames}},
  worlds:     {{jsonJS .KnownWorldNames}}
};
</script>
```

**Step 2** — 找到章節 textarea（搜尋 `<textarea` 找到存放章節文字的那個），在它下方加入預覽面板：

```html
<div id="chapter-preview" style="display:none;padding:12px;background:#f8fafc;border:1px solid #e2e8f0;border-radius:8px;line-height:1.9;font-size:0.9rem;white-space:pre-wrap;max-height:300px;overflow-y:auto"></div>
<button class="btn btn-ghost btn-sm" type="button" id="toggle-preview-btn" onclick="toggleTagPreview()" style="margin-top:6px">顯示標籤預覽</button>
```

**Step 3** — 在 `<script>` 加入：

```javascript
function highlightNames(text) {
  if (!text) return '';
  let html = text.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  (KNOWN_NAMES.characters || []).forEach(name => {
    if (!name) return;
    const esc = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    html = html.replace(new RegExp(esc, 'g'),
      `<a href="/characters#char-${encodeURIComponent(name)}" target="_blank"
          style="color:#3b82f6;text-decoration:underline dotted" title="查看角色：${name}">${name}</a>`);
  });
  (KNOWN_NAMES.worlds || []).forEach(name => {
    if (!name) return;
    const esc = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    html = html.replace(new RegExp(esc, 'g'),
      `<a href="/characters#world-${encodeURIComponent(name)}" target="_blank"
          style="color:#8b5cf6;text-decoration:underline dotted" title="查看世界觀：${name}">${name}</a>`);
  });
  return html;
}

let tagPreviewVisible = false;
function toggleTagPreview() {
  const preview = document.getElementById('chapter-preview');
  const btn = document.getElementById('toggle-preview-btn');
  tagPreviewVisible = !tagPreviewVisible;
  if (tagPreviewVisible) {
    // 把 'chapter-textarea' 換成實際 textarea 的 id
    const ta = document.querySelector('textarea[id*="chapter"], textarea[name*="chapter"]');
    preview.innerHTML = highlightNames(ta ? ta.value : '');
    preview.style.display = 'block';
    btn.textContent = '隱藏標籤預覽';
  } else {
    preview.style.display = 'none';
    btn.textContent = '顯示標籤預覽';
  }
}
```

---

## Task 4：感官敘事強化

**在修稿功能新增「五感強化」模式。**

### 修改：`internal/checker/checker.go`

在 `RewriteChapterStream` 函式之後加入：

```go
func (c *Checker) EnhanceSensoryStream(ctx context.Context, chapter string, w io.Writer) error {
	prompt := fmt.Sprintf(`
【待強化章節】
%s

請針對以下面向強化此章節的感官描寫：
1. **視覺**：光線、色彩、空間感、動態細節
2. **聽覺**：環境音、聲音質感、靜默對比
3. **嗅覺**：場景氣味、記憶聯想
4. **觸覺**：材質、溫度、身體感受
5. **味覺**（如適用）

規則：不改變情節與對白；感官描寫須符合場景氛圍；直接輸出修訂後的完整章節內容。
`, chapter)
	return c.stream(ctx, "你是專業文學編輯，擅長將平淡場景改寫為富有感官層次的文學散文。請用繁體中文回答。", prompt, w)
}
```

### 修改：`web/templates/check.html`

找到修稿模式的 `<select>` 或按鈕群組（搜尋 `rewrite`），加入新選項：

```html
<option value="sensory">五感敘事強化</option>
```

### 修改：`internal/server/handlers.go`

在 `handleRewriteStream` 裡，找到判斷 rewrite mode 的 switch-case 或 if-else，加入：

```go
case "sensory":
    err = s.checker.EnhanceSensoryStream(ctx, req.Chapter, cw)
```

---

## Task 5：黃金三章診斷

**新增審查類型，AI 模仿讀者心理評估 Hook 強度。**

### 修改：`internal/checker/checker.go`

加入新方法：

```go
func (c *Checker) DiagnoseOpeningStream(ctx context.Context, chapter string, w io.Writer) error {
	prompt := fmt.Sprintf(`
【待診斷章節】
%s

請模擬普通網路讀者（非文學評論家）閱讀此章節的體驗，從以下五個維度評分並給出建議：

1. **懸念鉤子（Hook）**（1-10分）：開頭是否有讓讀者想繼續讀的疑問或張力？
2. **主角魅力**（1-10分）：主角是否在短時間內展現出鮮明特質或讓人代入的處境？
3. **世界觀吸引力**（1-10分）：設定是否讓讀者感到新奇或有探索欲？
4. **節奏感**（1-10分）：前幾段的閱讀節奏是否流暢、不拖沓？
5. **留存意願**（1-10分）：讀完此章後，讀者會想看下一章嗎？

輸出格式：每項給出分數＋一到兩句說明；最後給出「總體建議」2-3條可立即執行的修改方向。
`, chapter)
	return c.stream(ctx, "你是資深網文平台編輯，擅長從普通讀者視角評估小說開頭的吸引力。請用繁體中文回答。", prompt, w)
}
```

### 修改：`web/templates/check.html`

在審查類型的 checkbox 群組（搜尋 `name="checks"` 或 `name="check"`）加入：

```html
<label class="checkbox-label">
  <input type="checkbox" name="checks" value="opening">
  黃金三章診斷
</label>
```

### 修改：`internal/server/handlers.go`

在 `handleCheckStream` 裡，找到處理各 check 類型的段落（參考 `contains(req.Checks, "behavior")` 的寫法），加入：

```go
if contains(req.Checks, "opening") {
    fmt.Fprintf(cw, "\n\n## 🎯 黃金三章診斷\n\n")
    if err := s.checker.DiagnoseOpeningStream(ctx, req.Chapter, cw); err != nil {
        log.Printf("opening diagnosis: %v", err)
    }
}
```

---

## Task 6：情緒曲線分析

**新增 API + 前端折線圖，顯示章節各段的情緒起伏。**

### 修改：`internal/checker/checker.go`

加入非串流方法（需要 `"encoding/json"` import，若已有則略過）：

```go
type EmotionPoint struct {
	Segment string  `json:"segment"`
	Score   float64 `json:"score"`  // -1.0（極悲）到 1.0（極喜）
	Label   string  `json:"label"`  // 喜悅 / 緊張 / 悲傷 / 憤怒 / 平靜 / 恐懼 / 期待 / 失落
}

func (c *Checker) AnalyzeEmotionCurve(ctx context.Context, chapter string) ([]EmotionPoint, error) {
	prompt := fmt.Sprintf(`
【待分析章節】
%s

請將此章節切成 5-10 個語意段落，對每段分析情緒基調。
輸出嚴格 JSON 陣列，不要有任何額外說明文字：
[
  {"segment": "段落摘要（15字以內）", "score": 0.8, "label": "喜悅"},
  {"segment": "...", "score": -0.5, "label": "緊張"}
]
score 規則：-1.0=極度悲傷/恐懼，0=中性，1.0=極度喜悅/興奮
label 從以下選一個：喜悅、緊張、悲傷、憤怒、平靜、恐懼、期待、失落
`, chapter)

	var buf strings.Builder
	if err := c.stream(ctx, "你是情緒分析專家，只輸出 JSON，不輸出任何額外文字。", prompt, &buf); err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(buf.String())
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("無法解析情緒曲線回應")
	}
	var points []EmotionPoint
	if err := json.Unmarshal([]byte(raw[start:end+1]), &points); err != nil {
		return nil, fmt.Errorf("JSON 解析失敗：%w", err)
	}
	return points, nil
}
```

### 修改：`internal/server/handlers.go`

加入新 handler：

```go
func (s *Server) handleEmotionCurve(c *gin.Context) {
	var req struct {
		Chapter string `json:"chapter"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Chapter) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "章節內容不可為空"})
		return
	}
	points, err := s.checker.AnalyzeEmotionCurve(c.Request.Context(), req.Chapter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"points": points})
}
```

### 修改：`internal/server/server.go`

在 `setupRoutes()` 加入：

```go
r.POST("/api/emotion-curve", s.handleEmotionCurve)
```

### 修改：`web/templates/check.html`

在 `<head>` 加 Chart.js（若 Task 2 已加則略過）：

```html
<script src="https://cdn.jsdelivr.net/npm/chart.js@4/dist/chart.umd.min.js"></script>
```

在審查結果區塊附近加入：

```html
<button class="btn btn-ghost" type="button" onclick="runEmotionCurve()" style="margin-top:8px">分析情緒曲線</button>
<div class="card" id="emotion-card" style="display:none;margin-top:16px">
  <div style="display:flex;justify-content:space-between;align-items:center">
    <h2>情緒曲線</h2>
    <button class="btn btn-ghost btn-sm" onclick="document.getElementById('emotion-card').style.display='none'">關閉</button>
  </div>
  <canvas id="emotion-chart" height="80"></canvas>
</div>
```

在 `<script>` 加入：

```javascript
let emotionChartInstance = null;

async function runEmotionCurve() {
  // 把 querySelector 裡的 selector 換成章節 textarea 的實際選擇器
  const ta = document.querySelector('textarea[id*="chapter"], textarea[name*="chapter"]');
  const text = ta ? ta.value.trim() : '';
  if (!text) { alert('請先輸入章節內容'); return; }

  document.getElementById('emotion-card').style.display = 'block';

  try {
    const resp = await fetch('/api/emotion-curve', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ chapter: text })
    });
    const data = await resp.json();
    if (!resp.ok) throw new Error(data.error || '分析失敗');

    const points = data.points || [];
    if (emotionChartInstance) emotionChartInstance.destroy();
    emotionChartInstance = new Chart(document.getElementById('emotion-chart'), {
      type: 'line',
      data: {
        labels: points.map(p => p.segment),
        datasets: [{
          label: '情緒分數', data: points.map(p => p.score),
          borderColor: '#3b82f6', backgroundColor: 'rgba(59,130,246,0.1)',
          pointBackgroundColor: points.map(p => p.score > 0.3 ? '#22c55e' : p.score < -0.3 ? '#ef4444' : '#f59e0b'),
          pointRadius: 7, tension: 0.4, fill: true
        }]
      },
      options: {
        responsive: true,
        plugins: {
          legend: { display: false },
          tooltip: { callbacks: { afterLabel: ctx => points[ctx.dataIndex]?.label || '' } }
        },
        scales: {
          y: {
            min: -1, max: 1,
            title: { display: true, text: '情緒強度（負=悲/懼，正=喜/奮）' }
          }
        }
      }
    });
  } catch (e) {
    alert('情緒曲線分析失敗：' + e.message);
  }
}
```

---

## Task 7：角色對談室

**新增頁面 `/chat`，作者可與角色進行模擬對話。**

### 修改：`internal/checker/checker.go`

加入新方法：

```go
func (c *Checker) ChatWithCharacterStream(ctx context.Context, characterProfile, history, userMessage string, w io.Writer) error {
	prompt := fmt.Sprintf(`
【對話歷史】
%s

【使用者說】
%s

請以角色身份回應，保持角色設定的語氣、用詞習慣與思考邏輯。不要跳出角色，不要說明自己是 AI。
回應長度：2-6句話，自然對話節奏。
`, history, userMessage)

	system := fmt.Sprintf(`你正在扮演以下角色，嚴格依照設定回應：

%s

規則：只說角色會說的話；保持設定中的語氣與個性；不要使用角色沒有的知識；請用繁體中文回答。`, characterProfile)

	return c.stream(ctx, system, prompt, w)
}
```

### 修改：`internal/server/handlers.go`

加入兩個 handler：

```go
func (s *Server) handleChatPage(c *gin.Context) {
	c.HTML(http.StatusOK, "chat.html", gin.H{
		"Title":      "角色對談室",
		"Characters": s.profiles.Characters,
	})
}

type chatRequest struct {
	CharacterName string `json:"character_name"`
	History       string `json:"history"`
	Message       string `json:"message"`
}

func (s *Server) handleChatStream(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "請求格式錯誤")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		c.String(http.StatusBadRequest, "訊息不可為空")
		return
	}
	char := s.profiles.FindByName(req.CharacterName)
	if char == nil {
		c.String(http.StatusBadRequest, "找不到角色："+req.CharacterName)
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Content-Type-Options", "nosniff")
	if err := s.checker.ChatWithCharacterStream(
		c.Request.Context(), char.RawContent, req.History, req.Message, c.Writer,
	); err != nil {
		log.Printf("chat stream: %v", err)
	}
	c.Writer.Flush()
}
```

### 修改：`internal/server/server.go`

在 `setupRoutes()` 加入：

```go
r.GET("/chat", s.handleChatPage)
r.POST("/chat/stream", s.handleChatStream)
```

### 新增：`web/templates/chat.html`

```html
{{define "chat.html"}}
<!DOCTYPE html>
<html lang="zh-TW">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>角色對談室 — 小說助手</title>
  <link rel="stylesheet" href="/static/style.css">
  <style>
    .chat-container { display:flex; flex-direction:column; height:calc(100vh - 240px); min-height:400px; }
    .chat-messages  { flex:1; overflow-y:auto; padding:16px; background:#f8fafc; border:1px solid #e2e8f0; border-radius:8px; margin-bottom:12px; }
    .chat-bubble    { margin-bottom:12px; max-width:75%; }
    .chat-bubble.user { margin-left:auto; text-align:right; }
    .chat-bubble.char { margin-right:auto; }
    .bubble-inner   { display:inline-block; padding:10px 14px; border-radius:12px; font-size:0.9rem; line-height:1.6; white-space:pre-wrap; }
    .user .bubble-inner { background:#3b82f6; color:#fff; border-bottom-right-radius:3px; }
    .char .bubble-inner { background:#fff; border:1px solid #e2e8f0; color:#1e293b; border-bottom-left-radius:3px; }
    .bubble-name    { font-size:0.75rem; color:#64748b; margin-bottom:3px; }
    .chat-input-row { display:flex; gap:8px; }
    .chat-input-row textarea { flex:1; min-height:52px; max-height:120px; resize:vertical; }
  </style>
</head>
<body>
<div class="layout">
  {{template "nav" .}}
  <main class="main-content">
    <div class="page-header">
      <h1>角色對談室</h1>
      <p>選擇角色後與其進行模擬對話，確認語氣與思考邏輯是否符合設定。</p>
    </div>

    <div class="card">
      <div style="display:flex;gap:12px;align-items:center;margin-bottom:16px">
        <label><strong>選擇角色</strong></label>
        <select id="char-select" onchange="resetChat()">
          <option value="">請選擇角色...</option>
          {{range .Characters}}
          <option value="{{.Name}}">{{.Name}}</option>
          {{end}}
        </select>
        <button class="btn btn-ghost btn-sm" onclick="resetChat()">清除對話</button>
      </div>

      <div class="chat-container">
        <div class="chat-messages" id="chat-messages">
          <p style="color:#94a3b8;text-align:center;font-size:0.85rem">選擇角色後開始對話</p>
        </div>
        <div class="chat-input-row">
          <textarea id="chat-input" placeholder="輸入你想說的話…" onkeydown="handleChatKey(event)"></textarea>
          <button class="btn" id="send-btn" onclick="sendMessage()">送出</button>
        </div>
        <div class="helper-text" style="margin-top:6px">Enter 送出，Shift+Enter 換行</div>
      </div>
    </div>
  </main>
</div>

<script>
let chatHistory = [];
let isStreaming = false;

function resetChat() {
  chatHistory = [];
  document.getElementById('chat-messages').innerHTML =
    '<p style="color:#94a3b8;text-align:center;font-size:0.85rem">選擇角色後開始對話</p>';
}

function appendBubble(role, name, text) {
  const box = document.getElementById('chat-messages');
  box.querySelector('p')?.remove();
  const div = document.createElement('div');
  div.className = `chat-bubble ${role}`;
  div.innerHTML = `<div class="bubble-name">${name}</div><div class="bubble-inner">${
    text.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')
  }</div>`;
  box.appendChild(div);
  box.scrollTop = box.scrollHeight;
  return div.querySelector('.bubble-inner');
}

async function sendMessage() {
  if (isStreaming) return;
  const charName = document.getElementById('char-select').value;
  if (!charName) { alert('請先選擇角色'); return; }
  const input = document.getElementById('chat-input');
  const message = input.value.trim();
  if (!message) return;
  input.value = '';

  appendBubble('user', '作者', message);
  const historyText = chatHistory
    .map(h => `${h.role === 'user' ? '作者' : charName}：${h.content}`)
    .join('\n');

  isStreaming = true;
  document.getElementById('send-btn').disabled = true;
  const replyEl = appendBubble('char', charName, '');
  let replyText = '';

  try {
    const resp = await fetch('/chat/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ character_name: charName, history: historyText, message })
    });
    if (!resp.ok) throw new Error(await resp.text());
    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      replyText += decoder.decode(value, { stream: true });
      replyEl.textContent = replyText;
      document.getElementById('chat-messages').scrollTop = 9999;
    }
    chatHistory.push({ role: 'user', content: message });
    chatHistory.push({ role: 'char', content: replyText });
  } catch (e) {
    replyEl.textContent = '⚠️ 發生錯誤：' + e.message;
  } finally {
    isStreaming = false;
    document.getElementById('send-btn').disabled = false;
  }
}

function handleChatKey(e) {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(); }
}
</script>
</body>
</html>
{{end}}
```

### 修改：`web/templates/_nav.html`

在 `<ul>` 最後一個 `<li>` 之後加入：

```html
<li><a href="/chat" {{if eq .Title "角色對談室"}}class="active"{{end}}>角色對談室</a></li>
```

---

## 實作順序建議

| 順序 | Task | 類型 | 估計工時 |
|---|---|---|---|
| 1 | 關聯圖譜視覺化 | 純前端 | 30 min |
| 2 | 時間線圖表化 | 純前端 | 20 min |
| 3 | 自動標籤點擊跳轉 | 前端 + 輕量後端 | 45 min |
| 4 | 感官敘事強化 | 後端 + 前端 | 30 min |
| 5 | 黃金三章診斷 | 後端 + 前端 | 30 min |
| 6 | 情緒曲線分析 | 後端 + 前端 | 60 min |
| 7 | 角色對談室 | 後端 + 新頁面 | 60 min |

## 注意事項

1. **Task 3、Task 6 的 textarea selector**：`check.html` 中章節 textarea 的實際 id 在執行前需確認，搜尋 `<textarea` 找到章節輸入欄位的 id，替換規格中的 selector。

2. **Task 6 的 JSON 可靠性**：小型 Ollama 模型對 JSON-only 指令的遵循率不穩定。若 parse 失敗，`AnalyzeEmotionCurve` 已回傳 error，前端顯示「分析失敗，請重試或換用更大的模型」即可。

3. **CDN 資源離線化**：若需離線運作，可將 `chart.umd.min.js` 和 `vis-network.min.js` 下載到 `web/static/`，路徑改為 `/static/` 開頭。

4. **每個 Task 獨立**：各 Task 互不依賴，可以分批 commit，失敗不影響其他功能。
