# Novel Assistant Feature Spec

> Updated: 2026-04-19  
> Status: Living product spec  
> Scope: current product baseline + next implementation targets

## Purpose

This document defines how Novel Assistant should evolve from a local review tool into a complete local-first fiction writing studio.

It is meant to sit between:

- `docs/ROADMAP.md`: long-term direction
- `docs/BACKLOG.md`: issue-sized work items
- GitHub Issues / PRs: implementation tracking

This file is the practical product spec:

- what the feature is
- who it is for
- what the user sees
- what data is stored
- what backend/frontend changes are expected
- what "done" means

## Product Baseline

The current shipped product already supports:

- local Ollama-based review and rewrite
- chapter loading and saving
- chapter -> scene parsing
- scene board planning
- drag-and-drop chapter and scene ordering
- review history, diff, and writeback
- timeline / foreshadow / relationship tracking
- project settings, Docker, backup / restore, and manuscript export

That means new work should build on a real writing workflow, not start from scratch.

## Spec Conventions

Each feature section should include:

1. User problem
2. Product behavior
3. Data model
4. Backend changes
5. Frontend changes
6. Validation / edge cases
7. Done criteria

Status values:

- `Shipped`
- `Next`
- `Planned`
- `Future`

---

## F-001 Scene Hierarchy Under Chapters

Status: `Shipped`

### User problem

Chapter-only editing is too coarse for actual fiction work. Writers think in scenes, not only files.

### Product behavior

- A chapter file may contain explicit scene markers:
  - `## Scene 1`
  - `## Scene 2: Title`
- Files without scene markers remain valid and load as plain chapters.
- The review page can target a full chapter or a single scene.
- Scene-aware review history stores scene scope separately from full-chapter scope.

### Data model

- Chapter source remains Markdown in `data/chapters/*.md`
- Scene structure is inferred from Markdown markers, not stored separately
- Review history includes `SceneTitle`

### Done criteria

- Plain chapter files remain backward compatible
- Scene selection is available in the review page
- Scene saves do not overwrite unrelated scenes

---

## F-002 Scene Board Planning

Status: `Shipped`

### User problem

Writers need a board-level view of story structure without opening each scene to inspect it.

### Product behavior

- Chapter overview shows a scene board for chapters with parsed scenes
- Each scene card exposes:
  - synopsis
  - POV
  - conflict
  - purpose
  - status (`draft`, `reviewed`, `rewritten`)
- Scene cards link directly to scene-scoped review

### Data model

- Scene planning lives in sidecar files:
  - `data/chapters/<chapter>.md.scenes.json`
- Sidecar includes:
  - `items[]`
  - `order[]`

### Reliability requirements

- Corrupt sidecar files must not break the entire chapter overview page
- Sidecar JSON should be written in stable order to avoid noisy diffs
- Writes should use replace-on-temp-file semantics

### Done criteria

- Scene board metadata survives reload
- Scene cards show status derived from review history
- Missing or corrupt sidecar data degrades safely

---

## F-003 Drag-and-Drop Reordering

Status: `Shipped`

### User problem

Writers need to restructure chapters and scenes visually, and that order must drive manuscript assembly.

### Product behavior

- Chapters can be drag-reordered on the chapter overview page
- Scenes can be drag-reordered within a chapter's scene board
- Failed saves roll the UI back to the previous order
- Manuscript export follows saved chapter order and scene order

### Data model

- Project-level chapter order:
  - `data/chapter_order.json`
- Scene order:
  - stored in each chapter's `.scenes.json` sidecar

### Reliability requirements

- Ordering must survive reload
- Chapter overview order and manuscript export order must match
- Cross-chapter scene moves are disallowed

### Done criteria

- Chapter and scene order persist
- Overview order matches export order
- Tests cover reorder persistence and chapter overview order

---

## F-004 Retrieval Control Panel

Status: `Next`

Related issues:

- `#4 Add retrieval control panel for story-specific RAG`
- `#5 Add task-specific retrieval presets for review and rewrite`
- `#6 Show expected-but-missed context during retrieval`

### User problem

The current RAG flow is useful but still too opaque. Writers can see retrieved sources, but they still cannot answer:

- Why was this source used?
- Why was another source ignored?
- How much context is enough?
- Can dialogue review and world review use different retrieval behavior?

Without controls, trust is limited.

### Goal

Make retrieval explicit, adjustable, and task-aware without turning the UI into an expert-only debugging panel.

### User-facing behavior

Add a retrieval panel to the review page with:

- source toggles
  - characters
  - worldbuilding
  - styles
  - chapter context
- retrieval controls
  - top-k
  - minimum similarity threshold
- preset selector
  - behavior review
  - dialogue review
  - world conflict review
  - rewrite
  - custom

The panel should:

- start collapsed by default
- show a short summary when collapsed
- be applied to the current review or rewrite request only unless saved as default

### Product rules

- Reasonable defaults should still work without touching the panel
- Invalid settings must fall back safely, not break review
- Users should always be able to tell which retrieval configuration was used for a given review run

### Data model

New project-level retrieval settings store, likely inside existing project settings or a dedicated JSON file:

```json
{
  "retrieval_defaults": {
    "top_k": 5,
    "min_similarity": 0.22,
    "sources": {
      "characters": true,
      "world": true,
      "styles": true,
      "chapters": true
    }
  },
  "retrieval_presets": {
    "behavior": { "...": "..." },
    "dialogue": { "...": "..." },
    "world": { "...": "..." },
    "rewrite": { "...": "..." }
  }
}
```

### Backend changes

Likely files:

- `internal/server/handlers.go`
- `internal/server/settings.go`
- `internal/server/server.go`
- `internal/checker/checker.go`
- `internal/vectorstore/store.go`
- possibly new package:
  - `internal/retrievalsettings/`

Backend responsibilities:

- accept retrieval config in review and rewrite requests
- merge:
  - request override
  - task preset
  - project default
- pass active retrieval config into source selection
- return structured retrieval metadata in the response stream

### Frontend changes

Likely files:

- `web/templates/check.html`
- `web/static/style.css`

Frontend responsibilities:

- render retrieval panel
- keep panel state consistent with selected check / rewrite task
- include retrieval config in:
  - `/check/stream`
  - `/rewrite/stream`
- show an "active retrieval config" summary in result metadata

### Edge cases

- no style files indexed
- no world files indexed
- threshold too high returns zero results
- top-k less than 1
- custom values not parseable
- one source type disabled but another still returns valid results

### Validation

Tests should cover:

- preset merge behavior
- invalid config fallback
- disabled source categories not participating in retrieval
- threshold and top-k bounds
- request metadata includes the active config

### Done criteria

- Users can control source categories, top-k, and threshold in the UI
- Different task presets actually change retrieval behavior
- Result metadata clearly shows what config was used
- Review still works with zero retrieved sources

---

## F-005 Missed-Context Warnings

Status: `Planned`

### User problem

Writers do not only need to know what was retrieved; they need to know what was likely relevant but absent.

### Product behavior

After a review run, the result view should show:

- sources used
- sources likely relevant but not retrieved

Examples:

- a known character appears in the chapter but no character profile was retrieved
- a glossary term appears but no related worldbuilding file was retrieved
- style review ran with no style source active

### Done criteria

- The UI can show "used context" and "missed context" separately
- Missed-context signals are heuristic and non-blocking

---

## F-006 Full Manuscript Build

Status: `Planned`

### User problem

Authors eventually need a full compiled manuscript, not only chapter-level exports.

### Product behavior

- Export assembled manuscript in saved chapter and scene order
- Later support:
  - chapter filtering
  - scene filtering
  - appendix inclusion
  - alternate output formats

### Current baseline

Basic manuscript export already exists and follows saved order.

### Next expansion

Evolve the export flow from "single fixed markdown output" into:

- configurable manuscript build
- multiple output formats
- optional review appendix

### Done criteria

- Export configuration is explicit
- Ordering is preserved
- Output format responsibilities are separated cleanly in code

---

## F-007 Multi-Project Workspace Isolation

Status: `Future`

### User problem

A serious writer may have multiple novels or side projects. A single `data/` root is not safe enough long-term.

### Product behavior

- Multiple projects / workspaces
- Per-project:
  - chapters
  - scene plans
  - vector store
  - review history
  - settings
  - backups

### Done criteria

- Switching projects never leaks context across novels
- Backup and export are project-scoped

---

## Immediate Build Order

The next spec-driven implementation order should be:

1. `F-004 Retrieval Control Panel`
2. `F-005 Missed-Context Warnings`
3. `F-006 Full Manuscript Build expansion`
4. `F-007 Multi-Project Workspace Isolation`

## Notes For Future Updates

When adding a new feature spec here:

- keep roadmap language high-level
- keep backlog issue-sized
- keep this file implementation-aware
- always mark whether the feature is already shipped, next, planned, or future
- link the feature to issue numbers once they exist
