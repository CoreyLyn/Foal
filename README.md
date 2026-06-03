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

## Command Shape

The planned command surface is:

```powershell
foal analyze
foal clean --dry-run
foal status --json
foal history
foal uninstall
```

`foal status --json` returns a read-only snapshot with disk capacity, OS runtime, Foal command state, elapsed time, and structured `skipped` / `errors` arrays for automation consumers.

## Scope

Foal is inspired by tools like Mole, but it is not "Mole for Windows". The roadmap is ordered by Windows risk and Foal's safety model rather than feature parity.

- `clean`: conservative rules, preview-first output, Recycle Bin-only execution by default.
- `uninstall`: preview-only for now; execution requires a future dedicated design.
- `analyze`: read-only, JSON-first directory insight.
- `status`: read-only system snapshot.
- `optimize`: future read-only health checks and recommendations; not current implementation scope.

Future TUI work should be a review and navigation surface over shared read models. It should not duplicate deletion, uninstall, or path-safety logic.
