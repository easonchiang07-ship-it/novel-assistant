# Roadmap

English | [繁體中文](ROADMAP.zh-TW.md)

This roadmap reflects the current product direction after chapter workflow, review history, rewrite diff, project settings, Docker support, and local backup / restore were added.

## Product Vision

A local AI writing studio where anyone — with no technical background — can install the tool, build their story world, generate a full draft, review it, and revise it into something they love. No cloud. No subscriptions. No coding.

The intended user journey:

```
Install → Build story world → Generate draft → Review → Revise → Export
```

## Current Focus

Novel Assistant is now strongest as a local-first fiction review workstation.

The next step is not adding more isolated features, but deepening three product layers:

- manuscript structure and writing flow
- controllable story-specific RAG behavior
- safer long-term project management

## Priority 0: First-Run Experience

Goal: make sure any user who clones the repo and runs `docker-compose up` has a working tool within minutes, with no manual steps.

- Auto-pull required Ollama models (`llama3.2`, `nomic-embed-text`) on first startup
- Add Ollama health check so the app waits for the model server before accepting requests
- Add a sample chapter to `data/chapters/` so the review page has something to try immediately
- Fix Docker quick-start docs to include `cp .env.example .env` as the first step

### Model selection by hardware tier

Users should never need to know a model name. The settings page should recommend the right model based on their hardware.

- Add a hardware tier selector in the settings page:
  - Entry (4GB VRAM) → `llama3.2:3b` — fast, basic quality
  - Standard (8GB VRAM) → `llama3.1:8b` — balanced (recommended default)
  - Advanced (24GB VRAM) → `gemma3:27b` — strong Traditional Chinese, high quality
  - Pro (48GB+ VRAM) → `llama3.3:70b` — near cloud-service quality
  - Custom → free text input for any Ollama model name
- Selecting a tier automatically updates `LLM_MODEL` and triggers `ollama pull` in the background
- Embed model (`nomic-embed-text`) is fixed and not exposed to users — it works well across all tiers

Why this matters:

- the first five minutes determine whether an open-source user keeps the tool or closes the tab
- all of this is tracked in Issues #20–24 and is the prerequisite for any growth
- users should not need to know what a quantized model is to get good output

## Priority 1: Guided Story World Setup

Goal: let users build their character profiles, worldbuilding notes, and style preferences through a guided Q&A interface — no Markdown knowledge required.

Currently, setting up a project requires manually writing formatted `.md` files with specific Chinese or English field headers. This is an invisible wall for non-technical users.

- Add a setup wizard page that walks users through story world creation step by step:
  - Story premise and genre
  - Characters (name, personality, core fear, speech style, etc.) via form fields
  - Worldbuilding (setting, rules, factions, locations) via free-form prompts
  - Writing style preferences (tone, pacing, perspective, forbidden patterns)
- System generates the correctly formatted `data/characters/`, `data/worldbuilding/`, and `data/style/` files automatically
- Users can edit generated files later if they want to go deeper, but never have to touch them to get started
- Add an "edit story world" page so users can update their setup without touching raw files

### Style guide sample passages

Every user has their own style. The style guide wizard must make it easy to capture that.

- Add a `sample_passages` field to the style guide format; `profile/manager.go` parses and passes it to generation and review prompts
- In the wizard's style step, prompt users explicitly:
  > "Paste 2–3 paragraphs you love — your own writing, or an author whose style you admire (be mindful of copyright). These examples directly shape the rhythm, vocabulary, and sentence structure of generated text."
- Show a preview of how the sample will be used so users understand the impact
- During generation and rewrite, inject sample passages as concrete few-shot examples rather than abstract style rules

Why this matters:

- abstract style rules ("short sentences, calm tone") produce inconsistent output; concrete examples let the LLM imitate directly
- this is the single highest-leverage input a user can provide to improve generation quality on a local model
- the entire review and generation pipeline depends on these asset files existing and being correctly formatted
- asking a non-technical user to write `# 角色：小明\n- 個性：...` by hand is a silent dealbreaker
- this is the most important onboarding step between "installed the tool" and "got useful output"

