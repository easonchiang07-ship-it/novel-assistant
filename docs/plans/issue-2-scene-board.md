# Issue #2 Plan: Scene Board UI

## Goal

Build a scene board that lets writers scan chapter scenes visually and move quickly into scene-level editing and review.

## Why

Novel Assistant already supports scene markers and scene-scoped review, but it still lacks a planning surface comparable to novelWriter or Manuskript.

A scene board closes that gap by making structure visible without opening full chapter text.

## Constraints

- Keep backward compatibility with plain chapter files that do not use scene markers.
- Reuse the current `## Scene N` / `## Scene N: Title` parsing model.
- Do not break the existing chapter overview and review page workflows.
- Prefer file-based storage that stays readable in Git.

## Scope For This Issue

1. Define scene planning metadata for:
   - synopsis
   - POV
   - conflict
   - purpose
   - quick status markers
2. Add backend support to load and save that metadata.
3. Add a scene board page or chapter-level board section with visual scene cards.
4. Link each scene card to the current editing / review flow.
5. Add tests for parsing, persistence, and board-facing data.

## Implementation Steps

1. Inspect current chapter and scene loading flow.
2. Choose a storage shape for scene planning metadata that is simple and Git-friendly.
3. Extend backend chapter loading so scene board data is available in one request.
4. Build the scene board UI with clear cards and quick actions.
5. Add tests and run formatting, `go test ./...`, and `go build ./...`.

## Done When

- Scenes are visible as cards from a board-style UI.
- Each card shows planning metadata and review status.
- Users can jump from a scene card into editing or review quickly.
- Plain chapters still work without errors.
