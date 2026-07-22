# Plan: Analyze read-only disk browser

Design captured from a `/grill-with-docs` session. Glossary terms live in `CONTEXT.md`. ADR-0034 supersedes ADR-0025 for the TUI shape and explicit local-volume analysis.

## Goal

Turn the Analyze TUI into a responsive, read-only Windows disk browser inspired by a ranked disk-usage view while preserving Foal's boundaries:

- measurement only; no delete, cleanup selection, file open, preview, or implicit Purge;
- on-demand work only after entering a drive or directory;
- honest partial and incomplete states;
- shared Analyze safety and measurement code beneath CLI, JSON, and TUI;
- no History, elevation, process stopping, or changes to Clean/Purge authorization.

## Shipped behavior (now)

| Surface | Behavior |
| --- | --- |
| CLI/JSON | `foal analyze [path]`; omit path means CWD; one recursive result with totals, fixed Top 10 children, skipped entries, and elapsed time |
| Validation | Analyze-only `ValidateAnalyzeReadRoot`: explicit local fixed/removable volume roots and readable local directories (including Windows-managed trees) allowed; UNC, device, unsupported volume, reparse roots fail closed. Never weakens Clean/Purge `ValidateUserScanRoot` |
| Limit | CLI/JSON: one global 100,000-descendant ceiling (`status=incomplete` on limit/cancel). TUI browse: independent 100,000-descendant ceiling per direct directory child |
| Size | Logical file bytes from filesystem metadata (not allocated/physical; not free-space complement) |
| TUI | Read-only on-demand disk browser (ADR-0034): drive entry → ranked direct children → drill-down; session cache; no Command-viewer path edit as primary UX |
| Classification | Exact direct-child allowlist names only → `project_artifact_clue` (TUI compact label `artifact`); recursive size walks do not classify nested matches |
| Purge handoff | Copy-only `foal purge <root>` when current root independently passes Purge root validation and has a direct artifact clue; volume/system/profile roots never get unusable hints; never launches Purge |
| Mutation | None; no delete, cleanup-select, file-open, file-preview, elevation, process stop, or History write |

## Product model

### Drive entry

- Analyze TUI opens on a drive list, focused on `C:` when present; otherwise focus the first available local volume.
- Include local fixed drives and readable removable drives.
- Exclude mapped/network drives, optical drives, UNC roots, and device paths.
- Read only inexpensive drive letter, label, filesystem, total-space, and free-space metadata.
- An unavailable local volume remains visible as `Unavailable` but cannot be entered.
- Drive entry performs no recursive filesystem scan.
- `R` refreshes drive enumeration and metadata.

### Root validation

- Add an Analyze-specific validator or mode that accepts explicit local volume roots and readable local directories, including Windows and Program Files trees.
- Use it for explicit CLI/JSON paths and TUI browse locations.
- No-argument CLI remains CWD.
- Reparse points, UNC roots, device paths, malformed paths, and unsupported volume types fail closed.
- Do not reuse this allowance in Clean, Purge, Uninstall, Protection, or any deletion validator.

### On-demand browse locations

- Entering a drive or directory creates the active browse location.
- Enumerate every direct child, including hidden and system entries.
- Direct files become complete from their metadata and remain non-navigable.
- Direct directories enter the measurement queue.
- Do not prefetch sibling drives, sibling browse locations, or directories merely highlighted on drive entry.
- `Enter` navigates only into a readable directory.
- `Esc` returns from a directory to its parent, from a volume root to drive entry, and from drive entry to the Foal main menu.
- `Q` quits Foal. No Backspace navigation is exposed.

## Child measurement

Each direct directory child is measured recursively and independently:

- limit: 100,000 inspected descendants per child;
- concurrency: two measurement workers;
- default queue order: name;
- focus priority: moving the cursor to a queued directory promotes it to the next available slot;
- no preemption of an already running child;
- cooperative cancellation throughout traversal;
- reparse descendants are not traversed;
- logical bytes remain the size metric.

### States

