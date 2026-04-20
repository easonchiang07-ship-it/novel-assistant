# Scene-level Chunk RAG Design

## Summary

Issue #35 changes chapter retrieval granularity from whole-chapter embeddings to chunk-level embeddings. Chapter documents will be split into scene chunks when explicit `## Scene N` markers exist, and into paragraph chunks when they do not. Each chunk is embedded and stored independently so retrieval returns focused excerpts instead of entire chapters.

## Goals

- Replace whole-chapter vectorization with scene/paragraph chunk vectorization for chapter files only.
- Attach chunk metadata to chapter documents so the backend and frontend can describe exactly where a retrieved excerpt came from.
- Keep character, world, and style embeddings unchanged.
- Preserve current cosine-similarity retrieval flow while improving context precision.

## Non-Goals

- No reranking layer.
- No incremental indexing.
- No vectorstore storage migration script.
- No chunking changes for character, world, or style documents.

## Data Model

`internal/vectorstore.Document` will gain these optional metadata fields:

- `chapter_file`
- `chapter_index`
- `scene_index`
- `chunk_type`

Semantics:

- `chapter_file`: original chapter filename, for example `第03章.md`
- `chapter_index`: chapter number parsed from filename, for example `第03章.md -> 3`
- `scene_index`: 1-based chunk order within the chapter
- `chunk_type`: `scene` or `paragraph`

Fallback rule:

- If the filename does not contain a parseable chapter number, `chapter_index = 0`
- This includes names such as `prologue.md`, `番外.md`, or `test.md`

## Chunking Strategy

Add `chunkChapter(name string, content string) []vectorstore.Document`.

Behavior:

1. Trim the input content.
2. If the chapter is empty, return an empty slice.
3. Call existing `parseScenes(content)`.
4. If scenes exist:
   - one scene becomes one chunk
   - `chunk_type = "scene"`
   - `scene_index` is the scene order already derived from `parseScenes`
   - `content` is the scene body only
5. If no scenes exist:
   - split by `\n\n`
   - trim each paragraph
   - skip empty paragraphs
   - each paragraph becomes one chunk
   - `chunk_type = "paragraph"`
   - `scene_index` is the 1-based paragraph order

Document IDs:

- scene chunks: `chapter_<name>_scene_<i>`
- paragraph chunks: `chapter_<name>_para_<i>`

## Ingest Flow

`Server.Ingest()` keeps the current high-level structure:

- clear store
- index characters
- index worlds
- index styles
- index chapters

The chapter portion changes:

- read chapter file
- call `chunkChapter(file.Name(), string(content))`
- for each returned chunk:
  - embed the chunk content
  - store one `vectorstore.Document`

Expected effect:

- chapter document count becomes greater than chapter file count after ingest
- ingest becomes slower because each chapter may now issue multiple embed calls

## Retrieval Flow

Retrieval algorithm stays unchanged:

- same vector query generation
- same filtered cosine search
- same top-k and threshold logic

Only the retrieved chapter document payload becomes more precise because `doc.Content` is now a scene or paragraph chunk instead of the whole chapter.

## Metadata Propagation

Chunk metadata must flow through these layers:

1. `vectorstore.Document`
2. `vectorProfile`
3. `referenceSummary`
4. `sources` SSE event payload
5. frontend source card rendering

`referenceSummary` will expose:

- `chapter_file`
- `chapter_index`
- `scene_index`
- `chunk_type`

## Frontend Display

`web/templates/check.html` source cards will show chapter location text derived from metadata:

- scene chunk: `第 3 章・Scene 2`
- paragraph chunk: `第 3 章・段落 4`
- unknown chapter number fallback: `章節檔案 prologue.md・Scene 1` or `章節檔案 prologue.md・段落 2`

The rest of the source rendering remains unchanged.

## Error Handling

- Empty chapter content produces zero chunks and therefore zero stored chapter documents.
- Nonstandard chapter filenames do not fail ingest; they use `chapter_index = 0`.
- Existing `store.Clear()` behavior remains sufficient, so old whole-chapter entries are replaced automatically on the next ingest.

## Testing

### Unit Tests

Add tests for `chunkChapter()` covering:

- scene-based chunking
- paragraph-based chunking
- empty chapter returns empty slice
- filename chapter number parsing
- fallback filename parsing returns `chapter_index = 0`

### Integration / E2E Tests

Add tests covering:

- ingest stores more chapter documents than chapter file count when a chapter has multiple scenes or paragraphs
- `sources` SSE payload includes `chapter_index`, `scene_index`, and `chunk_type`

### Frontend / Template Tests

Add tests covering:

- source rendering includes `第 N 章・Scene M`
- source rendering includes paragraph labels when `chunk_type = "paragraph"`
- source rendering falls back to filename-based labels when `chapter_index = 0`

