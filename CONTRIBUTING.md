# Contributing

Thanks for helping improve Novel Assistant.

## Before You Start

- Open the `novel-assistant` folder directly in your editor.
- Use Go `1.21+`.
- Install Ollama locally if you want to verify embedding or review flows end to end.

## Development Workflow

1. Create a branch for your change.
2. Run formatting and tests before opening a pull request.
3. Keep changes focused. Separate refactors from feature work when possible.
4. Update docs when behavior, setup, or public workflows change.

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

- Explain the user-facing problem.
- Summarize the chosen solution and tradeoffs.
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
