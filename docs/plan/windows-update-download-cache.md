# Plan: Windows Update download cache cleanup

## Status

Implemented. Governing decision: [ADR 0033](../adr/0033-windows-update-download-cache-requires-idle-update-services.md).

## Goal

Add one machine-wide, exact-selection-only Clean category that moves completed/stale Windows Update download payloads under `%SystemRoot%\SoftwareDistribution\Download` to the Recycle Bin, gated on Windows Update services being observably idle, without stopping services and without elevation.

Canonical category: `windows-update-download-cache`, label `Windows Update download cache`, report group `System`, eligibility `opt-in` with exact-selection-only policy (ADR 0030 class), running-application policy `distinctive-process-must-be-idle` backed by the service-aware idle gate (ADR 0030 control 2), planned action `move_to_recycle_bin`.

## Evidence

- Local observation (2026-07-22): `C:\Windows\SoftwareDistribution\Download` held 210 MB of already-applied update payloads.
- The directory is a download staging cache: Windows re-downloads anything it still needs; deleting stale content is the standard "reset Windows Update cache" guidance. The risk is exclusively concurrent use by the update stack, not data loss.
- Foal never stops or reconfigures services (ADR 0030), so safety must come from read-only service state plus a stability window — accepting frequent whole-category skips on machines where the update stack is active.

## Functional contract

### Root resolution

- Resolve exactly `%SystemRoot%\SoftwareDistribution\Download` from the `SystemRoot` environment variable; invalid values are silent absence. Missing directory is silent absence.
- Requires the same style of narrow, category-owned PathSafe carve-out as `windows-temp` (ADR 0032): exactly this subtree, never a general Windows-tree relaxation. Protection rules still apply inside.

### Service-aware idle gate

- Register a logical application identity declaring exact Windows service names `wuauserv` (Windows Update), `bits` (Background Intelligent Transfer), `dosvc` (Delivery Optimization), and `UsoSvc` (Update Session Orchestrator).
- Query each read-only via SCM before discovery and again after measurement (idle-before-and-after, mirroring process-gated categories). Any service not in the `Stopped` state means `running`; query failure means `unknown`. Running or unknown at either observation skips the whole category with a stable path-free reason (`windows_update_services_active`).
- Foal never starts, stops, or reconfigures any service, and never suggests stopping them in machine-mutating form (a textual hint that the user may stop services themselves is acceptable in the skip reason's guidance).

### Candidate rules

- Direct children of `Download` only; the root is never a candidate; reparse points are never candidates or traversed. A directory child is a single candidate covering its subtree.
- Stability window: 30 days latest observed modification (Thunder `thunder-update-download` precedent: consumption state is externally unreadable, so require a long externally-observable quiet period). Unknown or future timestamps exclude the child fail closed.
- No filename parsing or update-metadata interpretation: Foal never reads datastore/WU logs to attribute payload state.

### Authorization and scope disclosure

- Exact-selection-only: excluded from `all`, group tokens, TUI Select All; CLI requires literal `--opt-in windows-update-download-cache`; TUI row starts unselected; preview carries the machine-wide impact notice plus a "Windows re-downloads anything still needed" impact line.
- Non-elevated fail-closed: access-denied enumeration is a whole-category skip; access-denied deletion is a per-item record. Expect substantial per-item skips under default ACLs; partial reclaim is acceptable and disclosed.
- Planned action `move_to_recycle_bin` with capacity pre-checks; never permanent, never elevation, never the servicing helper (this is an ordinary path-backed deletion category, not `invoke_windows_servicing`).

### Preview and execution

- Dry-run runs the same service gate; active services yield the path-free skip, not candidates.
- Execute freshly re-resolves, re-checks services, and revalidates each item (identity under root, non-reparse, still past window) immediately before mutation. Post-measurement service re-check discards candidates when the update stack wakes mid-run.

## Non-goals

- No `SoftwareDistribution\DataStore` / `ReportingEvents` / catalog cleanup, no service control, no elevation, no DISM (`winsxs_component_store` owns component-store work), no permanent deletion.

## Test intent

- Hermetic seam tests with injected env/FS/clock/SCM fakes: root resolution and carve-out boundary (`DataStore` sibling still rejected), gate matrix (each service running/stopped/unknown × before/after), 30-day boundary (exactly-30-days is a candidate), future/unknown timestamps, reparse exclusion, per-item access-denied isolation, exact-selection-only exclusion, impact notices, pre-mutation revalidation drift.
- Count-assertion sites: `execute_test.go` opt-in names, `tui_clean_eager_test.go` matrix, `category_catalog_test.go` locked lengths + Recycle Bin matrix + exact-selection-only set, catalog matrix comment, `CONTEXT.md`, `AGENTS.md`, README, `docs/plan/clean-deletion-policy.md`.
