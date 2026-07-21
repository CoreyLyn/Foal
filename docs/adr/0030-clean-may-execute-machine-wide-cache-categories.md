# Clean may execute machine-wide cache categories

A real-machine disk audit surfaced multi-GB regenerable caches under machine-shared roots (`C:\ProgramData\LGHUB\cache`, `C:\ProgramData\Thunder Network\XLLiveUD\Download`) that Foal's current-user-scoped Clean model could not even represent. We decided to open a **Machine-wide cache category** class that is directly executable — not observation-only — under three compensating controls, because scope (who is affected) is the only new risk dimension: the evidence chain, Protection, fresh-scan validation, and non-elevation discipline all apply unchanged.

## Decision

1. **Exact-selection authorization** (NVIDIA completed-download-task precedent): machine-wide categories are permanently excluded from `all`, every group token, and TUI Select All; CLI requires the literal category name, TUI rows start unselected, and previews carry a path-free "affects all users of this machine" impact notice. No new CLI flag.
2. **Service-aware idle gating**: Running application detection is extended so a registered logical application may declare exact Windows service names; services are queried read-only via SCM, non-stopped means `running`, query failure means `unknown`. Foal never starts, stops, or reconfigures services.
3. **Non-elevated fail-closed**: unreadable or undeletable shared paths are skips; machine-wide scope never justifies elevation or ACL changes (reaffirms ADR 0019's no-elevation row).

First categories: `lghub-cache` (64-hex content-addressed blobs only, Recycle Bin — official troubleshooting deletes the directory but that is not a removal contract) and `thunder-update-download` (direct children with a 30-day latest-observed-modification stability window because update-package consumption state is externally unreadable; Recycle Bin permanently).

## Considered options

- **Read-only observation slice first** (mirroring how skipped-by-default discovery preceded ADR 0008 opt-in execution) — recommended by analysis, rejected by product decision: the measured value is already concrete and the compensating controls above were judged sufficient without an intermediate slice.
- **Dedicated `--allow-machine-wide` flag** (à la `--allow-servicing`) — rejected: servicing's flag guards delegation to an unenumerable system mutation set; machine-wide categories remain ordinary path-backed Foal deletions, so exact selection expresses the scope difference proportionately.
- **Ordinary group-token opt-in** — rejected: affecting other local users must never ride along on `app-caches`/`dev-caches` convenience.

## Consequences

- The category class ships with Recycle Bin actions only; any Permanent upgrade is a separate per-category evidence decision.
- GoogleUpdater's `crx_cache` stays excluded even though it looks similar: it lives under `Program Files (x86)` (pathsafe-rejected tree) and is an active differential-update working set, not dead cache (see ADR 0019 additions).
- Multi-user authorization semantics beyond "exact selection by the acting user" are deliberately not modeled; revisiting them is required before any machine-wide category could ever join a group token.
