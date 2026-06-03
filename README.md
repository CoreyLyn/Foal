# Foal

Foal is a safe, preview-first cleanup CLI for Windows.

It is designed for Windows developers and power users who want cleanup, uninstall review, disk analysis, and system snapshots without handing a tool permission to make unexplained destructive changes.

## Design Principles

- Preview first: cleanup candidates should be inspectable before execution.
- Recycle Bin by default: confirmed cleanup should use the Windows Recycle Bin, not permanent deletion.
- Conservative defaults: default cleanup rules should be easy to explain and low-disagreement.
- Windows-native safety: protected paths, reparse points, permissions, package managers, and installer ecosystems are first-class design concerns.
- JSON contracts first: human output can be friendly, but stable JSON output is the automation and future TUI contract.
- No automatic elevation: permission failures should be visible skipped items, not a reason to silently escalate.

## Implemented Command Shape

The current command surface is:

```powershell
foal --help
foal analyze --json .
foal clean --dry-run --json
foal clean --execute
foal status --json
foal history --json
foal uninstall --json
```

`foal clean` requires either `--dry-run` or `--execute`; `--dry-run` previews candidates and `--execute` confirms Recycle Bin cleanup for conservative Foal-owned temp sandbox entries. Docs and verification should prefer non-destructive examples such as `foal clean --dry-run --json`.

`foal analyze --json <path>` returns read-only directory insight with totals, top children, skipped entries, and elapsed time. `foal status --json` returns a read-only snapshot with disk capacity, OS runtime, Foal command state, elapsed time, and structured `skipped` / `errors` arrays for automation consumers.

`foal history --json` reads Foal operation history and reports recent sessions or structured history errors. `foal uninstall --json` is preview-only: it reports evidence sources, possible leftovers, shared-state concerns, unknown state, skipped discovery providers, and an execution object whose actions are empty.

## Scope

Foal is inspired by tools like Mole, but it is not "Mole for Windows". The roadmap is ordered by Windows risk and Foal's safety model rather than feature parity.

- `clean`: conservative Foal-owned temp sandbox rule, preview-first output, Recycle Bin-only execution after explicit `--execute`.
- `uninstall`: preview-only for now; Foal does not execute uninstallers, stop processes, or delete leftovers.
- `analyze`: read-only, JSON-first directory insight.
- `status`: read-only system snapshot.
- `history`: JSON-first record of prior Foal operations.
- `optimize`: future read-only health checks and recommendations; not current implementation scope.

Future TUI work should be a review and navigation surface over shared read models. It should not duplicate deletion, uninstall, or path-safety logic.
