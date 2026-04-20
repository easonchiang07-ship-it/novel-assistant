# Scene-level Chunk RAG Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Change chapter retrieval from whole-chapter vectors to scene or paragraph chunks so review and rewrite flows receive precise excerpts with chapter/scene metadata.

**Architecture:** Keep the existing vectorstore query algorithm and retrieval flow, but replace chapter ingest with a chunk builder that emits scene or paragraph `vectorstore.Document` entries. Propagate chunk metadata through `vectorProfile`, `referenceSummary`, SSE `sources`, and the `check.html` source card renderer so the UI can label each result as `第 N 章・Scene M` or `第 N 章・段落 M`.

**Tech Stack:** Go, Gin, html/template, vanilla JavaScript, Go tests

---

### Task 1: Expand vector document metadata

**Files:**
- Modify: `internal/vectorstore/store.go`
- Test: `internal/vectorstore/store_test.go`

- [ ] **Step 1: Write the failing metadata persistence test**
- [ ] **Step 2: Run test to verify it fails**
- [ ] **Step 3: Write minimal implementation**
- [ ] **Step 4: Run test to verify it passes**
- [ ] **Step 5: Commit**

### Task 2: Add chapter chunk builder and chapter index parsing

**Files:**
- Modify: `internal/server/chapters.go`
- Test: `internal/server/chapters_test.go`

- [ ] **Step 1: Write the failing chunking tests**
- [ ] **Step 2: Run test to verify it fails**
- [ ] **Step 3: Write minimal implementation**
- [ ] **Step 4: Run test to verify it passes**
- [ ] **Step 5: Commit**

### Task 3: Switch ingest from whole chapters to chapter chunks

**Files:**
- Modify: `internal/server/server.go`
- Test: `internal/server/e2e_test.go`

- [ ] **Step 1: Write the failing ingest test**
- [ ] **Step 2: Run test to verify it fails**
- [ ] **Step 3: Write minimal implementation**
- [ ] **Step 4: Run test to verify it passes**
- [ ] **Step 5: Commit**

### Task 4: Propagate chunk metadata through retrieval summaries

**Files:**
- Modify: `internal/server/handlers.go`
- Test: `internal/server/handlers_test.go`
- Test: `internal/server/e2e_test.go`

- [ ] **Step 1: Write the failing metadata propagation tests**
- [ ] **Step 2: Run test to verify it fails**
- [ ] **Step 3: Write minimal implementation**
- [ ] **Step 4: Run test to verify it passes**
- [ ] **Step 5: Commit**

### Task 5: Render chunk location labels in the source cards

**Files:**
- Modify: `web/templates/check.html`
- Test: `internal/server/templates_test.go`

- [ ] **Step 1: Write the failing template test**
- [ ] **Step 2: Run test to verify it fails**
- [ ] **Step 3: Write minimal implementation**
- [ ] **Step 4: Run test to verify it passes**
- [ ] **Step 5: Commit**

### Task 6: Final formatting and verification

**Files:**
- Modify: `internal/vectorstore/store.go`
- Modify: `internal/vectorstore/store_test.go`
- Modify: `internal/server/chapters.go`
- Modify: `internal/server/chapters_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/handlers_test.go`
- Modify: `internal/server/e2e_test.go`
- Modify: `internal/server/templates_test.go`
- Modify: `web/templates/check.html`

- [ ] **Step 1: Run gofmt on modified Go files**
- [ ] **Step 2: Run the full test suite**
- [ ] **Step 3: Verify formatting is clean**
- [ ] **Step 4: Commit**
