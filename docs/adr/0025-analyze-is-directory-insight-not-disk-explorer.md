---
status: superseded by ADR-0034
---

# Analyze is directory insight, not a disk explorer or cleanup surface

Foal's `analyze` command is a read-only **directory insight** tool: measure one analysis root (default CWD), report totals and top children by size, and attach only proven high-confidence classification clues (today: top-level `project_artifact_clue`). It is deliberately **not** a Mole-style interactive disk explorer, ad hoc Trash/delete surface, Clean opportunity scanner, or recursive project-artifact finder. Nested rebuildable discovery and permanent reclaim stay on `foal purge`; Clean keeps only presentation pointers.

## Why this shape

- **Trustworthy size insight first.** Users need honest “how big is this tree / who are the largest children” answers. Presenting incomplete scans as complete, or mixing insight with deletion, destroys that trust.
- **Foal boundaries.** Clean owns conservative cleanup categories; purge owns explicit-root project artifacts; Analyze must not become a third cleanup engine or a back door into disk-wide hunting.
- **Mole is inspiration, not parity.** Mole `mo analyze` opens a curated overview, supports enter-directory navigation, and can move items to Trash. Copying that would fight Foal's preview-first, no-TUI-owned-deletion, and no-feature-parity defaults.

## Locked rules (with plan detail in `docs/plan/analyze-directory-insight.md`)

- Single analysis root; omit path → CWD; reject dangerous roots via the same user-scan-root policy family as purge (`ValidateUserScanRoot` after absolute resolution).
- Descendant inspection ceiling aligned with the 100,000 Opportunity inspection limit; over-limit or cancel → `status=incomplete`, never estimated full-tree size.
- Human and JSON surfaces share the same core insight; human output is not JSON-only detail.
- Protection rules do not suppress Analyze measurement (cleanup-side only).
- Analyze does not write History.
- TUI is a read-only Command viewer (default CWD, simple path edit allowed) with no cleanup affordance; purge handoff is copy-only when artifact clues are present.
- Near-term classification remains only `project_artifact_clue` with the existing allowlist; top children fixed at 10.

## Considered options

- **Mole-like explorer + Trash from Analyze** — rejected: execution model, safety, and product identity cost; contradicts TUI non-ownership of deletion.
- **Analyze as cleanup opportunity / large-file recommendations engine** — rejected: collides with Clean opportunity and Potential space semantics.
- **Deep nested artifact labeling inside Analyze** — rejected: becomes a project scanner; purge already owns nested discovery under explicit roots.
- **Protection as Analyze deny-list** — rejected: hides disk facts; protection authorizes nothing and should not reshape read-only insight.
- **Defer human report / keep JSON-only detail** — rejected for product trust: directory insight must be usable without `--json`.

## Consequences

- Contributors should not add delete, multi-select reclaim, overview-as-launcher, or nested project scanning to Analyze without reopening this ADR.
- CLI contract work (dangerous root, incomplete status, human report) precedes TUI viewer work; TUI must project shared Analyze results rather than invent a second engine.
- Comparisons to DaisyDisk/Mole analyze “GB freed from the explorer” are intentionally out of scope; copy should describe directory insight and hand off purge/clean explicitly.
- `docs/plan/project-artifact-clues.md` remains valid for the label/point/purge ownership split; this ADR freezes Analyze's broader product identity around that split.
