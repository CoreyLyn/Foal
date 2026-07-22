# Plan: Analyze directory insight

Design captured from a `/grill-with-docs` session. Glossary terms live in `CONTEXT.md`. Product boundary decision: ADR 0025.

## Goal

Make `foal analyze` a trustworthy **directory insight** command: measure an analysis root like a constrained `du`, surface top children by size, attach only proven high-confidence classification clues, and never delete or authorize cleanup.

Hard constraints:

- Not a Clean opportunity scanner, Mole-style disk explorer, or ad hoc Trash/delete surface.
- Nested rebuildable discovery and permanent reclaim stay on `foal purge`.
- Protection rules remain cleanup-side only; they do not hide disk usage from Analyze.

## Current baseline (shipped today)

| Surface | Behavior |
| --- | --- |
| CLI | `foal analyze [path]` — omit path → `.` then absolute |
| Scan | Full recursive totals; root-level top 10 by bytes; reparse/permission → `skipped` |
| Clues | Top-child directory name in v1 allowlist → `classification=project_artifact_clue` |
| JSON | `status/root/totals/top_children/skipped/elapsed_ms` (`status` effectively always `ok`) |
| Human | Minimal: root, file/dir counts, skipped count — no bytes, top list, or clues |
| TUI | Main-menu placeholder pointing at `foal analyze --json` |
| History | Not recorded |
| Safety | No `ValidateUserScanRoot`; no descendant ceiling |

Related shipped docs: `docs/plan/project-artifact-clues.md`, ADR 0019 (`foal purge`).

## Decisions

### Product identity

- **Layered directory insight (D).** Core job is directory totals + top children by size. Optional layer: proven high-confidence **Analyze classification clues** only. Never Potential space, never cleanup candidates, never mutation.

### Analysis root

