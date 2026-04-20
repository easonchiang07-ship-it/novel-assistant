## Issue #22 Plan

- Add an Ollama HTTP health check to `docker-compose.yml`.
- Update `app` so it waits for `ollama` to become healthy before starting.
- Update `ollama-init` so model pull starts only after `ollama` is healthy.
- Refresh English and Traditional Chinese Docker Compose docs to mention the readiness gate.
