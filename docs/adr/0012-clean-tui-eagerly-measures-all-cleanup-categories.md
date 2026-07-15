# Clean TUI eagerly measures all cleanup categories

Entering the Clean TUI starts a read-only, sequential preview scan of every canonical default and opt-in cleanup category, including developer-tool caches, before the user selects any opt-in category. This makes category results and selected-size totals immediately interactive while keeping scan intent separate from cleanup authorization: opt-in selection still starts empty, permission-boundary entries remain notices, and the TUI receives path-free category results rather than an execution manifest. Review suggestions and review clues are not cleanup categories: the category-first Clean TUI does not render them as duplicate rows or run external tool query probes solely to discover them, while both retain their existing non-TUI preview contracts.

This qualifies ADR 0008's selected-only resolution rule. CLI dry-run and every Execute path continue to resolve only explicitly opted-in categories; only the Clean TUI eager preview scan may measure unselected opt-in categories. ADR 0009 remains otherwise unchanged: the TUI does not own resolution or deletion, passes only selected category identifiers at confirmation, and confirmed execution fresh-resolves and immediately validates its own candidates through the shared Clean path. Preview scanning and selection write no history or detailed candidate list, never run a third-party cleanup command, and never weaken Recycle Bin-only execution, Protection rules, running-application gates, or aggregate capacity checks.

## Considered options

- **Resolve a category only after selection** - rejected because every toggle would retain a blocking rescan and selected totals would not update immediately.
- **Reuse eager-preview paths during execution** - rejected because preview paths are stale, non-authoritative evidence; Execute must resolve selected category identifiers fresh.
- **Let the TUI implement category scanners directly** - rejected because it would create a second cleanup engine and split safety rules across callers.
- **Append non-executable Review suggestions to the category list** - rejected because they cannot participate in selection or totals, duplicate many canonical developer-cache categories, and would retain external probe latency in the primary cleanup flow.

## Consequences

Shared Clean preview scanning must expose path-free per-category observations suitable for incremental TUI rendering while keeping progress outside Result and the JSON contract. A partial category may retain only safely completed sibling candidates and bytes with diagnostics for excluded siblings; empty, skipped, incomplete, and failed categories contribute no invented reclaimable bytes. Selection may change provisionally while scanning; selected byte totals use only terminal complete or partial summaries, and selection changes never restart scanning. Confirmation still begins a separate fresh execution scan whose final candidates and bytes may differ from the preview.
