# Windows Temp is a machine-wide exact-selection category

## Status

Proposed.

## Context

`%SystemRoot%\Temp` is the machine-shared analogue of the per-user temp directory Foal already cleans (`user_temp`): the OS and services write scratch data there, aged entries are conventionally reclaimable, and a real-machine audit found 1.7 GB (service diagnostics, `wct*.tmp`, months-old logs). Foal's current model cannot represent it for two reasons: PathSafe rejects the entire `C:\Windows` tree, and the directory affects all users of the machine.

ADR 0030 already opened the machine-wide category class with three compensating controls (exact-selection authorization, service-aware gating where attributable, non-elevated fail-closed). Default ACLs mean a non-elevated Foal can delete only a subset of entries; the remainder are per-item skips.

## Decision

Register one machine-wide, exact-selection-only category `windows-temp`:

- Resolve exactly `%SystemRoot%\Temp` from the `SystemRoot` environment variable; invalid values are silent absence. Add a narrow category-owned PathSafe carve-out for exactly that subtree — the Windows-tree rejection stays in force for every other path, and Protection rules still apply inside the carve-out.
- Direct children only, root never a candidate, reparse points never candidates or traversed. A directory child is a single candidate covering its subtree (`user_temp` semantics).
- 14-day latest-observed-modification stability window; unknown or future timestamps exclude the child fail closed.
- Exact-selection-only per ADR 0030: excluded from `all`, group tokens, and TUI Select All; preview carries a path-free machine-wide impact notice.
- Non-elevated fail-closed: access-denied enumeration skips the category; access-denied deletion is a per-item record. No elevation, no ACL changes, no widening of the servicing helper.
- Planned action `move_to_recycle_bin` with capacity pre-checks; never permanent.
- Fresh re-resolution and per-item revalidation immediately before mutation.

## Consequences

- Foal can reclaim the user-deletable share of system temp (partial reclaim is expected and honest; skips disclose the rest).
- The PathSafe carve-out is the first Foal-owned deletion surface under `%SystemRoot%`; it is deliberately expressed as an exact-subtree exception owned by this category so future categories cannot inherit it silently.
- No service attribution is attempted; the age window plus per-item isolation is the concurrency defence, accepting that a service may recreate entries.
- `Prefetch`, `Logs`, CBS logs, and `SoftwareDistribution` remain out of scope; each needs separate evidence (SoftwareDistribution has its own proposed ADR 0033).
