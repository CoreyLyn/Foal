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

`foal clean` requires either `--dry-run` or `--execute`; `--dry-run` previews default candidates and reports skipped-by-default Opportunity categories: idle user-temp entries as `user_temp`, the current user's fixed CrashDumps, Windows Error Reporting, Explorer thumbnail cache, INetCache, D3D shader cache, and NVIDIA DX cache roots as `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache`, `d3d_shader_cache`, and `nvidia_dx_cache` when they exist, Chrome and Edge `browser_cache` when the browser is idle before and after complete profile cache inspection, VS Code `vscode_cache` when Code is idle before and after inspection of the fixed regenerating cache roots under the current user's standard Roaming AppData `Code` directory, and Cursor `cursor_cache` when Cursor is idle before and after inspection of the same fixed allowlist under the standard Roaming AppData `Cursor` directory. Observed opportunity bytes stay separate from `Potential space`. Browser review is gated by Chrome/Edge running-application detection; Application cache review uses generic application idle-before-and-after detection with independent editor identities (`Code.exe` for VS Code, `Cursor.exe` for Cursor). Running apps are reported as Running application skips and unknown process state is a recoverable diagnostic. The Recycle Bin is permanently excluded, developer-tool caches remain Review suggestions by default (opt-in `playwright-browsers` reclaims only complete versioned Playwright browser installations; MCP profiles and hermetic `PLAYWRIGHT_BROWSERS_PATH=0` roots are never scanned; structured `puppeteer-browsers` reclaims allowlisted product platform-version installations under the env/default Puppeteer cache root; opt-in `electron-cache` reclaims the Electron download cache root from non-blank `electron_config_cache` or `%LOCALAPPDATA%\electron\Cache`; opt-in `jetbrains-ide-caches` reclaims only exact `caches`/`index` children under standard `%LOCALAPPDATA%\JetBrains` product-version roots for supported IntelliJ-platform IDEs (IntelliJ IDEA Ultimate/Community, PyCharm Professional/Community, WebStorm, PhpStorm, RubyMine, CLion, DataGrip, DataSpell, GoLand, RustRover, Aqua, MPS, Writerside; Rider deferred) with independent product idle gates), Application cache categories stay skipped by default until `--opt-in vscode_cache`, `--opt-in cursor_cache`, `--opt-in playwright-browsers`, `--opt-in puppeteer-browsers`, `--opt-in electron-cache`, `--opt-in dev-caches`, `--opt-in all`, or Clean TUI selection (selecting one editor never selects the other), and administrator-only caches such as SoftwareDistribution and Delivery Optimization are communicated only as permission boundaries without automatic elevation. `--execute` does not run opportunity discovery or browser/application running-application detection by default and confirms Recycle Bin cleanup only for freshly scanned, validated Foal-owned temp sandbox candidates (plus any explicitly opted-in categories). Docs and verification should prefer non-destructive examples such as `foal clean --dry-run --json`.

`foal analyze --json <path>` returns read-only directory insight with totals, top children, skipped entries, and elapsed time. `foal status --json` returns a read-only snapshot with disk capacity, OS runtime, Foal command state, elapsed time, and structured `skipped` / `errors` arrays for automation consumers.

`foal history --json` reads Foal operation history and reports recent sessions or structured history errors. `foal uninstall --json` is preview-only: it reports registry-discovered applications, installed-application footprint evidence as possible leftovers, orphaned residue as low-confidence review evidence, shared-state concerns, unknown state, skipped discovery providers, JSON `review_sections`, and an execution object whose actions are empty.

### Protection Rules

Foal loads optional user-defined Protection rules from `%APPDATA%\Foal\protection.txt`. Set `FOAL_PROTECTION_FILE` to select a different file. Each non-empty, non-comment line is one absolute local path; comments begin with `#`. UNC paths, relative paths, and paths containing 8.3 short-name segments are invalid.

A valid entry protects that path and its entire subtree using normalized, case-insensitive, path-component-aware matching. Protection rules are deny-only: they can suppress default candidates and path-backed review-only discoveries, but can never add or authorize cleanup. Protected user-temp opportunities and Review suggestions with a resolved protected cache path are removed before totals, JSON, human output, the Clean TUI, detailed candidate lists, and history projection. Suggestions without a resolved cache path are not inferred from command text.

Invalid lines are skipped with structured Protection diagnostics. A missing default file means no user-defined rules; a selected override that cannot be loaded, or a selected file with invalid UTF-8, fails the Clean operation closed before scanning or execution.

### Interactive TUI

Running `foal` (or the `fo` alias) with no arguments in an interactive terminal opens a TUI: a main menu, category-first Clean preview with explicitly confirmed Recycle Bin execution, and read-only viewers for uninstall, status, and history. Entering Clean immediately starts a catalog-derived eager scan of every canonical default and opt-in cleanup category and shows path-free rows (cursor, checkbox, scan marker, measured size) while scanning continues. Defaults start selected but may be cleared; opt-in categories start unselected; `space` toggles the focused row, `a` selects every currently selectable category for this session, and `x` clears all. Selection changes never restart the scan and never write history or detailed lists. Non-executable Review suggestions and review clues stay on CLI/JSON dry-run contracts and do not appear in the category list; the TUI does not run external tool-query probes solely for those suggestions. `Enter` opens a separate confirmation only after every scannable category is terminal and the selection is non-empty; a second `Enter` freezes the exact selected category identifiers and runs shared Clean execution (fresh resolution, current Protection, running-application gates, aggregate Recycle Bin capacity checks, Recycle Bin-only moves). Progress is observation-only and path-free; the result page projects category outcomes and actual affected bytes from the authoritative Result. Optional TUI History provenance records `surface=tui`, `selection_mode=exact`, and the selected category identifiers without fabricating CLI args. Cancellation after confirmation does not roll back completed Recycle Bin operations. The first category-first slice has no retry or rescan control; leave and re-enter Clean to start a new scan. Scripts, pipes, and `--json` callers are unaffected.

## Scope

Foal is inspired by tools like Mole, but it is not "Mole for Windows". The roadmap is ordered by Windows risk and Foal's safety model rather than feature parity.

- `clean`: conservative Foal-owned temp sandbox rule, preview-first output, Recycle Bin-only execution after explicit `--execute`.
- `uninstall`: preview-only application review for registry-discovered installed applications, their high-confidence footprint evidence, and lower-confidence orphaned residue review clues. Foal does not execute uninstallers, stop processes, or delete leftovers.
- `analyze`: read-only, JSON-first directory insight.
- `status`: read-only system snapshot.
- `history`: JSON-first record of prior Foal operations.
- `optimize`: future read-only health checks and recommendations; not current implementation scope.

The TUI is a review and navigation surface over shared read models. Its primary Clean view is category-first: it measures catalog default and opt-in cleanup categories (including `user_temp`, `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache`, `d3d_shader_cache`, `nvidia_dx_cache`, Chrome/Edge `browser_cache`, Application caches `vscode_cache` and `cursor_cache`, and developer-tool opt-in categories) as path-free rows with permission-boundary notices, without writing history or detailed lists during preview. It does not duplicate deletion, uninstall, or path-safety logic.
