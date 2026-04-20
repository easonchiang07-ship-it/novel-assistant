## Issue #32 Plan

- Add an `internal/consistency` package that sends one preflight LLM call and parses JSON conflicts.
- Run the consistency precheck after RAG references are assembled but before the main check/rewrite generation starts.
- Emit a new SSE `conflict` event without blocking the downstream generation flow.
- Show conflict warnings in the review UI before the streamed result content.
- Cover JSON parsing and SSE ordering with tests.
