# Clean browser cache discovery requires running application detection

ADR-0006 deliberately excluded browser caches until Foal had real running-application detection. Foal now unlocks a narrow first slice: `foal clean --dry-run` and the read-only Clean TUI may report Google Chrome and Microsoft Edge Browser cache opportunities only after a three-state running check confirms the browser is idle before and after inspection.

The first browser cache scope is intentionally small: each supported browser is reported as one skipped-by-default opportunity covering only ordinary user profiles and only the regenerating `Cache`, `Code Cache`, and `GPUCache` directories. Foal reads the browser's `Local State` profile catalog rather than guessing profile directories, silently treats a missing current-user `User Data` root as absent, and treats an existing but unreadable or invalid profile catalog as unknown. Any matching browser process makes the whole browser running; Foal does not infer per-profile idleness from process command lines. If the browser becomes running or unknown during inspection, if any existing recognized cache directory cannot be inspected completely, or if any profile cache path is protected by Protection rules, Foal discards the browser's measured result instead of presenting a partial total.

Clean's human report may adopt Mole-inspired Report categories such as `System`, `User essentials`, `Browsers`, `Developer tools`, `Project artifacts`, `Protection`, and `Summary`, but those categories are presentation groupings only. They can colocate opportunities, running skips, diagnostics, clues, and suggestions under user-recognizable headings without changing JSON status, execution eligibility, or contribution to Potential space. Browser cache bytes remain skipped-by-default review data and are never default candidates.

## Considered options

- **Scan browser caches without process detection** - rejected: this is the exact unsafe case ADR-0006 blocked.
- **Treat unknown process state as idle** - rejected: a failed process check would become a cleanup signal.
- **Report partial browser totals when one profile or cache directory fails** - rejected: the browser summary would look complete while omitting data.
- **Copy Mole's top-level cleanup semantics** - rejected: Foal may borrow grouped presentation, but not Mole's default cleanup meaning.

## Consequences

This decision adds a new browser-summary shape to Clean's review model and JSON contract, so implementation must test JSON, human output, TUI, detailed review data, history privacy, Protection rules suppression, incomplete inspection side channels, and execute-path exclusion together. Future browser additions start from the same requirements: explicit running detection, narrow regenerating cache paths, complete-or-discard measurement, and no contribution to Potential space.
