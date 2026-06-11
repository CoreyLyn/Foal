# User protection entries are deny-only and enforced at the pathsafe chokepoint

Foal lets a user list paths to protect from cleanup in a plain-text config file (`%APPDATA%\Foal\protection.txt`, overridable via `FOAL_PROTECTION_FILE`). Each non-comment line is an absolute path that protects itself and its entire subtree (prefix match). These entries are consulted inside the `pathsafe` validation chokepoint — the same gate that rejects `c:\windows` and reparse points — so a protected path is skipped during both `foal clean --dry-run` preview and `foal clean --execute`, and any Review suggestion or opportunity whose path falls under a protected subtree is suppressed for free.

The entries are **deny-only**: they can only remove paths from cleanup, never add them. There is no allowlist mode that authorizes cleaning a path Foal would not otherwise touch.

`pathsafe` stays a pure, dependency-free validator: the config is loaded in the composition layer (clean/cli) and injected as a `Validator` value, rather than `pathsafe` reading the filesystem itself.

## Considered options

- **A "whitelist" that both protects and authorizes cleaning (Mole's word for it)** — rejected: an allowlist that authorizes cleaning is a backdoor around the frozen conservative-default boundary (`CONTEXT.md`: *Protection rules — Avoid: whitelist, allowlist-only model*). Mole's `is_path_whitelisted` only ever protects, so the deny-only stance matches Mole's actual behaviour even though the word differs.
- **Filter protected paths only in the clean preview layer** — rejected: `foal clean --execute` runs an independent scan + pathsafe validation, so preview-only filtering would let execute diverge from what the preview showed.
- **Let `pathsafe` load the config file itself** — rejected: breaks `pathsafe`'s purity and testability and makes validation order-dependent on filesystem state.

## Consequences

The config file format (newline-delimited absolute paths, `#` comments) becomes a user-facing contract that is awkward to change once people rely on it. A future contributor may be tempted to extend protection entries into an allowlist that authorizes new cleanup ("the user opted in, so it's safe") — that is explicitly out of bounds: protection is a one-way deny. Over-broad entries are fail-safe (they protect more, never delete more). Invalid lines (relative, UNC, 8.3 short-name) are skipped with a notice rather than failing the run.
