# Foal Overall Development Plan

Foal is a safe, preview-first cleanup CLI for Windows. The project is a pre-release hard rename: project name `Foal`, command name `foal`, and executable name `foal.exe`.

Foal may reference Mole as inspiration, but should not position itself as "Mole for Windows". Feature order follows Windows-specific risk and Foal's safety model.

## Product Decisions

- Target users are Windows developers and power users.
- The CLI is the primary interface. Future TUI work is planned, but it must remain read-model driven and call shared command/core execution paths.
- Human output should be readable; JSON contracts are the stable automation surface.
- Default cleanup rules are conservative. Higher-risk rule groups require explicit opt-in.
- Real deletion defaults to Windows Recycle Bin-only.
- Automatic elevation is out of scope.
- Operation history and logging are MVP-level safety features.
- `uninstall` is preview-only until a future execution model is designed.
- `optimize` is reserved for future read-only health checks and recommendations.

## Mole Category Mapping

| Mole category | Foal direction |
| --- | --- |
| `clean` | Conservative preview-first cleanup; Recycle Bin-only execution by default. |
| `uninstall` | Current preview-only application and leftover review; future execution model is separate. |
| `analyze` | Read-only, JSON-first directory insight engine. |
| `optimize` | Future read-only health checks and recommendations; not current implementation scope. |
| `status` | Read-only system snapshot; realtime monitoring is future TUI work. |

## Implemented Command Boundaries

| Command | Current behavior |
| --- | --- |
| `foal --help` | Shows the implemented command list and Foal/foal/foal.exe examples. |
| `foal status --json` | Read-only system and Foal state snapshot with disk, OS, elapsed time, skipped items, and errors. |
| `foal analyze --json <path>` | Read-only directory insight with totals, top children, skipped entries, and elapsed time. |
| `foal clean --dry-run --json` | Preview-only cleanup candidate review for conservative default rules. |
| `foal clean --execute` | Explicit cleanup confirmation path; default execution is Recycle Bin-only and still subject to path-safety validation. |
| `foal history --json` | Reads Foal operation history and reports sessions or structured history errors. |
| `foal uninstall --json` | Preview-only uninstall review; execution is not allowed and actions remain empty. |

## Phase 1: Rename and Safety Baseline

- Keep user-visible product references aligned on Foal naming.
- Prefer `foal`, `foal.exe`, and `cmd/foal` for command and build examples.
- Do not keep legacy command aliases or compatibility layers during the pre-release rename.
- Keep README, AGENTS, and plan docs aligned.
- Preserve preview-first cleanup and Recycle Bin-only execution.
- Preserve protected path and path validation invariants.

## Phase 2: Contract Stabilization

- Stabilize JSON output contracts for `analyze`, `clean`, `status`, `history`, and `uninstall`.
- Add contract tests around structured outputs and error semantics.
- Keep default-enabled cleanup behavior frozen unless a change is explicitly approved.
- Treat safety, skipped reasons, and recoverability as part of the command contract.

## Phase 3: Verification Sprint

- Prioritize path safety tests: protected paths, long paths, UNC, short names, symlinks, junctions, and hardlinks.
- Verify dry-run and real execution paths both validate candidates.
- Keep real Recycle Bin integration tests explicitly opt-in.
- Validate docs and command examples after rename.

## Phase 4: Uninstall Preview Quality

- Improve application discovery and evidence reporting.
- Classify leftovers by confidence and ownership.
- Explain skipped, unknown, and shared-state candidates.
- Do not execute uninstallers, delete leftovers, stop processes, or add a Phase 5 execution plan in this phase.

## Phase 5: Future TUI

- Build TUI as a review and navigation surface.
- Consume shared read models.
- Call shared command/core execution paths for confirmed actions.
- Do not place deletion, uninstall, or path-safety decisions inside the TUI layer.

## Explicit Non-Goals For Current Work

- No "Mole for Windows" feature parity commitment.
- No automatic elevation.
- No permanent deletion by default.
- No full rule pack or profile system.
- No default browser, IDE, package-manager, or developer-cache expansion without explicit opt-in.
- No system optimization actions.
- No uninstall execution model.
