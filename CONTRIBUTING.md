# Contributing

Thanks for helping improve Novel Assistant.

## Before You Start

- Open the `novel-assistant` folder directly in your editor.
- Use Go `1.21+`.
- Install Ollama locally if you want to verify embedding or review flows end to end.

## Development Workflow

1. Start from a GitHub issue whenever practical.
2. Create a dedicated branch for that issue instead of committing directly to `master`.
3. Keep changes focused. Separate refactors from feature work when possible.
4. Run formatting, tests, and build checks before opening a pull request.
5. Update docs when behavior, setup, or public workflows change.
6. Merge through a reviewed pull request, then close the issue.

Detailed conventions live in [docs/DEVELOPMENT_WORKFLOW.md](docs/DEVELOPMENT_WORKFLOW.md).

Recommended branch names:

- `feature/issue-<number>-short-name`
- `fix/issue-<number>-short-name`
- `docs/issue-<number>-short-name`
- `chore/issue-<number>-short-name`

## Useful Commands

On Windows PowerShell:

```powershell
./scripts/dev.ps1 fmt
./scripts/dev.ps1 test
./scripts/dev.ps1 build
./scripts/dev.ps1 run
```

On any shell with Go installed:

```bash
gofmt -w ./cmd ./internal
go test ./...
go build ./...
go run ./cmd
```

## Pull Request Expectations

- Link the issue being closed or advanced.
- Explain the user-facing problem.
- Summarize the chosen solution and tradeoffs.
- Note any migration, data-safety, or backward-compatibility concerns.
- Include test coverage for new parsing, validation, or review behavior when practical.
- If UI changes are visible, include a screenshot or short recording.

## Scope Guidelines

Good first contributions:

- Parser improvements for character, style, or worldbuilding files
- Better validation and error messages
- Documentation and onboarding improvements
- Additional tests for review flows and trackers

Larger changes should usually open an issue first:

- Major data format changes
- Replacing Ollama integration behavior
- New workflow pages or storage models
- Authentication, sync, or cloud features
