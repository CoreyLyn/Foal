# Foal

Foal is a safe, preview-first cleanup CLI for Windows.

It is designed for Windows developers and power users who want cleanup, uninstall review, disk analysis, and system snapshots without handing a tool permission to make unexplained destructive changes.

## Design Principles

- Preview first: cleanup candidates should be inspectable before execution.
- Rule-driven mixed actions: each cleanup category uses Recycle Bin or permanent deletion as declared by the catalog, never as a fallback.
- Conservative defaults: default cleanup rules should be easy to explain and low-disagreement.
- Windows-native safety: protected paths, reparse points, permissions, package managers, and installer ecosystems are first-class design concerns.
- JSON contracts first: human output can be friendly, but stable JSON output is the automation and TUI contract.
- No automatic elevation: permission failures should be visible skipped items, not a reason to silently escalate.

### Permanent deletion

Permanent deletion is an explicit planned action, not a Recycle Bin fallback.

| Planned action | Categories |
| --- | --- |
| `delete_permanently` (23) | `d3d_shader_cache`, `nvidia_dx_cache`, `amd_gpu_shader_caches`, `intel_gpu_shader_cache`, `browser_cache`, `vscode_cache`, `cursor_cache`, package/build caches (`npm-cache`, `pnpm-cache`, `yarn-cache`, `go-cache`, `pip-cache`, `cargo-cache`, `nuget-cache`, `nuget-global-packages`, `corepack-cache`, `uv-cache`, `bun-cache`), `playwright-browsers`, `puppeteer-browsers`, `electron-cache`, `jetbrains-ide-caches`, `visual-studio-caches` |
| `move_to_recycle_bin` (6) | `foal_owned_temp_sandboxes` (default), `user_temp`, `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache` |

- Dry-run reports the true planned action without authorization.
- CLI execute requires per-run `--allow-permanent` in addition to `--execute` (and the matching `--opt-in` when using CLI additive opt-in). Without it, permanent candidates are skipped with `permanent_deletion_not_authorized` while Recycle Bin work continues.
- The Clean TUI starts the default plus all permanent-action categories selected when safely measured (24 rows), leaves the five Recycle Bin opt-ins unselected, discloses permanent deletion in one confirmation, and passes equivalent authorization to shared Clean.
- Permanent deletion is ordinary filesystem removal only: no secure erasure, shred, free-space wipe, or forensic non-recoverability claim.

See [Clean deletion policy](docs/plan/clean-deletion-policy.md) and [ADR 0018](docs/adr/0018-permanent-deletion-is-an-explicit-planned-action.md).

## Build

Requires Go 1.25+ on Windows:

```powershell
go build -o foal.exe ./cmd/foal
.\foal.exe --help
```

Versioned releases inject tag and commit via the release pipeline; local builds report `dev`.

## Implemented Command Shape

```powershell
foal --help
foal version --json
foal analyze --json .
foal clean --dry-run --json
foal clean --execute
foal clean --execute --opt-in d3d_shader_cache --allow-permanent
foal clean --execute --opt-in playwright-browsers --allow-permanent
foal purge .\my-project
foal purge --json .\proj-a .\proj-b
foal purge --execute --allow-permanent .\my-project
foal status --json
foal history --json
foal uninstall --json
```

`foal version`, `foal --version`, and their `--json` forms report the version, source commit, Go runtime, and target platform without reading or changing user state.

### Clean

`foal clean` requires either `--dry-run` or `--execute`.

**Dry-run** previews default candidates and reports skipped-by-default Opportunity categories:

- Idle user-temp entries as `user_temp`
- Existence-observed current-user roots: `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache`, `d3d_shader_cache`, `nvidia_dx_cache`, `amd_gpu_shader_caches`, `intel_gpu_shader_cache`
- Chrome/Edge/Firefox `browser_cache` when the browser is idle before and after complete profile cache inspection
- VS Code `vscode_cache` and Cursor `cursor_cache` when the editor is idle before and after exact allowlisted root inspection under standard Roaming AppData `Code` or `Cursor`

Observed opportunity bytes stay separate from `Potential space`. The Recycle Bin is permanently excluded from opportunity discovery. Developer-tool caches remain Review suggestions by default, or become Opt-in candidates via `--opt-in <name>`, `--opt-in dev-caches`, `--opt-in all`, or Clean TUI selection. Administrator-only caches (SoftwareDistribution, Delivery Optimization) are permission-boundary notices only.

