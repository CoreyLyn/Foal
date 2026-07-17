# Plan: Clean TUI dual-channel presentation

Design note from a grilling session. Presentation-only polish for the category-first Clean TUI: make large reclaimable sizes scannable and permanent-delete risk visible without conflating "large" with "dangerous".

## Goal

Improve Clean TUI UI/UE so users can:

1. Spot high-byte categories and selected totals at a glance.
2. See planned deletion action risk before and during confirmation.
3. Keep Foal's safety-first color language (red ≈ irreversible risk, not size).

No change to discovery, JSON contracts, selection semantics, planned actions, path-free rules, or shared Clean execution.

## Dual-channel encoding

| Channel | Encodes | Means | Must not mean |
|---------|---------|-------|----------------|
| **Magnitude** | Trusted byte tokens only | How large the measured/affected size is | Danger, authorization, freed disk |
| **Risk** | Action grouping, copy, markers, footer notice | Recoverable vs permanent deletion | "This is big" |

### Magnitude emphasis

- Absolute thresholds, 1024-based, aligned with `cleanFormatBytes`:
  - **Neutral:** `< 100 MiB`
  - **Attention (amber/yellow):** `≥ 100 MiB` and `< 1 GiB`
  - **Strong (orange, not pure red):** `≥ 1 GiB`
- No magnitude color for `0`, empty, skipped, or pending/unfinished bytes.
- Optional bold on the byte token; when `NO_COLOR` or color is unavailable, keep bold (or plain) without magnitude hues.

### Risk channel

- Confirmation remains the primary risk surface: Permanent vs Recycle Bin groups, irreversible permanent warning (red/bold).
- Preview rows show catalog-projected markers: `perm` / `bin` (not whole-row red).
- When the exact selection includes any permanent category, footer adds a full sentence such as `includes permanent deletion`.
- Hints legend: `perm=permanent · bin=Recycle Bin`.

## Surfaces (v1)

| Surface | Magnitude on bytes | Notes |
|---------|-------------------|--------|
| Preview category row | Yes (complete/partial measured bytes) | Plus `perm`/`bin`; no whole-row size tint |
| Selected total line | Yes | Permanent notice when selection includes permanent |
| Focused detail | Same rule as row if implemented cheaply | Optional follow |
| Confirmation | Yes on measured byte numbers | Risk red reserved for permanent warning/headings |
| Execution progress | No | Avoid flashy recolor during in-progress states |
| Result | Yes on successful affected-style byte tokens | Never label aggregate as freed disk space |

## Layout polish in v1

- **Byte column alignment** on preview lists for easier magnitude scan.
- **Confirmation warning emphasis** (bold/risk color) for irreversible permanent copy.
- **No** reorder by size (display order stays catalog/scan order).
- **No** completion celebration, byte-derived percentages, retry/rescan, or full result success-green / fail-red palette in this slice.

## Engineering constraints

- Plain-text frame remains the source of truth; restricted token styling is applied after plain composition.
- Tests assert plain fragments (`1.2 GB`, `perm`, `includes permanent deletion`), not ANSI sequences.
- Existing whole-line styles (cursor reverse, section headings) remain; magnitude/risk tokens are a narrow mid-line exception only for agreed token kinds.
- TUI never chooses or overrides Planned deletion action; markers and notices project catalog/shared selection state only.

## Non-goals

- New cleanup categories or discovery.
- JSON / History contract changes.
- Changing initial selection defaults or confirmation double-Enter flow.
- Secure-erasure claims, elevation, process stopping.
- Sorting categories by size or promoting large rows out of stable order.

## Likely implementation slices

1. Pure helpers: magnitude tier from `int64` bytes; format+style byte token; `perm`/`bin` from planned action; selection permanent notice line.
2. Preview/confirm/result view wiring: align bytes, inject markers, apply token styles, strengthen confirmation warning.
3. Tests: tier boundaries, no color on zero/pending, plain-frame assertions, footer notice only when selection includes permanent, `NO_COLOR` bold fallback if wired.
4. Keep `CONTEXT.md` and this plan aligned if thresholds or markers change.

## References

- `CONTEXT.md`: Clean TUI dual-channel presentation, magnitude emphasis, planned-action marker, permanent-selection notice, TUI restricted token styling.
- ADR 0023: dual-channel presentation and restricted token styling.
- `docs/plan/clean-tui-category-first-interaction.md`: interaction model preserved.
- ADR 0018: planned action is catalog-owned; TUI displays and authorizes disclosure only.
