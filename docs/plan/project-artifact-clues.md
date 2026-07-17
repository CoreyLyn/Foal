# Plan: Project artifact clues

A design note captured from a grilling session, durable for a later `/to-prd`. This is the one resolved Clean borrow-from-Mole branch that did not warrant an ADR (the decisions are reversible, unsurprising, and low-stakes), so its decisions are recorded here instead.

## Goal

Borrow Mole's `purge` idea (surfacing large rebuildable project build artifacts like `node_modules`, `target`, `dist`) into Foal — but strictly in the read-only lane, respecting the existing glossary term **Project artifact clue**:

> A review clue for rebuildable project directories or build outputs that Foal may surface only through explicit analysis or future opt-in flows. _Avoid_: default project scan, default clean candidate.

The hard constraint: a default `foal clean --dry-run` must **not** go hunting for `node_modules` across the disk. Mole itself keeps `purge` as a separate command and `mo clean` only carries a hint pointing at it.

## Decisions

- **Ownership split (analyze labels; clean points).** Project-artifact awareness lives in `foal analyze`, which already performs a read-only directory walk and reports top children by size. When a top child's name matches a recognized rebuildable-artifact set, `analyze` labels it. `foal clean --dry-run` carries only a single static pointer clue directing the user to `foal analyze <path>`. This matches Mole's split (deep scan in `purge`/`analyze`, pointer in `clean`) and respects the glossary's "only through explicit analysis."

- **No deep scanning in analyze.** `analyze` stays "top children by size"; it does NOT recurse looking for nested artifacts. Going deeper would turn `analyze` into a project scanner (scope creep). Limitation: only artifacts that happen to be a top child of the analyzed path are labeled (e.g. `analyze <project-root>` catches a direct `node_modules`; `analyze ~/Projects` would not catch artifacts two levels down). Documented as acceptable for v1.

- **Recognized artifact set (high-confidence, v1).** `node_modules`, `target`, `dist`, `build`, `.build`, `.next`, `__pycache__`. Deliberately excludes generic `bin`/`obj` to avoid noise; the set can be widened later. Because this is a read-only label (not a deletion), over-labeling is low-risk.

- **clean's pointer clue is a presentation-only constant.** It fills the existing `ReviewClue` read-model slot with a constant entry (name "Rebuildable project artifacts", empty path, details pointing at `foal analyze <path>`). It is injected in the clean preview read-model projection and rendered in human output and the TUI. It is NOT added to the clean `Result` JSON contract — it is constant UX guidance, not data (consistent with how the suggestions static safety note is handled).

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

## Later direction (ADR 0019)

An independent **Project artifact purge flow** (separate command or dedicated flow, not default Clean discovery) may reclaim rebuildable artifacts under a user-supplied root after selection and confirmation. That work needs its own design/PRD; it does not reopen default disk-wide project scanning.

## How to turn this into a PRD later

In a new conversation: "Read `docs/plan/project-artifact-clues.md` and `CONTEXT.md` (Project artifact clue), then `/to-prd`."
