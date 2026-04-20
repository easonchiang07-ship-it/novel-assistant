# Release Asset and Screenshot Checklist

Use this checklist before publishing a tagged release, release notes, or public-facing repo refresh.

The goal is consistency: each release should leave behind clear notes, refreshed screenshots, and updated README-facing assets.

## 1. Release Scope

- Confirm the version number and release date.
- Review merged PRs since the previous release.
- Group changes into user-facing highlights, infrastructure/developer changes, and known limitations.
- Make sure each shipped issue or PR is either closed or clearly tracked for follow-up.

## 2. Screenshot Checklist

- Capture the current app UI using real product flows, not placeholder layouts.
- Prefer screenshots that reflect the current navigation labels and page structure.
- Refresh screenshots when any of these change:
  - sidebar navigation
  - dashboard cards or counts
  - review/check page layout
  - chapter overview or history flow
  - settings, backup, diagnostics, or tracker pages
- Check that screenshots do not expose personal manuscript data, secrets, local filesystem details, or private prompts.
- Use consistent window size and browser chrome where possible.
- Verify text is readable at GitHub README width.
- Remove stale screenshots from the repo or release notes when they no longer match the product.

## 3. Release Notes Checklist

- Create or update release notes under `docs/releases/`.
- Include:
  - highlights
  - product changes
  - developer/deployment changes
  - validation summary
- Add both English and Traditional Chinese notes when the release is public-facing.
- Link to the companion localized release note file when both versions exist.
- Keep the top section skimmable: readers should understand the release in under a minute.

## 4. Docs Update Checklist

- Re-read `README.md` and `README.zh-TW.md` for stale feature lists.
- Update docs when the release changes:
  - setup steps
  - environment variables
  - workflow page names
  - export / backup / diagnostics capabilities
  - architecture or data layout assumptions
- Check whether `docs/ROADMAP*.md`, `docs/ARCHITECTURE.md`, or `docs/DEVELOPMENT_WORKFLOW.md` need follow-up edits.
- If a user-facing workflow changed, confirm that examples and screenshots still describe the current behavior.

## 5. README Asset Refresh Notes

- Refresh the README feature list when a release introduces a new visible workflow or page.
- Refresh badges, release links, and screenshot references if filenames or URLs changed.
- Avoid mixing screenshots from different product eras in the same README.
- If a screenshot is intentionally deferred, leave a short note in the PR or release issue so it does not get forgotten.
- Make sure the English and Traditional Chinese READMEs stay roughly aligned in structure and claims.

## 6. Final Release Gate

- `go test ./...`
- `go build ./...`
- Manual smoke test on the pages highlighted in screenshots or release notes
- Final proofread for:
  - version number consistency
  - broken links
  - outdated asset names
  - mismatched screenshot captions

## Output Expectation

A release should not be considered complete until:

- release notes are committed
- screenshots/assets are either refreshed or explicitly deferred
- README-facing claims match the shipped product
- documentation changes are part of the same release trail
