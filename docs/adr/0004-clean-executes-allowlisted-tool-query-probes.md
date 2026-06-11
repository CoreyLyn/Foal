# Clean executes allowlisted tool query probes to resolve Review suggestion paths

`foal clean --dry-run` (and the Clean TUI) may execute a small, built-in allowlist of developer tools using only their read-only query subcommands (for example `npm config get cache`, `go env GOCACHE`, `dotnet nuget locals all --list`) to resolve the cache path shown in a Review suggestion. This is the one deliberate exception to Clean's otherwise execution-free preview. Each probe is gated by `exec.LookPath` (only installed tools are spawned), is restricted to non-mutating query subcommands, never runs a tool's cleanup command, and is bounded by a per-call context timeout (2s). Probes never run during `foal clean --execute`.

We chose this because Review suggestions point users at an external tool's own cleanup command, and the most accurate displayed path comes from asking the tool itself — config-file customizations (e.g. `.npmrc`) are invisible to environment-variable and default-path resolution. The suggested command stays correct regardless of the resolved path, so the probe buys display accuracy and existence-gating, not authorization.

## Considered options

- **Pure env + default-path resolution, no execution** — rejected: preserves the no-execution and no-timeout posture but misses tool-config-customized cache paths and cannot confirm a cache actually exists without guessing. Lower fidelity for a tool the user explicitly installed.
- **Unbounded execution of any matching PATH binary, no timeout (Mole's native style)** — rejected: a hijacked `npm`/`pip` on PATH would run during a read-only preview, and a wedged tool could hang `foal clean --dry-run` indefinitely. Mole itself wraps every such call in `run_with_timeout` for exactly this reason.

## Consequences

This reverses two prior postures: Clean previously executed no third-party binaries during scanning, and `CONTEXT.md` deliberately avoided wall-clock timeouts for opportunity inspection (a deterministic descendant-count ceiling was chosen instead). Those decisions still hold for filesystem inspection; this ADR scopes the exception narrowly to allowlisted, query-only, timeout-bounded probes used solely for Review suggestion paths. A future "Clean must never execute anything" hardening pass would have to remove tool-queried paths and fall back to env/default resolution — treat the probe as an intentional, bounded exception, not an oversight. The allowlist (`npm, pnpm, yarn, bun, pip, uv, conda, go, dotnet/nuget, corepack, mise`) is an execution surface: every addition must ship with both a read-only query subcommand and a native cleanup command, or it does not belong here.
