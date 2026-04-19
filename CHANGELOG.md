# Changelog

All notable changes to this project will be documented in this file.

The format is inspired by Keep a Changelog.

## [Unreleased]

### Added

- Task-specific retrieval presets for behavior, dialogue, world, and rewrite workflows
- Per-task retrieval overrides from the review page; overrides are only sent when the user modifies the preset
- Preset editor in the settings page for all four task types

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
