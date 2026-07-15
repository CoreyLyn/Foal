# Clean TUI default category is selected but removable

The Clean TUI presents default and opt-in cleanup categories as one explicit per-run selection. A default category starts selected, preserving Foal's conservative recommendation, but the user may clear it before confirmation. Confirmation authorizes exactly the selected category identifiers: the TUI must not perform hidden default cleanup, and its selected-size total must include only checked categories with completed measurements.

This deliberately differs from the additive CLI contract in ADR 0008, where `foal clean --execute` always includes default candidates and `--opt-in` adds categories. It also qualifies ADR 0009's TUI model: ADR 0009 assumed that the TUI selected only opt-in identifiers while shared execution implicitly retained defaults; the new exact TUI selection includes default identifiers and may omit them. The CLI remains unchanged. The TUI uses an explicit shared Clean execution request that can include or exclude defaults while still passing identifiers rather than preview paths; execution fresh-resolves the selected categories and retains every other ADR 0008/0009 safety invariant.

## Considered options

- **Keep the default category selected and locked** - rejected because the displayed selection and total would not represent the user's complete authorization.
- **Start the default category unselected** - rejected because it would hide Foal's conservative default recommendation and make the TUI diverge unnecessarily at entry.
- **Reuse the additive CLI request unchanged** - rejected because clearing the visible default category would still clean it, creating hidden behavior.

## Consequences

The TUI selection model and confirmation summary cover canonical default and opt-in identifiers together, while `Clean opt-in selection` remains the opt-in subset. Permission-boundary identifiers, review-only evidence, aliases, group tokens, and paths cannot enter an exact plan. Clearing a default category can only narrow cleanup. The shared execution layer must distinguish an explicit TUI category plan from the CLI's additive opt-in plan without creating a second resolver or deletion engine.
