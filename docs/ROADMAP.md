# Roadmap

This roadmap reflects the current product direction after chapter workflow, review history, rewrite diff, project settings, Docker support, and local backup / restore were added.

## Current Focus

Novel Assistant is now strongest as a local-first fiction review workstation.

The next step is not adding more isolated features, but deepening three product layers:

- manuscript structure and writing flow
- controllable story-specific RAG behavior
- safer long-term project management

## Priority 1: Writing Structure

Goal: move from chapter review to full manuscript organization.

- Add `Chapter -> Scene` hierarchy instead of chapter-only files
- Build a scene board with synopsis, POV, conflict, and purpose fields
- Support drag-and-drop reordering for chapters and scenes
- Show per-scene status such as reviewed, rewritten, unresolved, or needs worldbuilding
- Let exports assemble a full manuscript from selected scenes

Why this matters:

- `novelWriter` and `Manuskript` both feel powerful because they help authors shape structure, not just inspect text
- this is the clearest gap between Novel Assistant and mature fiction-writing tools

## Priority 2: Story-RAG Control

Goal: make retrieval more predictable, tunable, and trustworthy.

- Add per-source toggles for character, worldbuilding, style, and chapter context
- Add retrieval controls such as `top-k` and minimum similarity threshold
- Add task-specific retrieval presets for behavior review, dialogue review, rewrite, and world conflict checks
- Show expected-but-missed context when a likely source was not retrieved
- Add source weighting or priority so critical canon files are harder to ignore

Why this matters:

- `AnythingLLM` style workspace behavior and clear citations make systems feel dependable
- controllable retrieval is one of the most important differences between a clever demo and a serious writing tool

## Priority 3: Manuscript Build and Delivery

Goal: export writing projects in formats closer to real author workflows.

- Build full-manuscript export across chapters and scenes
- Export to multiple targets such as `Markdown`, `HTML`, `DOCX`, and `PDF`
- Offer optional appendices for review notes, rewrite summaries, and tracker state
- Support chapter bundles filtered by draft state or scene tags
- Add shareable review packets for editors or beta readers

Why this matters:

- mature writing tools do not stop at editing; they help produce the manuscript you actually send out

## Priority 4: Multi-Project Safety

Goal: make the app safe for heavier long-term usage.

- Add project / workspace isolation instead of a single `data/` root
- Add per-project settings and independent vector stores
- Add project import / export and migration helpers
- Add safer restore flows with preview before overwrite
- Add automatic snapshot rotation and restore history

Why this matters:

- once writers trust a tool with months of story work, project safety becomes as important as model quality

## Priority 5: Mature App Surface

Goal: reduce friction for adoption and maintenance.

- Add authenticated mode or optional local access protection
- Add richer screenshots and onboarding docs
- Add package / installer paths beyond `go run` and Docker
- Add metrics or debug views for index size, retrieval quality, and model latency
- Add compatibility notes for Windows, macOS, Linux, and containerized setups

## Suggested Build Order

If development continues in sequence, this is the recommended order:

1. Scene hierarchy and scene board
2. Retrieval controls and source weighting
3. Full manuscript export
4. Multi-project workspace support
5. Optional local auth and stronger operations tooling

## References

The roadmap direction is especially informed by the strengths of:

- `novelWriter` for chapters, scenes, structure, and manuscript build
- `Manuskript` for outlines, index cards, and story organization
- `AnythingLLM` for workspace and retrieval ergonomics
