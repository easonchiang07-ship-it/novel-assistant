# Style Analyzer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add style analysis persistence plus rewrite style presets without changing existing style Markdown files.

**Architecture:** Store analyzed style metadata in sidecar JSON files under `data/style/.analysis`, merge that metadata into `profile.StyleGuide` during load, expose analyze/apply APIs from `internal/server`, and inject preset constraints into the existing rewrite prompt path. Update the styles page and rewrite panel to surface the new capabilities while keeping current check and rewrite flows intact.

**Tech Stack:** Go, Gin, html/template, vanilla JavaScript, Ollama-backed checker, Go tests

---

### Task 1: Add style analysis domain model and persistence

**Files:**
- Modify: `internal/profile/model.go`
- Modify: `internal/profile/manager.go`
- Test: `internal/profile/manager_test.go`

- [ ] Step 1: Write failing profile tests for metadata loading and missing sidecar handling.
- [ ] Step 2: Run `go test ./internal/profile -count=1` and confirm the new tests fail for the expected missing-analysis behavior.
- [ ] Step 3: Add `StyleAnalysis`, attach it to `StyleGuide`, and load JSON sidecars from `data/style/.analysis`.
- [ ] Step 4: Re-run `go test ./internal/profile -count=1` and confirm the profile tests pass.

### Task 2: Add checker support for style analysis

**Files:**
- Modify: `internal/checker/checker.go`
- Test: `internal/checker/checker_test.go`

- [ ] Step 1: Write failing checker tests for successful JSON parsing and malformed JSON handling.
- [ ] Step 2: Run `go test ./internal/checker -count=1` and confirm the new tests fail.
- [ ] Step 3: Implement `AnalyzeStyle` with strict JSON prompting and response parsing.
- [ ] Step 4: Re-run `go test ./internal/checker -count=1` and confirm the checker tests pass.

### Task 3: Add server APIs and rewrite preset prompt logic

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/handlers.go`
- Test: `internal/server/handlers_test.go`
- Test: `internal/server/e2e_test.go`

- [ ] Step 1: Write failing tests for preset validation, analysis save behavior, analyze API parse failures, and rewrite prompt injection.
- [ ] Step 2: Run `go test ./internal/server -count=1` and confirm those tests fail.
- [ ] Step 3: Implement style preset definitions, analyze/apply handlers, sidecar save helper, and request payload updates.
- [ ] Step 4: Re-run `go test ./internal/server -count=1` and confirm the server tests pass.

### Task 4: Update styles and rewrite UI

**Files:**
- Modify: `web/templates/styles.html`
- Modify: `web/templates/check.html`

- [ ] Step 1: Add the analyzer UI, analysis rendering, apply action, and preset selector wiring.
- [ ] Step 2: Manually sanity-check that request payload field names match the backend handlers exactly.

### Task 5: Format and verify

**Files:**
- Modify: `internal/checker/checker.go`
- Modify: `internal/checker/checker_test.go`
- Modify: `internal/profile/model.go`
- Modify: `internal/profile/manager.go`
- Modify: `internal/profile/manager_test.go`
- Modify: `internal/server/e2e_test.go`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/handlers_test.go`
- Modify: `internal/server/server.go`
- Modify: `web/templates/check.html`
- Modify: `web/templates/styles.html`

- [ ] Step 1: Run `gofmt -w` on modified Go files.
- [ ] Step 2: Run `go test ./...`.
- [ ] Step 3: Run `gofmt -l` on the repository and confirm no output for touched Go files.