| State | Meaning | Size and percentage |
| --- | --- | --- |
| `scanning` | Measurement is active | Current observed bytes; approximate observed percentage |
| `complete` | Traversal finished with no omissions | Exact logical bytes; exact percentage only when the browse-location total is complete |
| `partial` | Traversal ended but descendants were skipped due to permission/read errors | `>=` observed bytes; approximate observed percentage |
| `incomplete` | Descendant limit or cancellation stopped traversal | `>=` observed bytes; approximate observed percentage |
| `skipped` | The direct child itself cannot be measured or is a non-traversed reparse point | No bar or percentage; stable reason |

An approximate percentage is the row's share of bytes observed so far across the current location. It may rise or fall and must be labeled as approximate/observed. Never render an incomplete percentage as a guaranteed `>=` lower bound: the unknown denominator makes that claim invalid.

### Incremental observations

- The Analyze core owns traversal, limits, state classification, counts, and stable reasons.
- It emits path-scoped child observations to the TUI at a throttled cadence suitable for smooth rendering; the UI must not infer safety or completion from byte changes.
- Updates include child identity, kind, latest observed bytes, file/directory counts, skipped counts grouped by stable reason, classification, and terminal state when applicable.
- Do not stream or retain an unbounded descendant-path error list for the TUI.

## Ordering and cursor identity

- Sort rows by latest observed logical bytes descending; tie-break by name.
- Re-sort immediately as observations arrive.
- Bind selection to the child's canonical path, not its row index.
- When the selected child changes rank, move the cursor and viewport with that path.
- Files and directories share the same size ranking.

## Navigation lifecycle and cache

- Navigating away cancels all unfinished work for the old location; no hidden location keeps reading disk.
- Retain completed child summaries in an in-memory session cache.
- Returning to a location reuses completed summaries and queues only missing work.
- Results made incomplete by the hard descendant limit remain terminal; returning does not silently retry them.
- `R` cancels active work, discards the current location's cached summaries, re-enumerates it, and starts a fresh measurement.
- Cache is scoped to the current Analyze TUI session, is never persisted, and never feeds cleanup execution.

## Presentation

### Wide layout

Render a ranked table with these concepts:

`cursor | rank | bar | percentage | name | kind/state | logical size`

- Header: `Analyze`, current path, and independent volume total/free metadata.
- Drive free space is never treated as the complement of summed logical bytes.
- Show all direct children in a vertical viewport.
- Focused-detail footer: state, observed logical bytes, file/directory counts, skipped count, and aggregated stable reasons.
- Do not show descendant path lists by default.

### Responsive layout

- Wide: full columns and bar.
- Medium: shorten bar and hide kind before hiding safety state.
- Narrow: hide bar; retain cursor, name, size, and state.
- Avoid horizontal scrolling.
- Preserve size and safety state; truncate the middle of long names or paths first.

### Color and fallback

- Selected row: Foal pink accent.
- Normal bars and scanning spinner: blue/cyan.
- Partial/incomplete: yellow.
- Skipped/unavailable: gray.
- Do not use red for this read-only surface.
- Under `NO_COLOR`, preserve every distinction with text and symbols.

## Classification and Purge handoff

- Apply the existing exact final-component allowlist only to direct children of the current browse location: `node_modules`, `target`, `dist`, `build`, `.build`, `.next`, `__pycache__`.
- Render a compact `artifact` label; do not recursively classify descendants during a child size walk.
- Show copy-only `foal purge "<current-location>"` guidance only when the current location independently passes Purge root validation and at least one direct child has the clue.
- Never show an unusable Purge hint for a volume root or protected Windows tree.
- Never launch Purge or transfer selection state from Analyze.

## CLI and JSON compatibility

