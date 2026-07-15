# Clean TUI history records exact selection structurally

Confirmed Clean TUI execution records path-free structured provenance in command parameters: `surface=tui`, `selection_mode=exact`, and the canonical `selected_categories` identifiers in stable display-and-scan order rather than user toggle order. It does not synthesize a `clean --execute --opt-in ...` command line, because the CLI's additive defaults cannot represent a TUI plan that explicitly omits a default category. Normal item-level execution history remains authoritative and unchanged.

The history JSON change is additive: the new fields are optional, CLI sessions retain their existing `args`, and older readers may ignore fields they do not understand. TUI provenance never contains preview candidate paths; actual executed or skipped item paths continue to follow the established Clean execution-history contract.

## Considered options

- **Record synthetic CLI arguments** - rejected because no public CLI invocation expresses exact default inclusion or omission, so the record could claim behavior that did not occur.
- **Omit the TUI selection from history** - rejected because a future reader could not reconstruct which categories the user authorized, especially when a default was cleared.
- **Add a hidden CLI flag solely for history formatting** - rejected because it would create a misleading unsupported command surface instead of describing the real TUI provenance.

## Consequences

History command parameters gain optional `surface`, `selection_mode`, and `selected_categories` fields. TUI tests must lock canonical identifier ordering, exact default omission, absence of candidate paths, and backward-compatible CLI serialization.
