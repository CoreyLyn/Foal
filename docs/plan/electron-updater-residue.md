# Plan: Electron updater residue cleanup

## Status

Implemented. Governing decision: [ADR 0031](../adr/0031-electron-updater-residue-is-a-structural-recycle-bin-category.md).

## Goal

Add one opt-in Clean category that reclaims downloaded installer payloads left behind by electron-builder's Windows auto-updater (electron-updater / NSIS web-style updater) under per-app `%LOCALAPPDATA%\<app>-updater\` caches, without treating any application's data, configuration, or the updater directory itself as cache.

Canonical category: `electron-updater-residue`, label `Electron updater residue`, report group `Applications`, eligibility `opt-in`, running-application policy `not-applicable`, planned action `move_to_recycle_bin`.

## Evidence

- electron-builder's Windows updater caches downloaded installers in `%LOCALAPPDATA%\<cacheDirName>\` where `<cacheDirName>` is conventionally `<appName>-updater` (scoped packages flatten to `@org-app-updater`). After a successful install the payload is stale; electron-updater re-downloads and checksum-verifies before any future install, so removing the cache only costs a re-download.
- Observed local structure across 25 `*-updater` directories on the author's machine (2026-07-22), all matching the same signature:
  - direct `installer.exe` (the downloaded full installer; 130–300 MB each)
  - optional direct `current.blockmap` (differential-download metadata)
  - optional direct `pending\` child containing `<product setup>.exe`, `temp-*.exe`, `current.blockmap`, and small `update-info.json`
- Local aggregate at observation time: roughly 2.5 GB across recordly, termius, orca, obsidian, cherrystudio, tabby, apifox, quark, aionui and others.
- One directory (`orca-updater`) had files written the same morning — a live pending update. This motivates the quiet-window gate below.
- Directories named like updaters but NOT matching the structural signature exist (e.g. `QuarkUpdater`, `sparkle-updater`); the signature requirement excludes them unless they genuinely match.

## Functional contract

### Root resolution

- Resolve only the current user's `%LOCALAPPDATA%` (known-folder / env, consistent with existing Local-AppData-based categories). Missing base is silent absence.
- Enumerate only direct children of `%LOCALAPPDATA%` whose final name component ends with the exact lowercase suffix `-updater` (case-insensitive match on the suffix only). Non-directory or reparse-point entries are excluded.
- No registry queries, no process launching, no reading of any app's configuration, no other drives or locations.

### Structural signature (per matched directory)

A matched `*-updater` directory participates only when every direct child is on this allowlist (unknown children ⇒ the whole directory is silently excluded, fail closed):

- ordinary file `installer.exe`
- ordinary file `current.blockmap`
- directory `pending`

Inside `pending`, every direct child must be one of:

- ordinary file `update-info.json`
- ordinary file `current.blockmap`
- ordinary `*.exe` file (product setup or `temp-*.exe` staging)

Any reparse point, nested directory, or unknown filename anywhere in the two-level structure excludes that whole updater directory. Zero structural matches means an empty category; there is no fallback.

### Candidates

- Candidates are the individual allowlisted files (never the `*-updater` directory itself, never `pending` itself). Directories are left in place empty.
- `update-info.json` is included (it is meaningless without its payload), but it is a candidate only alongside its sibling payload — a `pending` containing only `update-info.json` yields no candidates.

### Quiet-window gate (per directory)

- If any allowlisted file in a matched directory has a last-write time within 24 hours of the injected current time, or in the future, or unknown, skip that whole directory (not the whole category) with a stable reason (`electron_update_recent`).
- Rationale: a fresh `pending\` payload is an "install on quit" staged update; deleting it is recoverable (re-download) but pointless and user-hostile. 24 hours comfortably exceeds any download window.
- Unreadable metadata fails closed for that directory.

### Running-application policy

- Owning applications are arbitrary and not attributable from the cache name; use the existing `shared-runtime-not-attributable` policy (as `electron-cache` does). Foal never enumerates or stops app processes for this category.
- The quiet-window gate is the concurrency defence: an updater actively downloading rewrites files inside the window and is skipped. A file locked by an in-flight installer fails deletion locally and is reported as a failed item without affecting siblings (existing shared Clean semantics).

### Preview, selection, and action

- Eligibility `opt-in`; skipped by default in Dry-run/Execute. Enabled via exact `--opt-in electron-updater-residue`, `--opt-in app-caches`, `--opt-in all`, or Clean TUI selection. Not a `dev-caches` or `cli-agents` member.
- Planned action `move_to_recycle_bin` (aligned with `nvidia_installer_cache`: verified completed installer downloads are recoverable via the Bin; no `--allow-permanent` needed). Recycle Bin capacity pre-checks apply as for other Recycle Bin work.
- Per-run fresh revalidation of the full structural signature immediately before mutation (category-owned identity validation), consistent with the "validate immediately before execution" engineering constraint.
- Protection rules apply deny-only per path and may suppress individual files or whole directories before totals.

## Non-goals

- No cleanup of app data, `%APPDATA%` config, Squirrel `SquirrelTemp`/`packages` layouts, or non-conforming updater directories (separate evidence required).
- No process detection, no elevation, no permanent deletion.

## Test intent

- Hermetic seam tests with injected env/base-dir/clock/FS fakes (pattern: `grok_build_update_residue` tests). Matrix: signature accept/reject per unknown child, reparse points, suffix matching, quiet-window boundary (exactly-24h is quiet), future/zero timestamps, pending-only-update-info case, per-directory vs per-category skip isolation, protection suppression, identity revalidation drift.
- Count-assertion sites to bump when registering the category: `execute_test.go` valid opt-in names, `tui_clean_eager_test.go` matrix, `category_catalog_test.go` locked lengths and Recycle Bin matrix, catalog matrix comment, `CONTEXT.md`, AGENTS.md/README policy lists, `docs/plan/clean-deletion-policy.md`.
