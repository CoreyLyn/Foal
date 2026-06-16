# Foal

Foal is a safe, preview-first cleanup CLI for Windows.

It is designed for Windows developers and power users who want cleanup, uninstall review, disk analysis, and system snapshots without handing a tool permission to make unexplained destructive changes.

## Design Principles

- Preview first: cleanup candidates should be inspectable before execution.
- Recycle Bin by default: confirmed cleanup should use the Windows Recycle Bin, not permanent deletion.
- Conservative defaults: default cleanup rules should be easy to explain and low-disagreement.
- Windows-native safety: protected paths, reparse points, permissions, package managers, and installer ecosystems are first-class design concerns.
- JSON contracts first: human output can be friendly, but stable JSON output is the automation and TUI contract.
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

`foal clean` requires either `--dry-run` or `--execute`; `--dry-run` previews default candidates and reports exactly seven built-in skipped-by-default Opportunity categories: idle user-temp entries as `user_temp`, plus the current user's fixed CrashDumps, Windows Error Reporting, Explorer thumbnail cache, INetCache, D3D shader cache, and NVIDIA DX cache roots as `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache`, `d3d_shader_cache`, and `nvidia_dx_cache` when they exist. Observed opportunity bytes stay separate from `Potential space`. Browser review is gated by Chrome/Edge running-application detection: running browsers are reported as Running application skips, unknown process state is a recoverable diagnostic, and browser cache paths/bytes/opportunities remain excluded until cache measurement is explicitly implemented. The Recycle Bin is permanently excluded, developer-tool caches remain Review suggestions, and administrator-only caches such as SoftwareDistribution and Delivery Optimization are communicated only as permission boundaries without automatic elevation. `--execute` does not run opportunity discovery or browser running-application detection and confirms Recycle Bin cleanup only for freshly scanned, validated Foal-owned temp sandbox candidates. Docs and verification should prefer non-destructive examples such as `foal clean --dry-run --json`.

`foal analyze --json <path>` returns read-only directory insight with totals, top children, skipped entries, and elapsed time. `foal status --json` returns a read-only snapshot with disk capacity, OS runtime, Foal command state, elapsed time, and structured `skipped` / `errors` arrays for automation consumers.

`foal history --json` reads Foal operation history and reports recent sessions or structured history errors. `foal uninstall --json` is preview-only: it reports registry-discovered applications, installed-application footprint evidence as possible leftovers, orphaned residue as low-confidence review evidence, shared-state concerns, unknown state, skipped discovery providers, JSON `review_sections`, and an execution object whose actions are empty.

### Protection Rules

Foal loads optional user-defined Protection rules from `%APPDATA%\Foal\protection.txt`. Set `FOAL_PROTECTION_FILE` to select a different file. Each non-empty, non-comment line is one absolute local path; comments begin with `#`. UNC paths, relative paths, and paths containing 8.3 short-name segments are invalid.

A valid entry protects that path and its entire subtree using normalized, case-insensitive, path-component-aware matching. Protection rules are deny-only: they can suppress default candidates and path-backed review-only discoveries, but can never add or authorize cleanup. Protected user-temp opportunities and Review suggestions with a resolved protected cache path are removed before totals, JSON, human output, the Clean TUI, detailed candidate lists, and history projection. Suggestions without a resolved cache path are not inferred from command text.

Invalid lines are skipped with structured Protection diagnostics. A missing default file means no user-defined rules; a selected override that cannot be loaded, or a selected file with invalid UTF-8, fails the Clean operation closed before scanning or execution.

### Interactive TUI

Running `foal` (or the `fo` alias) with no arguments in an interactive terminal opens a read-only TUI: a main menu over the implemented commands, a clean preview browser, and read-only viewers for uninstall, status, and history. The Clean TUI shows default candidates and the same categorized skipped-by-default opportunities under its existing filters, keeps observed opportunity bytes distinct from `Potential space`, and supports cancellable reloads. Browsing records no history sessions, writes no detailed candidate list, and offers no cleanup selection or execution action. Scripts, pipes, and `--json` callers are unaffected and keep the deterministic help/command output.

## Scope

Foal is inspired by tools like Mole, but it is not "Mole for Windows". The roadmap is ordered by Windows risk and Foal's safety model rather than feature parity.

- `clean`: conservative Foal-owned temp sandbox rule, preview-first output, Recycle Bin-only execution after explicit `--execute`.
- `uninstall`: preview-only application review for registry-discovered installed applications, their high-confidence footprint evidence, and lower-confidence orphaned residue review clues. Foal does not execute uninstallers, stop processes, or delete leftovers.
- `analyze`: read-only, JSON-first directory insight.
- `status`: read-only system snapshot.
- `history`: JSON-first record of prior Foal operations.
- `optimize`: future read-only health checks and recommendations; not current implementation scope.

The TUI is a review and navigation surface over shared read models. Its Clean view displays the same `user_temp`, `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache`, `d3d_shader_cache`, and `nvidia_dx_cache` Opportunity categories and permission-boundary notices without writing history or detailed lists. It does not duplicate deletion, uninstall, or path-safety logic.
