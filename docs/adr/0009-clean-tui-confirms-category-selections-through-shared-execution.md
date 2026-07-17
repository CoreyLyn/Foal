# Clean TUI confirms category selections through shared Clean execution

> **Status note (ADR 0018):** Confirmed TUI execution through shared Clean remains mandatory. Catalog-owned planned actions may be `move_to_recycle_bin` or `delete_permanently`; the TUI does not choose the method. Permanent categories require the strengthened confirmation authorization equivalent to CLI `--allow-permanent`. Aggregate Recycle Bin capacity pre-checks apply only to Recycle Bin work. ADR 0013 exact selection and ADR 0018 initial permanent-category selection further qualify the "opt-in unselected by default" phrasing below.

The Clean TUI may execute cleanup after the user explicitly selects opt-in categories for the current run and confirms the resulting preview. The TUI passes only category identifiers to the shared Clean execution path; it never passes previewed candidate paths, owns candidate resolution, or implements deletion or path-safety decisions. Execute performs fresh Opt-in candidate resolution and immediate path validation, so the executed set may differ from the preview.

The existing safety model remains mandatory: opt-in is unselected by default, default candidates stay conservative, deletion is Recycle Bin-only, Protection rules remain deny-only, Running application detection remains fail-closed, no process is stopped, and Foal never elevates automatically. A select-all action is an explicit per-run opt-in, not a new default. Before a confirmed multi-item run begins, Clean must perform an aggregate per-volume Recycle Bin capacity pre-check that accounts for the selected candidates as a group; failure to establish recoverability skips execution rather than risking permanent deletion. Category-first browsing and selection do not create history, while confirmed execution records the normal Clean execution history plus optional path-free TUI provenance (ADR 0016).

This decision qualifies ADR 0008. It supersedes only ADR 0008's TUI-specific read-only consequence and its deferral of aggregate multi-item Recycle Bin capacity protection; the CLI opt-in contract, fresh-scan rule, Recycle Bin-only execution, frozen defaults, no automatic elevation, and no third-party cleanup-command execution remain unchanged. Uninstall and every non-Clean TUI command remain read-only unless separately designed.

## Considered options

- **Keep the Clean TUI permanently read-only** - rejected because it forces users to leave the primary interactive review surface before reclaiming the space they just reviewed.
- **Let the TUI execute the previewed path list** - rejected because preview data is stale and non-authoritative; execution must resolve selected categories again.
- **Put candidate resolution or deletion logic in the TUI** - rejected because it would create a second cleanup engine and split safety invariants across callers.
- **Select every opt-in category by default** - rejected because it would turn explicit opt-in into an implicit aggressive default.

## Consequences

The Clean TUI has category-level selection, explicit confirmation, executing, and result states over the shared Clean module. The shared Clean read model exposes category summaries and opt-in reclaimable bytes without turning paths into an execution manifest. Shared execution emits optional observation-only progress for fresh scanning, aggregate Recycle Bin safety checks, Recycle Bin operations, and completion; this progress is outside the JSON result and cannot authorize execution. Aggregate per-volume capacity protection is enforced before adapter calls. Cancellation after confirmation does not imply rollback; completed, skipped, and failed outcomes returned by Clean remain authoritative and are retained in normal history semantics.

ADR 0013 qualifies this ADR's opt-in-only selection model: defaults start selected but are removable, and the TUI confirms an exact canonical category set without silently restoring an omitted default. ADR 0014 defines Ctrl+C ownership during active execution, and ADR 0016 adds path-free structured exact-selection provenance to TUI execution history without fabricating CLI arguments.
