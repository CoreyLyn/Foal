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
- This is a pre-release hard rename from Wole: do not keep a `wole` alias, and do not add legacy config or history migration unless explicitly requested.
- User-facing docs, help text, examples, build outputs, and command paths should prefer `Foal`, `foal`, and `foal.exe`.

## Product boundaries

Foal is a safe, preview-first cleanup CLI for Windows. It is inspired by tools like Mole, but it is not "Mole for Windows" and should not chase feature parity by default.

- Build Windows-native behavior around Windows risk: protected paths, Recycle Bin behavior, reparse points, UAC, installers, package managers, and JSON-first automation.
- Keep default cleanup rules conservative. More aggressive or high-disagreement rules must require explicit opt-in.
- Default execution is Recycle Bin-only. Do not add permanent deletion as a default behavior.
- Do not add automatic elevation. Permission failures should be reported as skipped items with clear reasons.
- `uninstall` is preview-only until a future execution model is explicitly designed.
- `optimize` is not in the current implementation scope. Future optimize work starts as read-only health checks and recommendations.
- Future TUI work must consume read models and call shared command/core execution paths; it must not own deletion, uninstall, or path-safety logic.

## Engineering constraints

- Preserve preview-first behavior and JSON-contract-first command surfaces.
- Every real delete path must be validated immediately before execution, even if it was already scanned or previewed.
- Tests should prioritize safety invariants and JSON contracts over human-output snapshots.
- Keep README, AGENTS, and plan docs aligned when changing product boundaries.
