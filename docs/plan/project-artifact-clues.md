# Plan: Project artifact clues

A design note captured from a grilling session, durable for a later `/to-prd`. This is the one resolved Clean borrow-from-Mole branch that did not warrant an ADR (the decisions are reversible, unsurprising, and low-stakes), so its decisions are recorded here instead.

## Goal

Borrow Mole's `purge` idea (surfacing large rebuildable project build artifacts like `node_modules`, `target`, `dist`) into Foal — but strictly in the read-only lane, respecting the existing glossary term **Project artifact clue**:

> A review clue for rebuildable project directories or build outputs that Foal may surface only through explicit analysis or future opt-in flows. _Avoid_: default project scan, default clean candidate.

The hard constraint: a default `foal clean --dry-run` must **not** go hunting for `node_modules` across the disk. Mole itself keeps `purge` as a separate command and `mo clean` only carries a hint pointing at it.

## Decisions

- **Ownership split (analyze labels; clean points; purge deletes).** Project-artifact awareness for read-only insight lives in `foal analyze`, which reports top children by size. When a top child's name matches a recognized rebuildable-artifact set, `analyze` labels it. `foal clean --dry-run` carries only a single static pointer clue directing the user to `foal analyze <path>` and `foal purge <root>`. Nested discovery and deletion are owned by the shipped independent `foal purge` command—not by Clean.

- **No deep scanning in analyze.** `analyze` stays "top children by size"; it does NOT recurse looking for nested artifacts. Going deeper would turn `analyze` into a project scanner (scope creep). Limitation: only artifacts that happen to be a top child of the analyzed path are labeled (e.g. `analyze <project-root>` catches a direct `node_modules`; `analyze ~/Projects` would not catch artifacts two levels down). Nested reclaim under an explicit root is `foal purge`'s job.

- **Recognized artifact set (high-confidence, v1).** `node_modules`, `target`, `dist`, `build`, `.build`, `.next`, `__pycache__`. Deliberately excludes generic `bin`/`obj` to avoid noise; the set can be widened later. Analyze uses the set for labels only; `foal purge` uses the same set for discovery/deletion.

- **clean's pointer clue is a presentation-only constant.** It fills the existing `ReviewClue` read-model slot with a constant entry (name "Rebuildable project artifacts", empty path, details pointing at `foal analyze <path>` and `foal purge <root>`). It is injected in the clean preview read-model projection and rendered in human output and the TUI. It is NOT added to the clean `Result` JSON contract — it is constant UX guidance, not data (consistent with how the suggestions static safety note is handled).

## Rejected alternatives

- **Opt-in scan flow in clean** (e.g. `foal clean --project-artifacts <root>`): also glossary-legal as a "future opt-in flow", but adds a new flag and duplicates `analyze`'s directory walk.
- **Static clue in clean only, nothing in analyze**: minimal, but leaves the actual artifact insight unbuilt; `analyze` is the natural home for directory insight.

## Seams (prior art to follow)

- `analyze.Result` / `ChildResult` — add a way to flag a recognized rebuildable artifact among top children (an artifact-kind field on `ChildResult`, or a clues array on `Result`). Test through the public `analyze` entry point with a constructed directory tree; prior art: existing analyze tests.
- Clean preview read-model projection (`NewPreviewReadModel`) — inject the constant pointer `ReviewClue`; the renderer and TUI already render the Review clues section. Prior art: the clean preview projection/render tests, and `TestPreviewReportRendersReviewOnlySectionsWithoutExecutionSemantics`.

## Out of scope (this slice)

- Deletion of project artifacts through ordinary Clean default or catalog opt-in rows (this slice stays read-only labels/points).
- Deep/recursive artifact discovery in `analyze`.
- Configurable multi-root defaults analogous to Mole's `purge_paths` without an explicit user root.

## Shipped direction: `foal purge` (ADR 0019)

An independent **Project artifact purge flow** is shipped as the top-level command `foal purge` (issues #241–#243; user-docs alignment #244). It does **not** reopen default disk-wide project scanning or expand ordinary Clean catalog rows.

| Surface | Behavior |
| --- | --- |
| Command | `foal purge <root> [root...]` — roots never implied |
| Preview | Default dry-run (optional `--dry-run`); recursive allowlisted discovery under supplied roots; planned action `delete_permanently` without mutation |
| Execute | `--execute --allow-permanent`; re-discovers then permanently deletes matches; without authorization, candidates are skipped and nothing is deleted |
| Allowlist (v1) | Same high-confidence set as analyze: `node_modules`, `target`, `dist`, `build`, `.build`, `.next`, `__pycache__` (exact final component; no `bin`/`obj`) |
| Safety | Protection rules (deny-only), dangerous-root rejection, reparse fail-closed, inspection ceilings, history sessions distinct from Clean |
| Not shipped | Mole installer purge, implicit multi-root `purge_paths` config, Clean `--opt-in` project-artifact category, nested labeling inside `analyze` |

The read-only analyze labels and Clean presentation-only pointer in this plan remain valid. Clean still does not discover or delete project artifacts by default or via ordinary opt-in catalog rows.

## How to extend later

Further allowlist entries, selection filters, or a purge TUI entry need their own issue/tests. Do not fold project-artifact deletion into ordinary Clean opt-in rows without reopening ADR 0019.
