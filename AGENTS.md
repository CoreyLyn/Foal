## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues for this repository. See `docs/agents/issue-tracker.md`.

### Triage labels

Triage uses the default five-label vocabulary. See `docs/agents/triage-labels.md`.

### Domain docs

This repository uses a single-context domain documentation layout. See `docs/agents/domain.md`.

## Product identity

- The project name is **Foal**.
- The CLI command name is `foal`.
- This is a pre-release hard rename: do not add legacy command aliases, config compatibility, or history compatibility unless explicitly requested.
- User-facing docs, help text, examples, build outputs, and command paths should prefer `Foal`, `foal`, and `foal.exe`.

## Product boundaries

Foal is a safe, preview-first cleanup CLI for Windows. It is inspired by tools like Mole, but it is not "Mole for Windows" and should not chase feature parity by default.

- Build Windows-native behavior around Windows risk: protected paths, Recycle Bin behavior, reparse points, UAC, installers, package managers, and JSON-first automation.
- Keep default cleanup rules conservative. More aggressive or high-disagreement rules must require explicit opt-in.
- Default execution is Recycle Bin-only. Do not add permanent deletion as a default behavior.
- Do not add automatic elevation. Permission failures should be reported as skipped items with clear reasons.
- `uninstall` is preview-only until a future execution model is explicitly designed.
- `optimize` is not in the current implementation scope. Future optimize work starts as read-only health checks and recommendations.
- The TUI is a read-only review surface: it must consume read models and call shared command/core execution paths; it must not own deletion, uninstall, or path-safety logic.

## Implemented command boundaries

- `foal --help` is the current help surface and should use Foal/foal/foal.exe naming only.
- `foal status --json` is read-only and reports disk, OS, Foal command state, elapsed time, skipped items, and errors.
- `foal analyze --json <path>` is read-only and reports directory totals, top children, skipped entries, and elapsed time.
- `foal clean --dry-run --json` previews conservative cleanup candidates and reports skipped-by-default Opportunity categories: idle `user_temp` entries; existence-observed `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache`, `d3d_shader_cache`, and `nvidia_dx_cache` current-user roots; and Chrome/Edge `browser_cache` when the browser is idle before and after complete profile cache inspection. Observed bytes are excluded from `Potential space`; the read-only Clean TUI presents the same review semantics without history or detailed-list writes. Browser review is gated by Chrome/Edge running-application detection: running browsers are reported as Running application skips and unknown process state is a recoverable diagnostic. The Recycle Bin is permanently excluded, developer-tool caches remain Review suggestions, and administrator-only caches remain permission-boundary notices. `foal clean --execute` does not run opportunity discovery or browser running-application detection, is the explicit confirmation path for fresh default candidates, and uses the Recycle Bin. Do not document permanent deletion or automatic elevation paths.
- User-defined Protection rules load from `%APPDATA%\Foal\protection.txt` or `FOAL_PROTECTION_FILE`, are deny-only, and protect an exact path plus its subtree. Suppress protected path-backed review-only discoveries before totals and all downstream surfaces; do not infer a Review suggestion path from command text.
- `foal history --json` reads operation history and reports sessions plus structured errors.
- `foal uninstall --json` is preview-only. Do not document uninstall execution, process stopping, or leftover deletion as supported behavior.

## Engineering constraints

- Preserve preview-first behavior and JSON-contract-first command surfaces.
- Every real delete path must be validated immediately before execution, even if it was already scanned or previewed.
- Protection rules can only remove candidates or path-backed review discoveries; they never authorize cleanup or expand the default candidate set.
- Tests should prioritize safety invariants and JSON contracts over human-output snapshots.
- Keep README, AGENTS, and plan docs aligned when changing product boundaries.
