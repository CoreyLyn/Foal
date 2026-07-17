# Foal Overall Development Plan

Foal is a safe, preview-first cleanup CLI for Windows. The project is a pre-release hard rename: project name `Foal`, command name `foal`, and executable name `foal.exe`.

Foal may reference Mole as inspiration, but should not position itself as "Mole for Windows". Feature order follows Windows-specific risk and Foal's safety model.

## Product Decisions

- Target users are Windows developers and power users.
- The CLI is the primary interface. The TUI is read-model driven and calls shared command/core execution paths for confirmed Clean work.
- Human output should be readable; JSON contracts are the stable automation surface.
- Rich human reports may borrow Mole's grouped dry-run style, but the reporting experience must not imply Mole feature parity or expand Foal's default cleanup rules.
- Default cleanup rules are conservative. Higher-risk rule groups require explicit opt-in.
- Clean is mixed-action: every executable rule declares `move_to_recycle_bin` or `delete_permanently`. Permanent deletion applies only to proven regenerable categories, is never a Recycle Bin fallback, and requires per-run authorization (`--allow-permanent` or TUI confirmation). See ADR 0018 and `docs/plan/clean-deletion-policy.md`.
- Automatic elevation is out of scope.
- Operation history and logging are MVP-level safety features.
- `uninstall` is preview-only until a future execution model is designed.
- `optimize` is reserved for future read-only health checks and recommendations.

## Mole Category Mapping

| Mole category | Foal direction |
| --- | --- |
| `clean` | Conservative preview-first cleanup with rule-driven mixed-action execution (Recycle Bin or permanent per catalog). |
| `uninstall` | Current preview-only registry application discovery, installed-application footprint review, orphaned residue review clues, human report, and JSON review sections; future execution model is separate. |
| `analyze` | Read-only, JSON-first directory insight engine. |
| `optimize` | Future read-only health checks and recommendations; not current implementation scope. |
| `status` | Read-only system snapshot; realtime monitoring is future TUI work. |

## Implemented Command Boundaries

| Command | Current behavior |
| --- | --- |
| `foal --help` | Shows the implemented command list and Foal/foal/foal.exe examples. |
| `foal version`, `foal --version`, `--json` forms | Read-only shared build metadata: version, commit, Go runtime, and target platform. |
| `foal status --json` | Read-only system and Foal state snapshot with disk, OS, elapsed time, skipped items, and errors. |
| `foal analyze --json <path>` | Read-only directory insight with totals, top children, skipped entries, and elapsed time. |
| `foal clean --dry-run --json` | Preview-only cleanup candidate review; reports true planned actions without permanent authorization. |
| `foal clean --execute` | Explicit cleanup confirmation; uses each category's catalog planned action. Permanent categories also need `--allow-permanent`. |
| `foal history --json` | Reads Foal operation history and reports sessions or structured history errors. |
| `foal uninstall --json` | Preview-only uninstall review with registry-discovered applications, installed-application footprint evidence, orphaned residue review clues, shared-state and unknown-state sections, JSON review sections, and empty execution actions. |

## Phase 1: Rename and Safety Baseline

- Keep user-visible product references aligned on Foal naming.
- Prefer `foal`, `foal.exe`, and `cmd/foal` for command and build examples.
- Do not keep legacy command aliases or compatibility layers during the pre-release rename.
- Keep README, AGENTS, and plan docs aligned.
- Preserve preview-first cleanup and catalog-owned mixed-action deletion (ADR 0018).
- Preserve protected path and path validation invariants.

## Phase 2: Contract Stabilization

- Stabilize JSON output contracts for `analyze`, `clean`, `status`, `history`, and `uninstall`.
- Add contract tests around structured outputs and error semantics.
- Keep default-enabled cleanup behavior frozen unless a change is explicitly approved.
- Treat safety, skipped reasons, and recoverability as part of the command contract.
- Define a clean preview read model that can represent default candidates, skipped-by-default items, review clues, review suggestions, protection rules, permission boundary notices, running application skips, detailed candidate list metadata, and report totals.
- Keep `Potential space` scoped to default candidates only; skipped-by-default items, review clues, external suggestions, and permission-boundary skips must not be counted as Foal-cleanable space.
- Load deny-only user-defined Protection rules from `%APPDATA%\Foal\protection.txt` or `FOAL_PROTECTION_FILE`; valid entries protect an exact path and subtree but never authorize or expand cleanup.
- Suppress protected user-temp opportunities and Review suggestions with resolved protected cache paths before totals, read-model projection, detailed lists, and history. Do not infer paths from Review suggestion command text.

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
- Display `Protection rules` using Foal's Windows path-safety boundaries as the primary language.
- Include a permission boundary notice when administrator-only or protected locations are skipped, without recommending elevation as the normal path.
- Write a detailed candidate list under Foal's config/history area as a human-readable companion artifact, not as an execution manifest.
- Surface external tool commands as review suggestions only. Foal must not execute them, delegate cleanup to them, or count their estimates as `Potential space`.
- Treat browser, IDE, package-manager, AI-tool, Docker, WSL, virtualization, sync-client, project-artifact, and application-leftover findings as skipped-by-default or review clues unless an explicit future opt-in rule group is approved.
- Treat running application state as a skip reason, not as a close-and-clean prompt or default cleanup condition.

## Phase 3B: Clean Skipped-by-Default Discovery

Historical phase note: later work added browser/Application cache opportunities, opt-in execution, structured developer caches, and catalog-owned mixed actions (ADR 0018). The bullets below describe the original Phase 3B slice.

