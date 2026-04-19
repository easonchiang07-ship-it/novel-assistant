# Issue #4 Plan: Retrieval Control Panel for Story-Specific RAG

## Goal

Add a retrieval control panel to the review UI so authors can choose which
knowledge sources (character, world, style) are active and tune RAG parameters
(top-k, similarity threshold) before each review run.

## Why

`buildReferenceContext` hardcodes `topK=4` and queries all source types with no
threshold. Authors have no visibility into what canon is being used and no way
to exclude irrelevant sources or narrow results.

## What We Know About the Codebase

- `vectorstore.QueryScored(vec, topK, docType)` accepts a single type string; empty string means all types.
- `Ingest()` indexes **character**, **world**, and **style** types — all three can be toggled.
- Style content has two injection paths: (1) vectorstore similarity match via `buildReferenceContext`, and (2) direct full-profile injection via `resolveStyles`. These are not in conflict — the vectorstore returns the most relevant style excerpt as context, while `resolveStyles` injects the complete style guide when the "寫作風格" check is active. The "style" source toggle only controls path (1).
- `reviewrules.Settings` already holds per-session defaults (`ReviewBias`, `DefaultChecks`); retrieval defaults belong here too.
- Settings are saved via `POST /api/settings` in `internal/server/settings.go`. The `settingsSaveRequest` struct must be extended.
- `server.go` registers a `gin.FuncMap` with only `jsonJS`; a new `sourceEnabled` helper must be added there.
- `GET /api/settings` does not exist yet; the JS needs it to read current state before merging a save.

## Constraints

- Backward-compatible: a request with no `retrieval` field must behave like the current system (topK=4, all types, threshold=0).
- Do not change the existing `QueryScored` signature — add a new method.
- The retrieval panel must be visible **before** the user clicks "開始審查".
- Retrieval defaults persist in `review_rules.json` via the existing settings API.

## Scope

1. Extend `reviewrules.Settings` with `RetrievalSources`, `RetrievalTopK`, `RetrievalThreshold`.
2. Add `vectorstore.QueryFilteredScored` supporting multi-type sets and a minimum score threshold.
3. Thread `retrievalOptions` through `checkRequest` / `rewriteRequest` into `buildReferenceContext`.
4. Extend `settingsSaveRequest` and `handleSaveSettings` to persist the new retrieval fields.
5. Add `GET /api/settings` handler.
6. Add a retrieval control panel to `check.html` with source checkboxes, top-k input, threshold slider, live summary badge.
7. Add `sourceEnabled` template helper in `server.go`.
8. Tests for the new vectorstore method and updated `reviewrules` normalization.

## Out of Scope

- Indexing new source types beyond character / world / style.
- A dedicated retrieval settings page (reuse the existing `/settings` page).

---

## Implementation Steps

### Step 1: Extend `reviewrules.Settings`

**File:** `internal/reviewrules/store.go`

Add three fields to `Settings`:

```
RetrievalSources   []string `json:"retrieval_sources"`
RetrievalTopK      int      `json:"retrieval_top_k"`
RetrievalThreshold float64  `json:"retrieval_threshold"`
```

Update `Defaults()` to return:
- `RetrievalSources: []string{"character", "world", "style"}`
- `RetrievalTopK: 4`
- `RetrievalThreshold: 0.0`

Update `normalize()` — add after existing bias normalization:

```
allowed := map[string]struct{}{"character": {}, "world": {}, "style": {}}
filtered := make([]string, 0, len(item.RetrievalSources))
for _, s := range item.RetrievalSources {
    if _, ok := allowed[s]; ok {
        filtered = append(filtered, s)
    }
}
if len(filtered) == 0 {
    item.RetrievalSources = def.RetrievalSources
} else {
    item.RetrievalSources = filtered
}
if item.RetrievalTopK < 1 || item.RetrievalTopK > 20 {
    item.RetrievalTopK = def.RetrievalTopK
}
if item.RetrievalThreshold < 0 || item.RetrievalThreshold > 1 {
    item.RetrievalThreshold = 0.0
}
```

Update `clone()`:
```
item.RetrievalSources = append([]string(nil), item.RetrievalSources...)
```

**Tests (`internal/reviewrules/store_test.go`):**
- Verify defaults when file is absent.
- Verify unknown source types are stripped and fall back to defaults.
- Verify topK / threshold out-of-range values are normalized.

---

### Step 2: Add `vectorstore.QueryFilteredScored`

**File:** `internal/vectorstore/store.go`

Add a new exported method (leave `QueryScored` unchanged):

