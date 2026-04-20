# Issue #11 Plan: Backup Preview and Snapshot Rotation

## Goal

Make backup and restore safer by showing users what a snapshot contains before restore, limiting snapshot growth automatically, and making restore messaging clearer.

## Why

Raw restore is too risky once a project contains meaningful manuscript work.

Users need enough context to trust a restore action, and the app should not keep snapshots forever without any cleanup policy.

## Scope

1. Add snapshot metadata so each backup can be previewed without opening raw files.
2. Expose backup preview data to the settings page and restore flow.
3. Add a project-level retention setting for automatic snapshot rotation.
4. Create a safety snapshot before restore and report it clearly to the user.
5. Add regression tests for metadata generation, rotation, and restore safety behavior.

## Proposed Design

### Snapshot Manifest

Each backup directory includes a `.backup_manifest.json` file containing:

- snapshot name and creation time
- counts for chapters, characters, worldbuilding files, style guides, and JSON state files
- total file count
- a short sample list of chapter filenames

This allows the UI and API to show preview details without traversing the snapshot every time.

### Preview Flow

- `GET /api/backups` returns backup items plus preview metadata when available
- `GET /api/backups/:name/preview` returns one snapshot preview for focused UI refresh
- settings page shows the selected backup summary before restore

### Retention Policy

- add `backup_retention` to project settings
- default to a small safe cap
- after each new snapshot, delete oldest backups beyond the retention cap
- keep the rule simple: count-based retention only

### Restore Safety

- before restore, create an automatic safety snapshot of the current data
- return a clearer restore message that includes both the restored backup name and the new safety snapshot name
- reload in-memory data after restore as before

## Done When

- users can preview the selected snapshot before restoring
- backups rotate automatically according to project settings
- restore messaging explains what happened and where the safety snapshot is
- focused backend tests cover metadata, rotation, and restore behavior
