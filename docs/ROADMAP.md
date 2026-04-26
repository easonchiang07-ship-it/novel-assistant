# Roadmap

English | [繁體中文](ROADMAP.zh-TW.md)

> We are not building an AI writing tool.
> We are building a system that remembers your story.

---

## ✅ Already Shipped

These features are complete and merged into `main`.

| Feature | Issue / PR |
|---|---|
| Hook Tracker — detect, confirm/dismiss, unresolved list, stale reminders | #45 / #67 |
| Scene-aware RAG — timeline-bounded context, no future leakage | #44 / #65 |
| Auto Narrative Memory — extract events, relationships, world state, hooks per chapter | #46 |
| Story Health Scoring — 3-run median, confidence, variance, structured rubric | #47 / #73 |
| Per-chapter Summary Memory — `SummarizeChapter` + `chapter_summary` index layer | #79 |
| LLMStreamer interface — provider abstraction, mock-based tests | #74 / #75 |

---

## 🔥 Phase 0 — First Wow Experience

> Goal: Make users say *"this AI actually understands my story"* within seconds.

### Story Health Analyzer ✅
- One-click analysis from chapter or pasted text
- Stable score (3-run median), confidence indicator
- Concrete issues and strengths

### Story Health Share Card — [#77](https://github.com/easonchiang07-ship-it/novel-assistant/issues/77)
- Generate shareable PNG
- Score + Confidence + key issues (short form) + branding
- Before / after comparison support

### Demo Experience — [#80](https://github.com/easonchiang07-ship-it/novel-assistant/issues/80)
- Example story included out of the box
- Pre-generated analysis result viewable without Ollama running
- Usable without any setup (demo mode)

**Success criteria:** Users want to screenshot and share. Organic GitHub stars start growing.

---

## 🖥️ Phase 2 — Usability & First-run

> Goal: Non-technical users can start within 5 minutes.

### First-run Experience — [#78](https://github.com/easonchiang07-ship-it/novel-assistant/issues/78)
- Detect Ollama installation
- Guide install and model pull
- Hardware-based model recommendation (Entry / Standard / Advanced / Pro)
- Show value before setup completes (demo mode as entry point)

### Focus Chat Mode — [#81](https://github.com/easonchiang07-ship-it/novel-assistant/issues/81)
- Left panel: AI chat with context-aware generation
- Right panel: manuscript editor
- One-click insert generated content into draft
- RAG context shown transparently ("I'm using chapters 1–3 + character profiles")

**Success criteria:** No documentation needed. First-time users succeed without friction.

---

## 🧱 Phase 3 — Architecture for Scale

> Goal: Support large-scale writing without a full rewrite.
> ⚠️ Build the foundation only — avoid premature optimization.

### Scene-based Data Model ✅ (partial)
- `SceneIndex` field exists on `Document`
- `parseScenes()` and scene-level chunk indexing in place
- Remaining: propagate `scene_id` to tracker and narrative memory writes

### Chapter Summary Layer ✅
- `SummarizeChapter()` generates a 100-word summary per chapter on index
- `QueryChapterSummaries(beforeChapter)` available for retrieval
- Summary embedded and stored as `chapter_summary` document type

### Retriever Abstraction — [#82](https://github.com/easonchiang07-ship-it/novel-assistant/issues/82)
```go
type Retriever interface {
    Retrieve(ctx context.Context, req RetrievalRequest) ([]ContextChunk, error)
}
```
- Decouple handlers from `*vectorstore.Store` direct calls
- Enables future swap to hybrid search or rerank without rewriting upper layers

**Not in this phase:**
- Full hybrid search (BM25 + vector)
- Rerank
- State graph
- Incremental indexing

**Success criteria:** No future rewrite required. Scales toward 500k+ words.

---

## ☁️ Phase 4 — Cloud Version

> Goal: Build a different product, not a stronger version of the local tool.

- Large-scale Narrative Engine: 1M–10M+ word support, hierarchical memory, multi-stage retrieval
- Multi-Agent Editorial System: logic / plot / prose editors, structured feedback
- Series / Show Bible: characters, world, timeline, hooks, arcs
- Collaboration: multi-user editing, writer's room workflow, cloud sync

**Success criteria:** Users willing to pay. Clear separation from the open-source version.

---

## 🧠 Phase 5 — Narrative Operating System

- Audience simulation
- Style lock
- Story planning system
- Publishing pipeline

---

## Product Philosophy

- Do not compete on generation quality — that is the model's job.
- Compete on narrative memory and consistency.
- Optimize for trust, not novelty.

---

## One-line Summary

Most AI tools help you write more.
We help you write better — by remembering everything.