- **Single root.** Multi-root is out of this design.
- **Default CWD.** Omitting the path means the process current working directory after absolute resolution.
- **Analyze read-root fail-closed.** After absolute resolution, use Analyze-specific `ValidateAnalyzeReadRoot` (not purge's `ValidateUserScanRoot`). Explicit local fixed/removable volume roots and readable local directories—including Windows-managed trees and the profile root—are accepted for measurement only. UNC, device paths (`\\.\`), short-name, empty, relative final roots, unsupported volume types, and reparse roots fail closed. Mutation-oriented purge/clean root validators remain unchanged.
- **Not an implied whole-machine default.** No-argument Analyze still uses CWD; volume roots are accepted only when the user supplies them explicitly (or navigates in TUI after later slices).

### Scan completeness

- **Hard ceiling.** Use the same **100,000-descendant** Opportunity inspection limit.
- **Honest incomplete.** Hitting the ceiling or cooperative cancellation yields **`status=incomplete`**. Totals and top children describe only what was safely inspected; never present as a complete tree size; never invent estimates to “fill in” the rest.
- **Not a hard command crash** for ordinary over-limit stops (still a structured Analyze result, not an error envelope for that case).
- **Filesystem skips remain skips.** Permission, reparse, missing, read errors continue as `skipped` items and do not by themselves redefine product identity.

### Classification clues

- **Top children only.** No recursive nested artifact labeling inside Analyze (nested reclaim is purge).
- **Near-term single kind.** Only `project_artifact_clue` with the existing v1 allowlist shared with purge: `node_modules`, `target`, `dist`, `build`, `.build`, `.next`, `__pycache__` (exact final component; no `bin`/`obj`).
- **No new clue kinds** (large file, old download, etc.) without separate proof — those slide into opportunity-scanner territory.

### Human report

- Non-JSON output must surface the same core insight as JSON: analysis root, complete-or-incomplete status, totals **including bytes**, top children (name/size/kind/classification), skipped summary, and purge handoff when applicable.
- Must not invent execute/delete affordances or Potential space.

### Purge handoff

- When at least one project artifact clue is present, human report and Analyze TUI may show **read-only next-step copy** pointing at `foal purge <root>`.
- Never launch purge, never select purge candidates, never authorize deletion from Analyze.

### Protection

- **Non-intervention.** User `protection.txt` does not suppress or reshape Analyze measurement. Protection continues to deny Clean/purge candidates only.

### History

- **Non-recording.** Analyze runs create no History sessions. History stays for confirmed or mutating cleanup-class operations; scripts may capture `--json` themselves.

### Top children count

- **Fixed top 10** by bytes (name tie-break). No `--top` in this design slice.

### TUI

- **Command viewer**, not a Mole disk explorer.
- Default analysis root = CWD; allow **simple path edit/paste** then rescan another allowed root.
- Reload re-runs shared Analyze.
- No cleanup, selection, permanent deletion, overview launcher, or directory-drill delete UX.
- Renders Analyze human report / equivalent read model as scrollable text.

### Implementation order

1. Dangerous-root validation + 100k ceiling + `status=incomplete` (+ contract tests).
2. Human report aligned with JSON core (bytes, top children, clues, incomplete, purge handoff copy).
3. Analyze TUI viewer (CWD + simple path change, read-only).

## Rejected alternatives

| Idea | Why rejected |
| --- | --- |
| Analyze as cleanup opportunity / large-file finder | Conflicts with Clean/opportunity boundaries and conservative Foal identity |
| Mole-style overview + enter-dir + Trash delete | Feature-parity chase; turns Analyze into ad hoc cleanup; contradicts read-only TUI rules |
| Windows overview allowlist as first TUI | Useful later maybe; not first slice; Command viewer + path edit is enough |
| Deep nested artifact scan in Analyze | Scope creep into project scanner; purge owns nested discovery |
| Protection as Analyze scan deny-list | Hides honest disk insight; protection is a cleanup safety boundary |
| `status=ok` + `complete=false` | Weaker for scripts; incomplete is a first-class result state |
| Incomplete as non-zero crash | Ordinary large trees would “fail”; over-limit is expected |
| Multi-root analyze | Contract growth without need for du-style insight |
| Analyze History sessions | No mutation/authorization; noisy and path-sensitive |
| Expand clue allowlist / new clue types now | Noise and identity drift; needs separate evidence |
| Configurable `--top` in first slice | Presentation knob; not needed to land trustworthiness |

## Seams (implementation)

- `internal/analyze` — `Run`, `Result.Status`, ceiling counting, incomplete semantics; tests via public entry with temp trees.
- `internal/core/pathsafe.ValidateAnalyzeReadRoot` — Analyze-only read-root policy after `filepath.Abs` (volume roots and Windows-managed trees allowed; UNC/device/reparse fail closed). Purge keeps `ValidateUserScanRoot`; mutation keeps delete/portable validators.
- `internal/cli` human renderer for analyze — project from `analyze.Result`, not a second model.
- `internal/cli` TUI — Command viewer pattern used by status/history/uninstall; wire Analyze with path edit + reload calling shared `analyze.Run`.
- Contract tests: dangerous root rejection; incomplete status; human output contains bytes/top/clues; TUI has no delete affordance; no history side effects.

## Out of scope (this design)

- Mole overview parity, drill-down browser, Open/Preview/File modes, multi-select delete.
- Nested labeling or deep project scanning in Analyze.
- New classification kinds or allowlist expansion.
- `--top N`, multi-root, Analyze History.
- Protection-driven analyze skips.
- Any Analyze → purge auto-launch or shared selection state.
- Changing purge or Clean execution policy (only presentation handoff copy).

## Glossary pointers

See `CONTEXT.md`: **Analyze (directory insight)**, **Analysis root**, **Analyze incomplete scan**, **Analyze human report**, **Analyze protection non-intervention**, **Analyze TUI viewer**, **Analyze classification clue**, **Analyze purge handoff**, **Analyze history non-recording**, **Project artifact clue**, **Project artifact purge flow**.

## How to extend later

- New classification kinds need evidence, glossary updates, and usually their own plan/ADR if they change product identity.
- Nested insight remains purge (or a future explicit scanner), not silent Analyze deep-scan.
- If TUI path edit proves insufficient, a read-only overview of **safe** entry points may be reconsidered without adopting Mole delete semantics.
- `--top` remains a reversible presentation flag if users demand it after the trustworthiness slice ships.