## Priority 3: Writing Structure

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

### Pronoun resolution and character detection accuracy (Issue #19)

Current string matching only finds characters when their name appears explicitly. Scenes written entirely in pronouns are silently skipped, making consistency checks incomplete.

**Phase 1 — Prompt-level guidance (low effort)**
- Add an instruction to `CheckBehaviorStream` telling the LLM to treat pronouns (他 / 她 / he / she) as potentially referring to the target character based on context
- Covers the majority of single-chapter review scenarios within llama3.2's 128K context window

**Phase 2 — Sliding window for long-form text**
- Split chapters exceeding the context window into overlapping chunks
- Inject a character summary header into each chunk so the LLM has cross-chunk character context
- Merge and deduplicate findings across chunks

Why this matters:

- without pronoun resolution, a character who is only referred to by pronoun throughout a scene produces zero behavior findings, making the check misleading rather than just incomplete

## Priority 4: Bilingual Support (Chinese + English)

Goal: make the tool usable for authors writing in English without requiring Chinese-format asset files.

- Support English-format character profiles alongside existing Chinese format:
  ```
  # Character: John
  - Personality: ...
  - Core fear: ...
  - Speech style: ...
  ```
- Support English-format style guides and worldbuilding files
- Detect chapter language and switch LLM prompt language accordingly (Chinese chapter → Chinese prompt, English chapter → English prompt)
- Translate UI labels and error messages to English; add a language toggle in settings

Why this matters:

- currently `profile/manager.go` hardcodes Chinese field headers; English asset files produce empty character data and completely broken review output
- bilingual support is the minimum viable step before the project can attract a non-Chinese-speaking community

## Priority 5: Chapter Version Control

Goal: give authors a safe, reversible history for every chapter they edit or rewrite.

- Track every saved version of a chapter file with a timestamp and source label (manual save, rewrite, writeback)
- Allow users to diff any two versions side by side
- Allow one-click restore to any previous version
- Optionally tag versions (e.g. "before structural rewrite", "beta reader draft")

Why this matters:

- currently rewriting a chapter and saving overwrites the previous version with no recovery path
- authors revising a long manuscript cannot afford to lose work; version control is a basic safety guarantee that any serious writing tool must provide

## Priority 6: Custom Check Types

Goal: let users define their own review dimensions beyond the four built-in types.

- Allow users to add custom check definitions in settings: a name, a prompt template, and an optional retrieval preset
- Custom checks appear alongside behavior / dialogue / world / style in the review page
- Built-in checks remain unchanged; custom checks are additive
- Allow export and import of check definitions for sharing between projects or users

Why this matters:

- different genres have different consistency concerns; a hard sci-fi author needs a physics-consistency check, a historical fiction author needs an anachronism check
- the current four check types reflect general fiction; they cannot cover every author's needs

## Priority 7: AI-Assisted Novel Generation

Goal: let users go from story concept to full draft manuscript using the same local-first setup.

This is the largest single capability expansion. It turns Novel Assistant from a review workstation into a full writing companion that can generate as well as critique.

### Phase 1 — Outline generation

- User provides character profiles, worldbuilding notes, style guide, and a brief story premise
- LLM generates a structured outline: chapter list, per-chapter synopsis, key scene nodes, and word count targets
- Outline is editable before generation begins; each chapter can be approved, adjusted, or regenerated independently

### Phase 2 — Chapter-by-chapter generation

- Each chapter is generated in sequence using:
  - the approved chapter synopsis and scene nodes
  - a rolling summary of previously generated chapters (compressed to fit context)
  - the full character and worldbuilding profiles via RAG
  - the selected style guide
- Output streams in real time via SSE, same as the existing review and rewrite flows
- After each chapter, the user can review, edit, or regenerate before continuing

### Phase 3 — Consistency loop

