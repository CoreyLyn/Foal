---
status: accepted
---

# Analyze TUI is a read-only, on-demand disk browser

Foal Analyze keeps its non-mutating directory-insight identity, but its TUI is now an interactive Windows disk browser instead of a scrollable Command viewer. The landing view lists local fixed and readable removable drives, focuses `C:` when present, and starts filesystem measurement only after the user enters a drive or directory. This supersedes ADR-0025's rejection of volume roots and directory drill-down because the read-only boundary can be preserved without making Analyze a cleanup surface.

## Boundary

- Analyze may explicitly measure a local volume root in CLI, JSON, or TUI. No-argument CLI behavior remains CWD. This exception never relaxes Clean or Purge dangerous-root validation.
- TUI navigation may enter readable local system directories. Reparse points, mapped/network drives, optical drives, UNC roots, and device paths remain non-navigable.
- Analyze never deletes, selects cleanup targets, launches Purge, writes History, requests elevation, or exposes file-open or preview actions.
- `project_artifact_clue` remains a label on direct children only. A Purge command hint appears only when the current location independently passes Purge root validation.

## Measurement model

- Drive entry reads only volume metadata. Entering a location enumerates all direct children and starts independent recursive measurement of those children; no sibling drive or invisible location is prefetched.
- Each child has a 100,000-descendant limit. Two workers run concurrently; focused queued children receive next-task priority without preempting active work.
- Child states are `scanning`, `complete`, `partial`, `incomplete`, and `skipped`. Partial and incomplete sizes are observed lower bounds. Their percentages are explicitly approximate observed shares, never guaranteed lower bounds.
- Logical file bytes remain the shared CLI, JSON, and TUI size metric. They are not claimed to equal allocated or used disk space.
- Rows continuously re-rank by latest observed bytes while the cursor stays bound to the selected path. Leaving a location cancels unfinished work; session memory retains completed results, returning resumes missing work, and refresh discards that location's cache.

## Consequences

The TUI needs a browse-specific projection and incremental observation API in the shared Analyze package rather than reusing the generic Command viewer. CLI and JSON retain their fixed Top 10 result shape and global 100,000-descendant limit, while explicit local volume roots become valid. The TUI may show every direct child because navigation would otherwise hide smaller directories. Dynamic bars and volume free-space metadata are presentation only and do not create reclaimable-space claims.
