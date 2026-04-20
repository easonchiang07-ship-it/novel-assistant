# Development Workflow

This repository now follows an issue-driven branch and pull request workflow.

The goal is to keep feature scope clear, make reviews easier, and leave a clean trail from idea to shipped change.

## Standard Flow

1. Start from an existing GitHub issue.
2. Create a dedicated branch for that issue.
3. Implement the smallest complete slice that closes the issue.
4. Run formatting, tests, and build checks locally.
5. Open a pull request instead of pushing directly to `master`.
6. Review for behavior changes, regressions, missing tests, and documentation impact.
7. Merge only after verification is complete.
8. Close the issue after the PR lands.

Direct pushes to `master` should be treated as exceptions for urgent maintenance only.

## Branch Naming

Use one of these formats:

- `feature/issue-<number>-short-name`
- `fix/issue-<number>-short-name`
- `docs/issue-<number>-short-name`
- `chore/issue-<number>-short-name`

Examples:

- `feature/issue-2-scene-board`
- `fix/issue-4-rag-threshold-validation`
- `docs/issue-14-release-checklist`

## Commit Guidance

- Keep commits focused and intentional.
- Prefer one logical change per commit.
- Use clear imperative commit messages.

Examples:

- `Add scene-aware chapter editing workflow`
- `Fix chapter overview foreshadow counts`
- `Document PR workflow for issue-driven development`

## Pull Request Expectations

Each PR should include:

- the issue it addresses
- a short problem statement
- the chosen solution
- verification steps and results
- screenshots or recordings for visible UI changes
- follow-up work or known limitations, if any

## Review Checklist

Reviewers should focus on:

- correctness and regression risk
- data safety and persistence behavior
- backward compatibility for existing Markdown assets
- test coverage for new parser, storage, or workflow behavior
- documentation updates when user-facing behavior changes

## Minimum Verification

Before opening a PR, run:

```bash
gofmt -w ./cmd ./internal
go test ./...
go build ./...
```

If the change affects UI flows, also do a manual smoke test for the changed page.

## Release Documentation Rule

If a PR is part of a public release, or it changes a user-visible workflow that will need refreshed screenshots, check [docs/RELEASE_ASSET_CHECKLIST.md](docs/RELEASE_ASSET_CHECKLIST.md) before the release is finalized.

## Review Notes for Agents

When Codex, Claude, or another coding agent works on an issue:

- work on an issue branch, not directly on `master`
- keep the write scope limited to the issue
- summarize findings before claiming completion
- call out residual risk if tests or manual verification are missing
- prefer adding or updating tests when behavior changes

## Issue Closing Rule

Close the GitHub issue only after:

- the implementation is merged
- the PR description documents verification
- any important follow-up work has been split into separate issues
