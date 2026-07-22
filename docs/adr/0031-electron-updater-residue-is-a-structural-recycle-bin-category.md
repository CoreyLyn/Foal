# Electron updater residue is a structural Recycle Bin category

## Status

Proposed.

## Context

electron-builder's Windows auto-updater caches downloaded installers under per-app `%LOCALAPPDATA%\<app>-updater\` directories (`installer.exe`, `current.blockmap`, and a `pending\` staging child with the product setup executable and `update-info.json`). After an update installs, the payload is stale; electron-updater re-downloads and checksum-verifies before any future install, so the cache is rebuildable at the cost of one download. Machines with many Electron apps accumulate gigabytes of these payloads (2.5 GB across 25 directories observed locally on 2026-07-22).

The owning application for a given `*-updater` directory is arbitrary and not reliably attributable to a process name, so per-app idle gating (as used for editors and Obsidian) does not scale here. Meanwhile a freshly written `pending\` payload can be a staged "install on quit" update that a user expects to apply.

Directories that merely look like updaters (`QuarkUpdater`, `sparkle-updater`) may hold unrelated program data.

## Decision

Register one opt-in category `electron-updater-residue` with these boundaries:

- Enumerate only direct `%LOCALAPPDATA%` children whose name ends with `-updater`, and accept a directory only when its full two-level content matches an exact structural allowlist (`installer.exe`, `current.blockmap`, `pending\` with `update-info.json` / `current.blockmap` / ordinary `*.exe` only). Any unknown child, nested directory, or reparse point excludes the whole directory, fail closed.
- Candidates are the allowlisted files only; the updater directories themselves are never candidates.
- Skip a whole directory (not the category) when any allowlisted file was written within 24 hours, has a future or unknown timestamp, or has unreadable metadata (`electron_update_recent` / fail closed).
- Running-application policy is `shared-runtime-not-attributable`; Foal never enumerates or stops application processes for this category. The quiet window plus per-item failure isolation is the concurrency defence.
- Planned action `move_to_recycle_bin` (recoverable, like `nvidia_installer_cache`); Recycle Bin capacity pre-checks apply; never permanent deletion, never elevation.
- Selection: exact `electron-updater-residue`, `app-caches`, `all`, or Clean TUI selection; never `dev-caches` or `cli-agents`. Skipped by default.
- Full structural revalidation immediately before mutation; Protection rules deny-only per path.

## Consequences

- Foal reclaims multi-gigabyte stale installer payloads across arbitrary Electron apps with one rule, without per-app evidence gathering.
- A staged update deleted after its quiet window costs one re-download; electron-updater re-verifies checksums, so no corrupted-install risk is introduced.
- Structurally non-conforming updater directories (extra logs, custom layouts) are silently not cleaned; widening the allowlist requires new evidence, not pattern loosening.
- The category may false-skip whole directories on any unknown sibling file; this is intentional and safer than partial matching.
- Squirrel-style layouts (`SquirrelTemp`, `packages\*.nupkg`) remain out of scope pending separate evidence.
