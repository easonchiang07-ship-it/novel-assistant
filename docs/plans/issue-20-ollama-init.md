## Issue #20 Plan

- Add an `ollama-init` service to `docker-compose.yml`.
- Wait for Ollama to become reachable before any model pull attempt.
- Pull `llama3.2` and `nomic-embed-text` only when they are missing.
- Update English and Traditional Chinese README quick-start steps so Docker setup becomes `cp .env.example .env` then `docker compose up --build`.
- Mention that the first startup may take longer because required models are downloaded automatically.
