# Foal Overall Development Plan

Foal is a safe, preview-first cleanup CLI for Windows. The project is a pre-release hard rename: project name `Foal`, command name `foal`, and executable name `foal.exe`.

Foal may reference Mole as inspiration, but should not position itself as "Mole for Windows". Feature order follows Windows-specific risk and Foal's safety model.

## Product Decisions

- Target users are Windows developers and power users.
- The CLI is the primary interface. Future TUI work is planned, but it must remain read-model driven and call shared command/core execution paths.
- Human output should be readable; JSON contracts are the stable automation surface.
- Rich human reports may borrow Mole's grouped dry-run style, but the reporting experience must not imply Mole feature parity or expand Foal's default cleanup rules.
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
- Define a clean preview read model that can represent default candidates, skipped-by-default items, review clues, review suggestions, protection rules, permission boundary notices, running application skips, detailed candidate list metadata, and report totals.
- Keep `Potential space` scoped to default candidates only; skipped-by-default items, review clues, external suggestions, and permission-boundary skips must not be counted as Foal-cleanable space.

## Phase 3: Verification Sprint

- Prioritize path safety tests: protected paths, long paths, UNC, short names, symlinks, junctions, and hardlinks.
- Verify dry-run and real execution paths both validate candidates.
- Keep real Recycle Bin integration tests explicitly opt-in.
- Validate docs and command examples after rename.
- Verify rich human report rendering as presentation behavior over the clean preview read model, while keeping JSON contracts as the stable automation surface.

## Phase 3A: Clean Dry-Run Human Report

- Start with the clean preview read model and human report renderer over existing conservative clean data; do not introduce new scanner rules in the first implementation slice.
- Add a Mole-inspired but Windows-native non-JSON `foal clean --dry-run` report with grouped sections, plain ASCII scan-friendly labels, clear preview-only wording, and a final summary.
- Keep terminal output focused on summary, grouped overview, counts, and short candidate samples; put full path detail in the detailed candidate list.
- Show at most 10 default candidates, skipped items, or inspection errors per terminal report section; when a section is truncated, point to the detailed candidate list for the full path detail.
- Display `Protection rules` instead of `Whitelist`, using Foal's Windows path-safety boundaries as the primary language.
- Include a permission boundary notice when administrator-only or protected locations are skipped, without recommending elevation as the normal path.
- Write a detailed candidate list under Foal's config/history area as a human-readable companion artifact, not as an execution manifest.
- Surface external tool commands as review suggestions only. Foal must not execute them, delegate cleanup to them, or count their estimates as `Potential space`.
- Treat browser, IDE, package-manager, AI-tool, Docker, WSL, virtualization, sync-client, project-artifact, and application-leftover findings as skipped-by-default or review clues unless an explicit future opt-in rule group is approved.
- Treat running application state as a skip reason, not as a close-and-clean prompt or default cleanup condition.

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