```
func (s *Store) QueryFilteredScored(queryVec []float64, topK int, types []string, threshold float64) []ScoredDocument {
    s.mu.RLock()
    defer s.mu.RUnlock()

    typeSet := make(map[string]struct{}, len(types))
    for _, t := range types {
        typeSet[t] = struct{}{}
    }

    type pair struct {
        doc   Document
        score float64
    }
    var results []pair
    for _, d := range s.docs {
        if len(typeSet) > 0 {
            if _, ok := typeSet[d.Type]; !ok {
                continue
            }
        }
        score := cosine(queryVec, d.Embedding)
        if threshold > 0 && score < threshold {
            continue
        }
        results = append(results, pair{d, score})
    }
    sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })

    out := make([]ScoredDocument, 0, topK)
    for i := 0; i < topK && i < len(results); i++ {
        out = append(out, ScoredDocument{Document: results[i].doc, Score: results[i].score})
    }
    return out
}
```

**Tests (`internal/vectorstore/store_test.go`):**
- `threshold=0` returns all results (existing behaviour).
- `threshold=0.9` excludes low-similarity documents.
- `types=["character"]` excludes world/style docs.
- `types=nil` includes all types.
- TopK cap is respected when more results qualify.

---

### Step 3: Thread `retrievalOptions` through handlers

**File:** `internal/server/handlers.go`

Add a shared options struct near the other request types:

```
type retrievalOptions struct {
    Sources   []string `json:"sources"`
    TopK      int      `json:"top_k"`
    Threshold float64  `json:"threshold"`
}
```

Add the field to `checkRequest` and `rewriteRequest`:

```
Retrieval retrievalOptions `json:"retrieval"`
```

Update `buildReferenceContext` signature and body:

```
func (s *Server) buildReferenceContext(ctx context.Context, chapter string, opts retrievalOptions) ([]vectorProfile, error) {
    if s.store.Len() == 0 {
        return nil, nil
    }
    queryVec, err := s.embedder.Embed(ctx, chapter)
    if err != nil {
        return nil, err
    }

    rules := s.rules.Get()
    topK := opts.TopK
    if topK < 1 {
        topK = rules.RetrievalTopK
    }
    sources := opts.Sources
    if len(sources) == 0 {
        sources = rules.RetrievalSources
    }

    docs := s.store.QueryFilteredScored(queryVec, topK, sources, opts.Threshold)
    // ... rest unchanged
}
```

Update both call sites in `handleCheckStream` and `handleRewriteStream`:

```
references, err := s.buildReferenceContext(ctx, req.Chapter, req.Retrieval)
```

---

### Step 4: Extend settings save/load

**File:** `internal/server/settings.go`

Add to `settingsSaveRequest`:

```
RetrievalSources   []string `json:"retrieval_sources"`
RetrievalTopK      int      `json:"retrieval_top_k"`
RetrievalThreshold float64  `json:"retrieval_threshold"`
```

Pass new fields to `rules.Update` in `handleSaveSettings`:

```
s.rules.Update(reviewrules.Settings{
    DefaultChecks:      req.DefaultChecks,
    DefaultStyles:      req.DefaultStyles,
    ReviewBias:         req.ReviewBias,
    RewriteBias:        req.RewriteBias,
    RetrievalSources:   req.RetrievalSources,
    RetrievalTopK:      req.RetrievalTopK,
    RetrievalThreshold: req.RetrievalThreshold,
})
```

Add `GET /api/settings` handler:

```
func (s *Server) handleGetSettings(c *gin.Context) {
    rules := s.rules.Get()
    project := s.project.Get()
    c.JSON(http.StatusOK, gin.H{
        "default_checks":      rules.DefaultChecks,
        "default_styles":      rules.DefaultStyles,
        "review_bias":         rules.ReviewBias,
        "rewrite_bias":        rules.RewriteBias,
        "retrieval_sources":   rules.RetrievalSources,
        "retrieval_top_k":     rules.RetrievalTopK,
        "retrieval_threshold": rules.RetrievalThreshold,
        "ollama_url":          project.OllamaURL,
        "llm_model":           project.LLMModel,
        "embed_model":         project.EmbedModel,
        "port":                project.Port,
    })
}
```

Register in `setupRoutes()` in `server.go`:

```
r.GET("/api/settings", s.handleGetSettings)
```

---

### Step 5: Add `sourceEnabled` template helper

**File:** `internal/server/server.go`, inside the existing `gin.FuncMap{...}` literal:

```
"sourceEnabled": func(sources []string, name string) bool {
    for _, s := range sources {
        if s == name {
            return true
        }
    }
    return false
},
```

Pass retrieval defaults in `handleCheckPage` (file: `internal/server/handlers.go`).
`handleCheckPage` already calls `rules := s.rules.Get()` — do **not** redeclare it.
Only add the three new keys to the existing `gin.H{...}` literal:

