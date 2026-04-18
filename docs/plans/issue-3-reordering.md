# Issue #3 Plan: Drag-and-Drop Reordering

## Goal

Add persistent drag-and-drop reordering for chapters and scenes, and make downstream export follow the saved order.

## Why

Structural iteration is a core fiction workflow.

Reordering should not require manual file renaming or manual scene text shuffling.

## Dependency Note

This issue depends partly on Issue #2:

- chapter reordering can be implemented directly from `master`
- scene reordering inside a visual board depends on the scene board UI from Issue #2

Because of that, the safest implementation path is either:

1. merge Issue #2 first, then build Issue #3 on top of `master`
2. temporarily stack Issue #3 on the Issue #2 branch / PR

## Proposed Storage

- chapter ordering: a project-level JSON file that stores ordered chapter filenames
- scene ordering: a per-chapter sidecar JSON file that stores ordered scene titles

This keeps the Markdown files readable while allowing UI-driven structure changes.

## Scope

1. Add persistent chapter order metadata.
2. Add persistent scene order metadata.
3. Make chapter overview respect saved chapter order.
4. Make scene board respect saved scene order.
5. Make export follow saved order.
6. Add regression tests for reload persistence and export order.

## Open Decision

Should Issue #3:

- wait for Issue #2 to merge, then build on `master`
- or stack directly on top of the Issue #2 branch now

## Done When

- chapter reorder survives reload
- scene reorder survives reload
- export uses saved order instead of filename order
- plain chapters and unordered projects still work