**Clean does not discover or delete project artifacts** (`node_modules`, `target`, `dist`, and similar rebuildable project directories) by default or via ordinary Clean `--opt-in` catalog rows. Dry-run may show a presentation-only Review clue pointing at `foal analyze` / `foal purge`. Use [`foal purge`](#purge) for explicit-root project artifact preview and deletion.

**Execute** does not run opportunity discovery or browser/application detection by default. It confirms cleanup for freshly scanned, validated default candidates plus any explicitly opted-in categories, using each category's catalog planned action. Permanent categories require `--allow-permanent`. When opting in:

- `browser_cache` / Application caches: running-application idle-before-and-after gates run fresh
- Developer caches: paths resolve only from environment variables and default paths (no tool probing at execute)
- Recycle Bin capacity pre-checks apply only to Recycle Bin work

Structured developer-cache highlights:

- `playwright-browsers`: complete versioned installations under the global Playwright root; `PLAYWRIGHT_BROWSERS_PATH=0` yields no candidate; MCP profiles excluded
- `puppeteer-browsers`: allowlisted product/platform-version installations under `PUPPETEER_CACHE_DIR` or `~\.cache\puppeteer`
- `electron-cache`: whole root from non-blank `electron_config_cache` or `%LOCALAPPDATA%\electron\Cache`
- `jetbrains-ide-caches`: exact `caches`/`index` (and Rider `resharper-host`) under supported IntelliJ-platform product-version roots; Local History excluded; independent product idle gates
- `visual-studio-caches`: exact `ComponentModelCache` under current-user 14.0+ instance hives and shared `Roslyn` under `%LOCALAPPDATA%\Microsoft\VisualStudio`; devenv idle gate; Settings/Extensions/ProgramData excluded

Prefer non-destructive examples such as `foal clean --dry-run --json` in docs and verification.

### Purge

`foal purge` is an independent **Project artifact purge** command. It is not a Clean catalog category and never runs as part of `foal clean`.

- **Explicit roots only.** One or more user-supplied project/workspace roots are required (for example `foal purge .\my-project`). Foal never invents defaults, never loads implicit multi-root config (no Mole-style `purge_paths`), and rejects dangerous roots such as volume roots and Windows/Program Files paths before scanning.
- **Preview first.** Default mode is dry-run (optional explicit `--dry-run`). It recursively discovers allowlisted rebuildable directories under the supplied root(s) and reports kind, path, relative path, measured bytes, and planned permanent deletion without mutating.
- **v1 allowlist** (exact final path component; same high-confidence set as analyze labels): `node_modules`, `target`, `dist`, `build`, `.build`, `.next`, `__pycache__`. Names such as `bin`, `obj`, or `node_modules_backup` are not matched.
- **Execute.** `foal purge --execute --allow-permanent <root> [root...]` re-discovers under the same roots, then permanently deletes matching artifacts. Without `--allow-permanent`, execute skips every candidate with `permanent_deletion_not_authorized` and deletes nothing. Permanent deletion is ordinary filesystem removal only (not secure erasure). Removing project artifacts requires reinstalling dependencies and rebuilding afterward.
- **Safety.** User Protection rules apply (deny-only). Fresh path-safety validation runs before each deletion. No elevation, process stopping, installer purge, or secure-erase claims.

See `foal --help` (Purge options) for the shipped flag surface. Prefer `foal purge --json .\my-project` for preview automation.

### Other commands

- `foal analyze --json <path>`: read-only directory insight with totals, top children, skipped entries, and elapsed time. When a **top child** name matches the project-artifact allowlist, it is labeled `project_artifact_clue` only (no deep nested scan, no deletion)
- `foal status --json`: read-only snapshot with disk capacity, OS runtime, Foal command state, elapsed time, and structured `skipped` / `errors`
- `foal history --json`: operation history sessions or structured history errors (including purge sessions)
- `foal uninstall --json`: preview-only registry applications, footprint leftovers, orphaned residue, shared-state concerns, and empty execution actions

### Protection Rules

Foal loads optional user-defined Protection rules from `%APPDATA%\Foal\protection.txt`. Set `FOAL_PROTECTION_FILE` to select a different file. Each non-empty, non-comment line is one absolute local path; comments begin with `#`. UNC paths, relative paths, and paths containing 8.3 short-name segments are invalid.

A valid entry protects that path and its entire subtree using normalized, case-insensitive, path-component-aware matching. Protection rules are deny-only: they can suppress Clean candidates, path-backed review-only discoveries, and purge candidates, but can never add or authorize cleanup. Protected path-backed discoveries are removed before totals, JSON, human output, the Clean TUI, detailed candidate lists, and history projection. Suggestions without a resolved cache path are not inferred from command text.

Invalid lines are skipped with structured Protection diagnostics. A missing default file means no user-defined rules; a selected override that cannot be loaded, or a selected file with invalid UTF-8, fails Clean or purge closed before scanning or execution.

### Interactive TUI

Running `foal` with no arguments in an interactive terminal opens a TUI: a main menu, category-first Clean preview with confirmed shared Clean execution, and read-only viewers for uninstall, status, and history.

Entering Clean starts a catalog-derived eager scan of every canonical default and opt-in cleanup category and shows path-free rows while scanning continues.

- Defaults start selected but may be cleared
- Permanent-action categories start selected when measurable
- Other opt-ins start unselected
- `space` toggles the focused row; `a` selects every currently selectable category; `x` clears all
- Selection never restarts the scan and never writes history or detailed lists
- Non-executable Review suggestions stay on CLI/JSON dry-run only; the TUI does not run external tool-query probes solely for those suggestions
- `Enter` opens confirmation only after every scannable category is terminal and the selection is non-empty
- Confirmation groups Permanent deletion and Recycle Bin work; a second `Enter` freezes the exact selected categories plus permanent authorization and runs shared Clean (fresh resolution, Protection, gates, Recycle Bin capacity for Recycle Bin work only, Recycle Bin first then permanent)
- Progress is observation-only; the result page projects outcomes from the authoritative Result
- Optional TUI History provenance records `surface=tui`, `selection_mode=exact`, and selected category identifiers
- Cancellation after confirmation does not roll back completed work
- No retry or rescan control in the first category-first slice; leave and re-enter Clean to start a new scan

Scripts, pipes, and `--json` callers are unaffected.

## Scope

Foal is inspired by tools like Mole, but it is not "Mole for Windows". The roadmap is ordered by Windows risk and Foal's safety model rather than feature parity. Shipped purge does **not** claim Mole parity (no installer purge, no implicit `purge_paths` config).

- `clean`: conservative preview-first rules; mixed-action execution after explicit `--execute`. Permanent categories are the full permanent matrix above (CLI `--allow-permanent`, TUI confirmation). The six Recycle Bin categories stay Recycle Bin. Clean never discovers or deletes project artifacts by default or via ordinary catalog opt-in rows.
- `purge`: independent explicit-root Project artifact purge; preview by default; permanent deletion only with `--execute --allow-permanent`. Not a Clean category.
- `uninstall`: preview-only application review. Foal does not execute uninstallers, stop processes, or delete leftovers.
- `analyze`: read-only, JSON-first directory insight (top-child project-artifact labels only; no nested project scan, no deletion).
- `status`: read-only system snapshot.
- `history`: JSON-first record of prior Foal operations (including purge).
- `optimize`: future read-only health checks and recommendations; not current implementation scope.

The TUI is a review and navigation surface over shared read models. Its primary Clean view is category-first. It does not duplicate deletion, uninstall, or path-safety logic.

## Releases

Foal uses Semantic Version tags to create draft GitHub releases. The initial release channel provides portable Windows amd64 and arm64 ZIP archives containing `foal.exe`, plus SHA-256 checksums and GitHub provenance attestations. A maintainer smoke-tests each draft before publishing it; there are no automatic nightly releases.

See [Release process](docs/plan/release-process.md) for release gates, artifact contents, and the staged Scoop/WinGet plan.

## Platform compatibility

Foal's compatibility baseline is Windows 10 or later, or Windows Server 2016 or later. Windows 11 x64 is the primary desktop target. ARM64 archives are preview builds until native ARM64 smoke testing is available. The current hosted CI executes only on Windows Server 2025 x64; see the [Windows support research](docs/research/windows-support.md) for the distinction between compatibility and verified support.

## License

Foal is licensed under the [GNU General Public License v3.0 only](LICENSE) (`GPL-3.0-only`).
