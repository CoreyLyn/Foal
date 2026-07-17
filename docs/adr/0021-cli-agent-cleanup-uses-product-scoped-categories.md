# CLI agent cleanup uses product-scoped categories

## Status

Accepted for P1 design. This ADR fixes category granularity only; it does not approve any CLI agent artifact for cleanup or add a catalog entry.

## Context

P1 research covers Claude Code, Codex CLI, Grok Build, Antigravity CLI, and Gemini CLI. Their local roots mix materially different data: credentials, configuration, conversation history, recovery state, plugins, downloaded runtimes, logs, lookup caches, and regenerable artifacts. Some products also share state with a desktop application or provide their own session, plugin, or update lifecycle commands.

A single executable `cli-agents` category would hide product-specific impact and make one policy appear transferable across unrelated layouts. It would also prevent independent selection and make future allowlist changes unnecessarily coupled.

## Decision

- Model each supported CLI agent as an independent canonical Clean category only after its exact eligible children and exclusions are proven.
- Give each category its own discovery policy, runtime/concurrency gate, planned deletion action, impact text, and tests.
- A future `cli-agents` token may be a convenience selection alias that expands to the independently registered categories. It is never an executable category, never owns candidates, and never defines shared path or deletion semantics.
- Do not infer eligibility from a top-level agent home, or from names such as `cache`, `tmp`, `logs`, or `downloads`.
- Do not reuse desktop Application cache policy merely because a CLI agent has a companion GUI product.

## Consequences

- Users can inspect and deselect each agent independently in CLI/TUI surfaces.
- Evidence or implementation for one agent does not unblock another.
- Shared catalog plumbing may be reused, but product-specific policy remains explicit.
- P1 research must still decide whether any precise artifact is valuable and safe enough to become a category; current research alone creates no cleanup behavior.
