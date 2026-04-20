## Issue #31 Plan

- Add an `internal/worldstate` package for snapshot storage and latest-before lookup.
- Load and save `worldstate.json` alongside the existing tracker JSON stores.
- Add `POST /api/chapters/:name/snapshot` and `GET /api/worldstate`.
- Use the latest snapshot before the current chapter as a high-priority system prefix in check and rewrite flows.
- Surface snapshot generation and the latest snapshot summary inline on the chapter overview page.
- Cover store boundary behavior, snapshot generation, and prompt injection with tests.
