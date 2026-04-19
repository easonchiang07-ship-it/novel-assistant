# Changelog

All notable changes to this project will be documented in this file.

The format is inspired by Keep a Changelog.

## [Unreleased]

### Added

- Rewrite diff view for history entries
- Project settings for Ollama URL, models, and default review behavior
- Chapter bundle export with review, rewrite, timeline, foreshadow, and relationship context
- Data backup and restore from the settings page
- Dockerfile, docker-compose support, and `.env.example`
- API-level e2e coverage for chapter save, review, rewrite, writeback, and history export

### Changed

- Chapter review and rewrite history now retains input content for later diff comparison
- Config loading now supports environment variables and local `.env`

## [0.1.0] - 2026-04-18

### Added

- Initial local-first novel review dashboard
- Character behavior review
- Dialogue style review
- Writing-style review with `data/style/*.md` support
- Relationship, timeline, and foreshadow trackers
- Local RAG context display during chapter review
- Markdown report export
- Project scaffolding for open-source collaboration

### Changed

- Improved validation and persistence error handling
- Expanded README, contribution docs, and troubleshooting guidance