- Keep the default candidate set frozen; this phase expands read-only discovery, not default or opt-in execution.
- Discover non-Foal-owned top-level entries under the current user's Windows temporary directory for `clean --dry-run` and the read-only Clean TUI only.
- Report an entry as a user temp opportunity only when the latest modification observed across the entry and all safely inspectable descendants is at least seven days old.
- Exclude entries from opportunity results when inspection is incomplete because of path-safety rejection, permission failure, cancellation, or the deterministic per-entry ceiling of 100,000 descendants.
- Report each opportunity with path, measured bytes, latest observed modification, idle days, `skipped_by_default` status, and the fixed `requires_explicit_opt_in` reason.
- Report opportunity count and observed bytes separately; never include observed opportunity bytes in `Potential space`.
- Persist only opportunity count and observed bytes in dry-run history. Do not persist non-Foal opportunity paths in history, and do not add opportunity data to execution history.
- Include full opportunity review data in a separate detailed candidate list section while keeping that file non-authoritative and unused by execution.
- Do not run skipped-by-default discovery during `clean --execute`; execution continues to fresh-scan and validate only executable default candidates.
- Keep protected review-only paths out of JSON, human output, the Clean TUI, detailed candidate lists, and raw history records while preserving unprotected siblings.
- Do not impose a wall-clock timeout. Honor cancellation, report elapsed time, and continue scanning other top-level entries after an entry-specific incomplete inspection.
- Categorize every opportunity in the shared contract. The existence-observed System opportunity catalog includes `user_temp`, `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache`, `d3d_shader_cache`, `nvidia_dx_cache`, `amd_gpu_shader_caches`, and `intel_gpu_shader_cache` (plus gated browser/application opportunities elsewhere). Preserve idle-age observation for `user_temp`, and observe the current user's fixed CrashDumps, Windows Error Reporting, Explorer thumbnail cache, INetCache, D3D/NVIDIA/AMD/Intel GPU shader cache roots as existence-based opportunities without age fields (AMD/Intel use exact allowlisted multi-root layouts from research).
- Keep browser caches absent until running-application detection exists, keep the Recycle Bin permanently absent, and keep allowlisted developer-tool caches in Review suggestions so no bytes are duplicated across surfaces.
- Never inspect SoftwareDistribution, Delivery Optimization, or other administrator-only roots as opportunities. Communicate those exclusions through the shared permission-boundary notice without requesting or recommending elevation.

## Phase 4: Uninstall Preview Quality

- Implemented registry application discovery and evidence reporting for installed applications.
- Implemented installed-application footprint review as high-confidence possible leftovers tied to a currently discovered application.
- Implemented orphaned residue as low-confidence read-only review evidence, distinct from installed-application footprint and never treated as cleanup candidates.
- Implemented human uninstall reporting and JSON review sections over the shared uninstall result.
- Continue to explain skipped discovery, unknown state, and shared-state concerns as preview-only review information.
- Do not execute uninstallers, delete leftovers, stop processes, or add a Phase 5 execution plan in this phase.

## Phase 5: TUI (Partial) and mixed-action Clean (implemented)

Implemented an interactive TUI: running `foal` with no arguments opens a main menu, a category-first Clean four-stage flow (eager path-free preview scan → exact selection with live measured totals → separate confirmation → shared execution/result), and read-only viewers for uninstall, status, and history. Entering Clean starts a catalog-derived sequential measurement of every canonical default and opt-in cleanup category; defaults start selected but removable, permanent-action categories start selected when safely measurable, Recycle Bin opt-ins start unselected, and selection never restarts scanning. With the current catalog that is 24 initially selected categories when all rows are measurable. Retry and rescan controls remain deliberately outside the current TUI scope; after external state changes, the user leaves Clean and enters it again to start a new eager preview. Review suggestions remain on non-TUI dry-run contracts only. Confirmed execution freezes exact category identifiers and permanent authorization when permanent actions are disclosed, resolves candidates fresh, reloads safety boundaries, performs aggregate per-volume Recycle Bin capacity checks for Recycle Bin work only, executes Recycle Bin work first and permanent work last, validates immediately before each mutation, reports observation-only progress, and records normal Clean history plus optional path-free TUI provenance and split actual-action totals. This mandatory execution-time re-resolution is a safety invariant, not a user-facing preview rescan feature. Preview browsing, empty/unavailable/no-selection, and cancellation before confirmation create no history or companion files. Cancellation during execution does not promise rollback; results show only outcomes returned by shared Clean. Non-Clean TUI commands remain read-only, and Analyze/future extensions remain navigation placeholders.

ADR 0018 and `docs/plan/clean-deletion-policy.md` describe the implemented rule-driven deletion matrix: 23 permanent and 6 Recycle Bin executable categories plus one non-executable administrator permission boundary. CLI and TUI share category actions; CLI permanent execution requires `--allow-permanent`; the TUI confirmation supplies equivalent authorization; permanent deletion is never a Recycle Bin fallback and is not secure erasure.

The following design principles continue to govern TUI work:

- Build the Clean TUI as a review, category-selection, confirmation, and result surface; keep other command views read-only.
- Consume shared read models.
- Call shared command/core execution paths for confirmed actions.
- Do not place candidate resolution, deletion, uninstall, process-stopping, elevation, or path-safety decisions inside the TUI layer.

## Explicit Non-Goals For Current Work

- No "Mole for Windows" feature parity commitment.
- No automatic elevation.
- No implicit, undisclosed, fallback, or secure-erasure permanent deletion.
- No full rule pack or profile system.
- No browser cache discovery before running-application detection.
- No Recycle Bin Opportunity category.
- No default expansion of IDE, package-manager, or developer-cache rules; those stay skipped-by-default Review suggestions until explicit Opt-in or Clean TUI selection, then use their catalog planned action (many permanent per ADR 0018).
- No administrator-only Opportunity roots or automatic elevation.
- No system optimization actions.
- No uninstall execution model.
