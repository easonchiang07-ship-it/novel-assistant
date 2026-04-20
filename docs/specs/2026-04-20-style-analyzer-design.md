# Style Analyzer Design

## Summary

Issue #33 adds two related but independent capabilities:

1. Style Analyzer: analyze pasted prose with the LLM and return structured style traits.
2. Style Preset: inject a hardcoded rewrite style constraint into rewrite prompts.

To keep existing style guide Markdown files unchanged, analyzed metadata is stored separately and merged into the in-memory `StyleGuide` model at load time.

## Goals

- Add `POST /api/styles/analyze` to return structured JSON style analysis.
- Let the styles page analyze arbitrary pasted text and apply the result to an existing style guide.
- Add optional `style_preset` to rewrite requests.
- Inject preset-specific constraints only into rewrite stream prompts.

## Non-Goals

- No style preset CRUD.
- No style preset support for check stream.
- No changes to the authored Markdown format in `data/style/*.md`.
- No multi-preset blending.

## Data Model

`internal/profile.StyleGuide` gains an optional `Analysis *StyleAnalysis`.

`StyleAnalysis` shape:

- `dialogue_ratio`
- `sensory_freq`
- `avg_sentence_len`
- `tone`
- `summary`

Persisted files live under `data/style/.analysis/<style-name>.json`.

This keeps authored Markdown and machine-generated metadata isolated while preserving backward compatibility.

## Backend Flow

### Style Analyzer

`POST /api/styles/analyze`

- Request: `{ "text": "..." }`
- Validate non-empty input.
- Call a new checker method that asks the LLM for strict JSON.
- Parse the response into `profile.StyleAnalysis`.
- Return `400` when the LLM output cannot be parsed into the expected JSON shape.

### Apply Analysis

`POST /api/styles/:name/analysis`

- Validate the target style exists.
- Bind request JSON into `StyleAnalysis`.
- Save JSON metadata file under the hidden analysis directory.
- Reload profiles so the updated analysis appears immediately in the UI.

### Rewrite Style Preset

`rewriteRequest` gains `StylePreset string`.

The server keeps a hardcoded preset map:

- `cold_hard`
- `light_novel`
- `epic`

When `style_preset` is present:

- validate the key
- append the matching style constraint block to the rewrite prompt
- keep existing selected style guides and retrieval context behavior unchanged

## Frontend Flow

### Styles Page

Add a new analyzer card with:

- textarea for sample text
- analyze button
- result panel
- style guide selector
- apply button

After analyze:

- render structured fields
- allow applying the result to the selected style guide
- refresh page state after a successful apply

Each style card also shows its saved analysis when available.

### Check Page Rewrite Panel

Add a preset selector above rewrite actions:

- no preset
- cold_hard
- light_novel
- epic

`runRewrite()` includes `style_preset` in the request body.

## Error Handling

- Empty analyze input returns `400`.
- Malformed analyze LLM output returns `400`.
- Unknown preset returns `400`.
- Applying analysis to a missing style returns `404`.
- Metadata save failures return `500`.

## Testing

- checker unit tests for style analysis JSON parsing
- profile manager tests for metadata load
- server handler tests for preset resolution and validation
- end-to-end tests for analyze API, apply-analysis persistence, and rewrite preset prompt injection

