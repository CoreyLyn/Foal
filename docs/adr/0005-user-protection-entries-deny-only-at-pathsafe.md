# User protection entries are deny-only and enforced at the pathsafe chokepoint

Foal lets a user list paths to protect from cleanup in a plain-text config file (`%APPDATA%\Foal\protection.txt`, overridable via `FOAL_PROTECTION_FILE`). Each non-empty, non-comment line is an absolute local path that protects itself and its entire subtree. Matching is normalized, case-insensitive, and path-component-aware. Relative paths, UNC paths, and paths containing 8.3 short-name segments are invalid and produce structured, recoverable diagnostics.

These entries are consulted inside the configured `pathsafe` validation chokepoint — the same gate that rejects `c:\windows` and reparse points — so a protected default candidate is skipped during both `foal clean --dry-run` preview and `foal clean --execute`. Clean's review-only projection uses the same configured validator to suppress protected user-temp opportunities and Review suggestions with resolved cache paths before totals, read models, detailed lists, or history are produced. Suggestions without a resolved cache path are not matched by interpreting command text.

The entries are **deny-only**: they can only remove paths from cleanup, never add them. There is no allowlist mode that authorizes cleaning a path Foal would not otherwise touch.

`pathsafe` stays a pure, dependency-free validator: the config is loaded in the composition layer (clean/cli) and injected as a `Validator` value, rather than `pathsafe` reading the filesystem itself.

The default file is optional: if it does not exist, Foal runs with no user-defined Protection rules. If `FOAL_PROTECTION_FILE` selects a file that cannot be loaded, or the selected file is not valid UTF-8, Clean fails closed before scanning or execution.

## Considered options

- **A protection entry that also authorizes cleaning** — rejected: cleanup authorization would be a backdoor around the frozen conservative-default boundary. Protection rules only deny; they never make a path eligible for cleanup.
- **Filter protected paths only in the clean preview layer** — rejected: `foal clean --execute` runs an independent scan + pathsafe validation, so preview-only filtering would let execute diverge from what the preview showed.
- **Let `pathsafe` load the config file itself** — rejected: breaks `pathsafe`'s purity and testability and makes validation order-dependent on filesystem state.

## Consequences

The config file format (newline-delimited absolute paths, blank lines ignored, `#` comments) becomes a user-facing contract that is awkward to change once people rely on it. A future contributor may be tempted to extend protection entries into a cleanup authorization mechanism ("the user opted in, so it's safe") — that is explicitly out of bounds: protection is a one-way deny. Over-broad entries are fail-safe (they protect more, never delete more). Invalid lines are skipped with Protection diagnostics; selected-file load failures remain fail-closed.