```
"RetrievalSources":   rules.RetrievalSources,
"RetrievalTopK":      rules.RetrievalTopK,
"RetrievalThreshold": rules.RetrievalThreshold,
```

---

### Step 6: Add retrieval control panel to `check.html`

**File:** `web/templates/check.html`

Insert a new `.card` block **after** "章節檔案庫" card and **before** "審查設定" card:

```html
<div class="card">
  <h2>RAG 擷取設定</h2>
  <p class="helper-text">控制哪些知識來源會被納入本次審查的參考上下文。</p>

  <div id="retrieval-summary" class="helper-text" style="margin-bottom:12px;font-weight:600"></div>

  <div class="form-group">
    <label>啟用來源</label>
    <div class="checkbox-group">
      <label class="checkbox-label">
        <input type="checkbox" name="retrieval-source" value="character"
               {{if sourceEnabled .RetrievalSources "character"}}checked{{end}}>
        角色設定
      </label>
      <label class="checkbox-label">
        <input type="checkbox" name="retrieval-source" value="world"
               {{if sourceEnabled .RetrievalSources "world"}}checked{{end}}>
        世界觀
      </label>
      <label class="checkbox-label">
        <input type="checkbox" name="retrieval-source" value="style"
               {{if sourceEnabled .RetrievalSources "style"}}checked{{end}}>
        寫作風格
      </label>
    </div>
  </div>

  <div class="grid-2">
    <div class="form-group">
      <label>Top-K（最多取幾筆）</label>
      <input type="number" id="retrieval-topk" min="1" max="20" value="{{.RetrievalTopK}}">
    </div>
    <div class="form-group">
      <label>相似度門檻 <span id="threshold-display">{{printf "%.2f" .RetrievalThreshold}}</span></label>
      <input type="range" id="retrieval-threshold" min="0" max="1" step="0.01"
             value="{{.RetrievalThreshold}}"
             oninput="document.getElementById('threshold-display').textContent=parseFloat(this.value).toFixed(2);updateRetrievalSummary()">
    </div>
  </div>

  <button class="btn btn-ghost" type="button" onclick="saveRetrievalDefaults()">設為預設</button>
</div>
```

JavaScript additions (append before closing `</script>`):

```javascript
function getRetrievalOptions() {
  return {
    sources: Array.from(document.querySelectorAll('[name="retrieval-source"]:checked')).map(el => el.value),
    top_k: Math.max(1, Number(document.getElementById('retrieval-topk').value) || 4),
    threshold: Number(document.getElementById('retrieval-threshold').value) || 0,
  };
}

function updateRetrievalSummary() {
  const opts = getRetrievalOptions();
  const labels = { character: '角色', world: '世界觀', style: '風格' };
  const srcStr = opts.sources.length
    ? opts.sources.map(s => labels[s] || s).join('、')
    : '（無）';
  document.getElementById('retrieval-summary').textContent =
    '來源：' + srcStr + '　Top-K：' + opts.top_k + '　門檻：' + opts.threshold.toFixed(2);
}

async function saveRetrievalDefaults() {
  const opts = getRetrievalOptions();
  try {
    const current = await fetch('/api/settings').then(r => r.json()).catch(() => ({}));
    const body = Object.assign({}, current, {
      retrieval_sources: opts.sources,
      retrieval_top_k: opts.top_k,
      retrieval_threshold: opts.threshold,
    });
    const resp = await fetch('/api/settings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
    const data = await resp.json();
    if (!resp.ok) throw new Error(data.error || '儲存失敗');
    alert('已設定為預設擷取設定');
  } catch (e) {
    alert('儲存失敗：' + e.message);
  }
}
```

Wire into `DOMContentLoaded` (add inside the existing listener):

```javascript
document.querySelectorAll('[name="retrieval-source"]').forEach(function(el) {
  el.addEventListener('change', updateRetrievalSummary);
});
document.getElementById('retrieval-topk').addEventListener('input', updateRetrievalSummary);
updateRetrievalSummary();
```

In `runCheck()`, add `retrieval: getRetrievalOptions()` to the fetch body JSON object.
In `runRewrite()`, apply the same addition.

---

### Step 7: Final verification

```
go test ./internal/vectorstore/...
go test ./internal/reviewrules/...
go test ./internal/server/...
go build ./...
```

Fix any failures before marking complete.

---

## Done When

- The retrieval control panel appears on `/check` before the user clicks "開始審查".
- The live summary badge reflects current selections immediately on change.
- Per-source checkboxes (character, world, style) filter vector store results.
- Top-K and threshold inputs are sent in review/rewrite requests and applied in `buildReferenceContext`.
- "設為預設" persists settings to `review_rules.json`; they survive page reload.
- A request with no `retrieval` field falls back to saved defaults from `reviewrules`.
- All `go test ./...` and `go build ./...` pass with no regressions.
