# Architecture Overview

Novel Assistant is a local-first Go web application for long-form fiction review.

## Core Goals

- Keep story data on the author's machine
- Make narrative constraints visible and reviewable
- Combine structured story assets with local LLM assistance

## Main Components

### `internal/profile`

Loads Markdown-based project assets:

- `data/characters/*.md`
- `data/worldbuilding/*.md`
- `data/style/*.md`

These files are parsed into in-memory models for UI display and review prompts.

### `internal/embedder`

Wraps Ollama's embeddings API and converts project documents into vectors for retrieval.

### `internal/vectorstore`

Stores vectors locally in `data/store.json` and supports cosine-similarity search.

### `internal/checker`

Wraps Ollama's generation API for streaming review tasks:

- Behavior consistency
- Dialogue style consistency
- Writing style consistency

### `internal/tracker`

Maintains structured JSON-backed story state:

- Relationships
- Timeline events
- Foreshadowing

### `internal/server`

Serves HTML pages, static assets, ingest operations, trackers, export behavior, and SSE review streaming.

## Review Flow

1. Load profiles from Markdown
2. Reindex documents into the local vector store
3. Submit a chapter for review
4. Auto-resolve mentioned characters when not explicitly selected
5. Retrieve related local context from the vector store
6. Stream review output back to the browser over SSE

## Design Notes

- Structured story assets are intentionally simple, file-based, and easy to version.
- Retrieval is local and lightweight instead of relying on external vector databases.
- UI and data storage are optimized for solo creators first, then small-team collaboration.
