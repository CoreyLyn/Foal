# Windows Update download cache requires idle update services

## Status

Proposed.

## Context

`%SystemRoot%\SoftwareDistribution\Download` stages Windows Update payloads. Applied payloads linger (210 MB observed locally on 2026-07-22; commonly gigabytes after feature updates), and clearing the directory is standard reset guidance because Windows re-downloads anything it still needs. The only real hazard is deleting content the update stack is concurrently using.

Conventional cleaners stop `wuauserv` first. Foal never starts, stops, or reconfigures services (ADR 0030 control 2 allows read-only SCM queries only), and never elevates for deletion. The directory also sits under the PathSafe-rejected Windows tree and affects all users, the same two obstacles ADR 0032 resolves for `windows-temp`.

## Decision

Register one machine-wide, exact-selection-only category `windows-update-download-cache`:

- Resolve exactly `%SystemRoot%\SoftwareDistribution\Download`; a narrow category-owned PathSafe carve-out covers exactly that subtree (`DataStore`, `ReportingEvents`, and the rest of the Windows tree stay rejected).
- Service-aware idle gate before discovery and again after measurement: exact services `wuauserv`, `bits`, `dosvc`, `UsoSvc` queried read-only via SCM; any non-`Stopped` state is `running`, query failure is `unknown`; running or unknown at either observation skips the whole category with stable path-free reason `windows_update_services_active`. Foal never mutates service state.
- Direct children only; root never a candidate; reparse points excluded; a directory child is one candidate covering its subtree.
- 30-day latest-observed-modification stability window (Thunder update-download precedent: consumption state is externally unreadable); unknown or future timestamps fail closed.
- Exact-selection-only per ADR 0030: excluded from `all`, group tokens, and TUI Select All; preview disclosure includes the machine-wide notice and that Windows re-downloads needed content.
- Non-elevated fail-closed; access-denied enumeration skips the category, access-denied deletion is a per-item record; partial reclaim is expected and disclosed.
- Planned action `move_to_recycle_bin` with capacity pre-checks; never permanent; never the servicing helper — this is an ordinary path-backed deletion category, distinct from `winsxs_component_store`.

## Consequences

- On machines with an active update stack the category usually skips whole; this is intentional. Users who want a guaranteed window may stop services themselves; Foal only observes.
- No update-metadata interpretation means Foal may skip payloads Windows itself considers expired; the 30-day window plus re-download recoverability keeps false candidates harmless.
- Together with ADR 0032 the exact-subtree carve-out pattern now has two instances; any third Windows-tree category must still argue its own carve-out rather than generalizing the mechanism.
- `DataStore` reset, service control, and DISM-based cleanup remain out of scope.
