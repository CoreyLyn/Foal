# Plan: Windows system temp cleanup

## Status

Draft specification (P1). Not yet implemented. Governing decision: [ADR 0032](../adr/0032-windows-temp-is-a-machine-wide-exact-selection-category.md).

## Goal

Add one machine-wide, exact-selection-only Clean category that moves stale direct children of the shared system temp directory `%SystemRoot%\Temp` to the Recycle Bin, without elevation and without touching any other part of the Windows tree.

Proposed canonical category: `windows-temp`, label `Windows system temp`, report group `System`, eligibility `opt-in` with exact-selection-only policy (ADR 0030 class), running-application policy `not-applicable`, planned action `move_to_recycle_bin`.

## Evidence

- Local observation (2026-07-22): `C:\Windows\Temp` held 1.7 GB — `Whesvc\` 1.1 GB, `DiagOutputDir\` 429 MB, `wct*.tmp` 126 MB, plus assorted `*.log` files, some months old.
- The directory is the machine-shared analogue of the already-shipped `user_temp` category: OS and services write scratch data there and industry cleanup tools treat aged entries as reclaimable.
- Default ACLs let non-elevated users create files but frequently deny deleting entries owned by SYSTEM/services. Non-elevated fail-closed (skip on access denied) is therefore expected to reclaim only part of the observed bytes; that is acceptable and consistent with ADR 0030's compensating controls and the existing access-denied skip semantics.

## Functional contract

### Root resolution

- Resolve exactly `%SystemRoot%\Temp` via the `SystemRoot` environment variable (absolute, non-UNC, canonicalizable); fall back to nothing. Blank/relative/unsafe values are silent absence.
- The root itself is never a candidate. Only direct children participate; no recursion into children for candidate discovery (a directory child is one candidate covering its subtree, mirroring `user_temp`).
- PathSafe today rejects the whole `C:\Windows` tree. This category requires a narrow, category-owned carve-out for exactly the resolved `%SystemRoot%\Temp` subtree — never a general relaxation of the Windows-tree rule. The carve-out must be expressed so that Protection rules and dangerous-root rejection still apply to everything else under `%SystemRoot%`.

### Candidate rules

- Direct children only; reparse points are never candidates and never traversed.
- Stability window: a child is a candidate only when its latest observed modification (deep latest-write for directories, matching `user_temp` semantics) is at least 14 days old. Unknown or future timestamps exclude the child, fail closed.
- Active-use markers are excluded regardless of age: any child that cannot be opened for metadata, or is locked, is a per-item skip (`access_denied` / existing stable reasons), never a category failure.
- No filename allowlist: unlike application-scoped categories, system temp contents are arbitrary by design; the age window plus per-item fail-closed deletion is the safety mechanism (same posture as `user_temp`).

### Authorization and scope disclosure

- Exact-selection-only (ADR 0030): excluded from `all`, every group token, and TUI Select All. CLI requires the literal `--opt-in windows-temp`; the TUI row starts unselected. Preview carries a path-free "affects all users of this machine" impact notice.
- No elevation, ever: access-denied enumeration of the root is a whole-category recoverable skip; access-denied deletion of an item is a per-item failed/skipped record. Foal never edits ACLs, never uses an elevated helper for this category (the servicing helper's capability surface is not widened).
- Planned action `move_to_recycle_bin`; Recycle Bin capacity pre-checks apply. Never permanent deletion.

### Preview and execution

- Dry-run reports candidates with per-item bytes and idle days (mirroring `user_temp` presentation) plus the machine-wide impact notice.
- Execute performs fresh re-resolution and per-item revalidation (path identity under the resolved root, non-reparse, still past the window) immediately before each move.
- Protection rules deny-only per path.

## Non-goals

- No cleanup elsewhere in the Windows tree (`Prefetch`, `Logs`, `SoftwareDistribution` — the latter is a separate spec), no service queries, no elevation, no permanent deletion.

## Test intent

- Hermetic seam tests with injected env/FS/clock fakes: root resolution (unset/blank/relative/UNC `SystemRoot`), carve-out boundary (sibling `%SystemRoot%\Logs` still rejected), 14-day boundary (exactly-14-days is a candidate), future/unknown timestamps, reparse exclusion, per-item access-denied isolation, exact-selection-only exclusion from `all`/group tokens/TUI Select All, machine-wide impact notice presence, pre-mutation revalidation drift.
- Count-assertion sites: `execute_test.go` opt-in names, `tui_clean_eager_test.go` matrix, `category_catalog_test.go` locked lengths + Recycle Bin matrix + exact-selection-only set, catalog matrix comment, `CONTEXT.md`, `AGENTS.md`, README, `docs/plan/clean-deletion-policy.md`.