- After generation, the existing behavior, dialogue, world, and style review flows run automatically on each chapter
- Findings are surfaced as a post-generation report rather than blocking generation
- User can trigger targeted rewrite on flagged scenes before final export

### Architecture notes

- New `internal/generation` package handles outline state, chapter sequencing, and rolling summary management
- New `generation.html` page with a step-by-step wizard UI: setup → outline → generate → review → export
- Shares `internal/profile`, `internal/embedder`, `internal/checker`, and `internal/vectorstore` with existing flows
- Outline state is persisted in `data/generation/` so interrupted sessions can resume

### Dependencies

- Requires Priority 1 (scene hierarchy) to be complete first; generation targets scenes, not raw chapter files
- Works best after Priority 2 (Story-RAG Control) is stable so retrieval during generation is reliable

Why this matters:

- most local AI writing tools stop at chat or single-scene generation; chapter-by-chapter generation with cross-chapter memory is the missing piece
- keeping generation and review in the same tool means authors never need to copy-paste between apps to check consistency

## Priority 8: Manuscript Build and Delivery

Goal: export writing projects in formats closer to real author workflows.

- Build full-manuscript export across chapters and scenes
- Export to multiple targets such as `Markdown`, `HTML`, `DOCX`, and `PDF`
- Offer optional appendices for review notes, rewrite summaries, and tracker state
- Support chapter bundles filtered by draft state or scene tags
- Add shareable review packets for editors or beta readers

Why this matters:

- mature writing tools do not stop at editing; they help produce the manuscript you actually send out

## Priority 9: Multi-Project Safety

Goal: make the app safe for heavier long-term usage.

- Add project / workspace isolation instead of a single `data/` root
- Add per-project settings and independent vector stores
- Add project import / export and migration helpers
- Add safer restore flows with preview before overwrite
- Add automatic snapshot rotation and restore history

Why this matters:

- once writers trust a tool with months of story work, project safety becomes as important as model quality

## Priority 10: Review Quality Feedback (Local Only)

Goal: let users mark which review findings were useful, purely to improve their own local experience — no data ever leaves the device.

- Add thumbs-up / thumbs-down on individual review findings in the history page
- Store accepted / rejected labels locally alongside existing `reviewhistory` entries
- Use labels to surface per-check-type acceptance rate as a personal quality summary
- No export, no opt-in sharing, no telemetry of any kind — labels stay on the user's machine

Why this matters:

- the core promise of this project is that manuscripts never leave the user's computer; any data collection mechanism, even opt-in, risks eroding that trust
- local feedback labels still have value: users can see which check types consistently produce useful findings for their own writing, and tune their preset settings accordingly

## Priority 11: Mature App Surface

Goal: reduce friction for adoption and maintenance.

- Add authenticated mode or optional local access protection
- Add richer screenshots and onboarding docs
- Add package / installer paths beyond `go run` and Docker
- Add metrics or debug views for index size, retrieval quality, and model latency
- Add compatibility notes for Windows, macOS, Linux, and containerized setups

## Suggested Build Order

If development continues in sequence, this is the recommended order:

1. First-run experience (Issues #20–24)
2. Guided story world setup (wizard UI for characters, worldbuilding, style)
3. Scene hierarchy and scene board
4. Pronoun resolution and retrieval accuracy
5. Bilingual support (Chinese + English)
6. Chapter version control
7. Custom check types
8. AI-assisted novel generation (outline → chapter-by-chapter → consistency loop)
9. Full manuscript export
10. Multi-project workspace support
11. Review feedback collection (after sufficient user base)
12. Optional local auth and stronger operations tooling

## References

The roadmap direction is especially informed by the strengths of:

- `novelWriter` for chapters, scenes, structure, and manuscript build
- `Manuskript` for outlines, index cards, and story organization
- `AnythingLLM` for workspace and retrieval ergonomics
- `Sudowrite` and `NovelAI` for AI-assisted generation UX patterns
