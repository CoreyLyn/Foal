# Active Clean execution owns Ctrl+C until the final Result

> **Status note (ADR 0018):** Cooperative cancellation during active execution extends to both Recycle Bin and permanent deletion. Completed work of either action is not rolled back; interrupted permanent candidates may warn about partial irreversible removal and contribute zero permanent bytes.

Outside active confirmed Clean execution, Ctrl+C remains Bubble Tea's interrupt exit path with status 130 as decided by ADR 0003. During active Clean execution, Ctrl+C instead requests cooperative cancellation and the TUI remains attached until shared Clean returns its final Result; repeated Ctrl+C does not force exit, while Escape, `b`, and `q` remain inactive. The final Result and normal history must preserve completed, skipped, failed, and item-level `context_canceled` outcomes because cancellation does not roll back completed Recycle Bin or permanent deletion operations; the result view may project the latter as a path-free canceled category outcome.

This deliberately qualifies only ADR 0003's universal Ctrl+C consequence. The active execution exception prevents the terminal shell from abandoning an in-flight partial cleanup before Foal can present and retain its authoritative outcome; every other Bubble Tea terminal-restoration and interrupt behavior remains unchanged.

## Considered options

- **Always exit 130 immediately on Ctrl+C** - rejected because confirmed execution may already have moved items and still needs to return and record its partial authoritative Result.
- **Let a second Ctrl+C force exit** - rejected because it recreates the same abandoned-result ambiguity after cancellation has begun.

## Consequences

The execution view must acknowledge the cancellation request and state that completed operations will not be rolled back. Once the final Result arrives, the normal result view becomes active, includes any path-free canceled category projection, and lets the user return to the menu or quit through its ordinary keys.
