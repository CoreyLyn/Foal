# Clean TUI reports path-free Protection exclusions

The Clean TUI eager preview distinguishes a category with Protection-suppressed candidates from one with no candidates. When every discovered candidate is protected, the category reports a path-free `skipped` outcome and is disabled; when unprotected candidates remain, it reports `partial`, stays selectable, and counts only the unprotected candidates and bytes. Focused detail may expose a generic stable reason and excluded count, but never a protected path, protected bytes, or a raw error message that may embed a path.

This qualifies ADR 0005's rule that protected review discoveries disappear before read models and downstream projection. The protected candidates themselves, their paths, and their bytes still disappear before reclaimable totals, selection, detailed lists, history, and execution; the only new projection is path-free category-level evidence that Protection, rather than absence, limited the scan. Protection remains deny-only and never authorizes cleanup.

## Considered options

- **Render Protection-suppressed categories as empty** - rejected because it falsely states that no candidate was found and hides an active safety boundary.
- **Show the protected paths or bytes** - rejected because it reverses the privacy and suppression purpose of Protection rules.
- **Suppress every trace of the category** - rejected because the category-first TUI must distinguish a completed absence from a deliberate safety exclusion.

## Consequences

Shared Clean preview scanning must retain a path-free exclusion count and stable reason code before discarding protected candidate details. Any diagnostic text reaching the primary TUI must be constructed from stable category state rather than forwarding raw path-bearing OS errors.
