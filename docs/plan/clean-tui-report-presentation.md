# Plan: Clean TUI report presentation

A design note captured from a grilling session, durable for a later `/to-prd`.
The goal is to make the Clean TUI feel closer to a scan-friendly terminal report
while preserving Foal's Windows-native safety model and read-only TUI boundary.

## Goal

Improve the existing Clean TUI presentation layer only.

The first slice should render the existing Clean preview read model as grouped
report content rather than a tool-like debug browser. It must not change Clean
discovery, JSON contracts, history behavior, detailed-list behavior, execution
eligibility, path-safety rules, or protection filtering.

## Decisions

- The scope is **Clean TUI report presentation** only.
- The TUI may use Unicode markers, but markers are presentation-only and must
  emphasize safety semantics rather than cleanup authorization.
- The Clean view uses a compact title, not the Foal main-menu banner.
- The top of the Clean view should show only identity and preview state, such as
  `Foal Clean` and `Preview only - no files changed`.
- Summary totals belong at the bottom, not the top.
- The report uses the existing Foal categories only:
  - `System`
  - `User essentials`
  - `Browsers`
  - `Developer tools`
  - `Project artifacts`
  - `Protection`
  - `Summary`
- Do not add new categories such as Applications, Cloud, Virtualization, Large
  files, or System Data clues until Foal has real read-model data for them.
- Default item labels should be compact: short name, marker, size, count, and
  state where useful.
- Full paths and detailed contract fields should appear through expansion or
  existing detailed review surfaces, not in the default list.
- Browser cache opportunities remain summary-only by default; profile and cache
  directory paths should not appear in the default Clean TUI summary.
- `c copy` continues to copy only default candidate paths.
- `f filter` remains available but becomes an auxiliary focus tool. The default
  view is the full report.

## Status marker intent

Suggested marker semantics:

- `●` default candidate: previewed default candidate, counted in Potential
  space, executable only through the existing explicit Recycle Bin path.
- `○` skipped-by-default opportunity: review-only observed opportunity, not
  counted in Potential space, not executable from the TUI.
- `◇` review clue or review suggestion: manual investigation or external tool
  review prompt.
- `⊘` running application, protected path, or permission boundary skip.
- `!` recoverable diagnostic or incomplete inspection.
- `✓` empty or successfully loaded review state only; never cleanup
  authorization or deletion completion.

Exact symbols can change during implementation if terminal compatibility is
poor, but the semantics should remain stable.

## Filter intent

The filter should be simplified around user intent:

- `all`
- `actionable preview`: default candidates plus skipped paths and errors
- `review-only`: opportunities, review clues, review suggestions, and running
  application skips
- `diagnostics`: recoverable errors, incomplete inspections, and protection
  diagnostics

Filtering must not alter totals, read-model data, or copy behavior.

## Bottom summary

The bottom summary should preserve preview-first wording:

```text
Dry-run complete
No files changed.
Potential space: ...
Observed opportunity bytes: ... (not counted as Potential space)
Default candidates: ... | Skipped: ... | Diagnostics: ...
Run foal clean --execute to move default candidates to the Recycle Bin.
```

Do not use `Cleanup complete` for a Clean TUI preview view.

## Non-goals

- No new cleanup discovery.
- No new JSON fields or JSON status changes.
- No new execution affordance.
- No TUI-owned cleanup, uninstall, path-safety, or protection logic.
- No history session or detailed-list write from browsing the TUI.
- No copied review-only path list.
- No Mole-for-Windows positioning or Mac-specific category vocabulary.

## Likely implementation slices

1. Add compact item label rendering over `clean.PreviewReportCategories` or a
   neighboring presentation helper, preserving plain-text assertions.
2. Rework `internal/cli/tui_clean.go` header, footer, filter names, and default
   report layout.
3. Update TUI tests for marker semantics, compact labels, bottom summary, copy
   boundaries, filter behavior, and browser summary privacy.
4. Keep docs aligned after implementation.

## References

- `CONTEXT.md`: `Clean TUI report presentation`, `TUI status markers`, `TUI
  compact item labels`, `Report category`, `Read-only TUI action model`.
- `docs/adr/0003-bubble-tea-v2-for-tui-shell.md`: Bubble Tea owns terminal
  mechanics; the TUI remains read-only.
- `docs/adr/0007-clean-browser-cache-discovery-requires-running-application-detection.md`:
  report categories are presentation groupings only; browser cache bytes remain
  skipped-by-default review data.
