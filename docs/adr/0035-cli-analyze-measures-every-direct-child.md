---
status: accepted
---

# CLI analyze measures every direct child with its own descendant ceiling

ADR-0034 kept one global 100,000-descendant ceiling for `foal analyze` CLI/JSON while the TUI browser measured each direct child independently. On a real volume root that global ceiling was exhausted depth-first inside `C:\Program Files`, so `C:\Users` (164 GB) and `C:\Windows` (93 GB) never appeared in `top_children` or `skipped`; only a root-level `status=incomplete` hinted at the omission, and the ranked Top 10 looked authoritative. This supersedes ADR-0034's consequence that "CLI and JSON retain their fixed Top 10 result shape and global 100,000-descendant limit" and the analyze plan's "preserve the CLI's one global 100,000-descendant limit".

## Decision

- CLI and JSON analyze enumerate every direct child of the root and measure each directory child independently with its own 100,000-descendant ceiling, using the same shared measurement engine as the TUI browser (bounded concurrency; results merged in directory order so output is deterministic).
- Each `top_children` entry gains an additive `state` field with the browse vocabulary: `complete`, `partial` (readable, but some descendants were omitted), `incomplete` (its ceiling or cancellation stopped traversal), or `skipped` (unreadable or a reparse point). Partial and incomplete bytes are observed lower bounds; the human report prefixes them with `>=` and names the state.
- Root `status` is `incomplete` when any child is incomplete or the run was canceled; permission omissions alone make a child `partial` without changing root status (unchanged semantics).
- The result shape is otherwise unchanged: `status`, `root`, `totals`, fixed Top 10 `top_children`, path-bearing `skipped`, `elapsed_ms`. Reparse points (symlinks, junctions, mount points, cloud placeholders) follow the shared engine's attribute-based rule: never traversed, listed in `skipped` with reason `reparse_point`.

## Consequences

- A volume root now ranks its real largest children, each honestly marked; no child is silently omitted. Total inspected work is bounded by ceiling × number of direct children rather than one global ceiling.
- CLI totals exclude reparse files and directories the same way the TUI does, so the two surfaces agree on a location.
- Logical-byte semantics, root validation, read-only boundaries, and "no History, no elevation" are unchanged.