- Explicit local volume roots become valid: `foal analyze C:\` and its JSON form.
- No-argument CLI remains CWD.
- Preserve the existing one-result contract: `status`, `root`, `totals`, fixed Top 10 `top_children`, `skipped`, and `elapsed_ms`.
- Preserve the CLI's one global 100,000-descendant limit for this slice.
- Preserve logical-byte semantics.
- TUI-specific all-child and incremental state need not expand the existing JSON result; they must still come from shared Analyze core types and traversal rules.

## Safety invariants

- Analyze is read-only and does not write History.
- No automatic or requested elevation.
- Permission failures remain visible diagnostics, not prompts to run as administrator.
- Protection rules do not hide Analyze measurements and never authorize navigation or cleanup.
- Reparse points are visible but not traversed or entered.
- Cancellation stops future reads; it has no mutation or rollback semantics because Analyze never mutates.
- No percentages or bars imply reclaimable, cleanable, allocated, or physically occupied bytes.

## Implementation seams

- `internal/analyze`
  - Analyze-specific local-root validation.
  - Local drive enumeration abstraction for Windows with test seams.
  - Direct-child enumeration.
  - Independent child scanner with progress observations, state classification, limits, and cancellation.
  - Shared logical-byte and artifact-classification behavior.
- `internal/cli`
  - Dedicated Analyze TUI model replacing Analyze's generic viewer route.
  - Drive entry, navigation stack, two-worker scheduler, path-bound cursor, cache, viewport, responsive renderer, and key handling.
  - Existing Status and History viewers remain unchanged.
- `internal/core/pathsafe`
  - Keep deletion-oriented validators unchanged; add or expose an Analyze-only local read root policy without broadening mutation callers.


## Release-boundary note (#351)

Artifact clues, guarded Purge copy, negative read-only capabilities, protection non-intervention, and docs alignment are part of the shipped browser stack. Do not document unimplemented Open/Preview/delete affordances.

## Implementation order

1. Add tests for Analyze-only local volume/system-directory validation while proving Purge/Clean still reject their dangerous roots.
2. Add drive enumeration and direct-child scanner APIs with deterministic fake filesystem/volume seams.
3. Add child states, per-child limit, progress, two-worker scheduling, cancellation, and cache tests.
4. Replace Analyze's generic viewer route with drive entry and directory navigation.
5. Add realtime sorting with path-bound cursor and responsive rendering.
6. Add focused detail, artifact labels, and guarded Purge copy.
7. Run Analyze, CLI, TUI, pathsafe, and full repository tests; update help/README only for behavior actually implemented.

## Acceptance tests

- Drive entry focuses `C:` when available and never starts a scan before Enter.
- Network, optical, UNC, and device roots cannot be entered.
- Explicit CLI/JSON local volume root succeeds validation; Purge/Clean volume-root behavior is unchanged.
- Entering a location enumerates all direct children, including hidden/system entries.
- No more than two child scans run concurrently.
- Each child independently stops at 100,000 descendants and becomes incomplete.
- Permission skips create partial rather than complete state.
- Reparse children are visible and non-navigable.
- Rows re-sort on observations while selection remains attached to the same path.
- Focus changes prioritize queued work without preempting active scans.
- Escape cancels invisible work and follows the agreed directory/drive/menu hierarchy.
- Returning reuses completed cache entries and resumes missing work; refresh discards the location cache.
- Approximate percentages are labeled; incomplete percentage never uses `>=`.
- Narrow layouts retain name, size, and state without horizontal scrolling.
- `NO_COLOR` output retains all semantic distinctions.
- Analyze exposes no delete/open/preview action, writes no History, and requests no elevation.
- Purge copy appears only for a Purge-valid current location with a direct artifact clue.

## Out of scope

- Deletion, Recycle Bin, permanent cleanup, multi-select, file opening, and file preview.
- Allocated-size accounting, sparse/compressed-file physical usage, and hard-link deduplication.
- Network or UNC browsing, optical media, raw devices, and automatic elevation.
- Persistent scan cache, filesystem watching, background scanning of invisible locations, or cross-drive aggregation.
- Recursive artifact classification during size measurement.
- Changing Clean, Purge, Uninstall, Protection, or History semantics.

## Glossary pointers

See `CONTEXT.md`: **Analyze (directory insight)**, **Analysis root**, **Analyze drive entry**, **Analyze browse location**, **Analyze child measurement**, **Analyze logical bytes**, **Analyze TUI browser**, **Analyze browse-session cache**, **Analyze focused child detail**, **Analyze classification clue**, **Analyze purge handoff**, and **Analyze history non-recording**.
