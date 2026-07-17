# Plan: Clean deletion policy

This is the implemented Clean deletion policy. Shared Clean assigns each executable category an explicit planned action, executes mixed actions with per-run permanent authorization, and records split actual-action totals. ADR 0018 records the decision and this document is the canonical category matrix.

## Core policy

- Every executable canonical cleanup rule must explicitly declare exactly one planned action: `move_to_recycle_bin` or `delete_permanently`. Registration must fail when the action is missing or unknown.
- A category may use `delete_permanently` only when its exact candidate layout proves that all surviving content is regenerable or re-downloadable and excludes user-authored, diagnostic, configuration, history, and login state. Age, a cache-like name, or a Temp location is not enough.
- CLI and TUI use the same catalog action. The TUI does not choose or override deletion methods.
- Permanent deletion is never a fallback for a disabled, full, or failed Recycle Bin operation.
- Every candidate is freshly resolved, protected-path filtered, reparse-point checked, and validated immediately before mutation. Existing application-idle gates and structural exclusions remain mandatory.

## Complete rule matrix

`Initially selected` describes the Clean TUI state when the category has at least one safely measured candidate. Users may clear any selection. CLI opt-in behavior remains explicit and additive.

| Canonical category | Catalog class | Permanent-delete eligibility | Initially selected | Planned action | Reason and mandatory guard |
| --- | --- | --- | --- | --- | --- |
| `foal_owned_temp_sandboxes` | Default | Not proven | Yes | `move_to_recycle_bin` | The `foal-` / `Foal-` prefix alone does not prove ownership or inactivity. |
| `user_temp` | Opt-in | Not proven | No | `move_to_recycle_bin` | Seven-day idle age does not prove arbitrary Temp content is regenerable. |
| `crash_dumps` | Opt-in | Not proven | No | `move_to_recycle_bin` | Crash dumps are non-recreatable diagnostic evidence. |
| `windows_error_reporting` | Opt-in | Not proven | No | `move_to_recycle_bin` | WER content is non-recreatable diagnostic evidence. |
| `explorer_thumbnail_cache` | Opt-in | Not proven | No | `move_to_recycle_bin` | The current whole Explorer root is broader than proven thumbnail-cache content; an exact allowlist requires a new decision. |
| `inet_cache` | Opt-in | Not proven | No | `move_to_recycle_bin` | The current whole INetCache root is broader than proven disposable content; an exact allowlist requires a new decision. |
| `d3d_shader_cache` | Opt-in | Proven | Yes | `delete_permanently` | Regenerating shader cache under the exact current-user root. |
| `nvidia_dx_cache` | Opt-in | Proven | Yes | `delete_permanently` | Regenerating NVIDIA DX cache under the exact current-user root. |
| `browser_cache` | Opt-in | Proven | Yes | `delete_permanently` | Only allowlisted Chrome/Edge profile cache roots; browser must be idle before and after complete inspection. |
| `vscode_cache` | Opt-in | Proven | Yes | `delete_permanently` | Only allowlisted regenerating roots under the standard Code directory; Code must be idle before and after inspection. Re-fetch impact remains visible. |
| `cursor_cache` | Opt-in | Proven | Yes | `delete_permanently` | Only allowlisted regenerating roots under the standard Cursor directory; Cursor must be idle before and after inspection. Re-fetch impact remains visible. |
| `npm-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact npm content-addressed cache; existing resolver and shared-runtime caveats remain. |
| `pnpm-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact pnpm content-addressable store root from env/default only; shared-runtime (Node); re-download/hardlink impact disclosed. Never project `node_modules`. |
| `yarn-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact Yarn global cache root (`YARN_CACHE_FOLDER` or `%LOCALAPPDATA%\Yarn\Cache`); shared-runtime (Node); re-download/offline impact disclosed. Never project-local `.yarn/cache`. |
| `go-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact Go build cache; it can be rebuilt, with rebuild cost disclosed. |
| `pip-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact pip download/build cache; packages may need to be downloaded or rebuilt. |
| `cargo-cache` | Opt-in | Proven | Yes | `delete_permanently` | Only existing allowlisted regenerating Cargo cache content; source/build re-fetch cost remains visible. |
| `nuget-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact regenerating NuGet caches; existing resolver and impact notices remain. |
| `nuget-global-packages` | Opt-in | Proven | Yes | `delete_permanently` | Restorable package cache, but offline, private-source, removed, or inaccessible packages may not restore; show a high-impact warning. |
| `corepack-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact Corepack download cache; package-manager artifacts must be downloaded again. |
| `uv-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact uv/uvx regenerating cache only; retain fail-closed idle gating and the upstream direct-cache-modification warning. |
| `bun-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact Bun cache; dependencies may need to be downloaded again. |
| `playwright-browsers` | Opt-in | Proven | Yes | `delete_permanently` | Only complete allowlisted versioned browser installations; exclude MCP profiles, metadata, parents, and hermetic `PLAYWRIGHT_BROWSERS_PATH=0`. |
| `puppeteer-browsers` | Opt-in | Proven | Yes | `delete_permanently` | Only allowlisted product/platform-version installations; exclude root and product parents, and retain shared-runtime policy. |
| `electron-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact Electron download cache root only; never scan legacy or project-local state. |
| `jetbrains-ide-caches` | Opt-in | Proven | Yes | `delete_permanently` | Only exact `caches`, `index`, and Rider `resharper-host` children under supported product-version roots; exclude Local History and require independent product idle gates. |
| `administrator_only_caches` | Permission boundary | Not executable | No | None | Notice only; no automatic elevation and no cleanup authorization. |

## Authorization and confirmation

- Dry-run reports the true planned action without requiring authorization.
- CLI execution requires `--allow-permanent` in addition to `--execute` for permanent actions. Without it, permanent candidates are skipped with `permanent_deletion_not_authorized`; authorized Recycle Bin work continues.
- The TUI starts with the 21 eligible rows described above selected when safely measurable. Its one confirmation view separates Permanent deletion and Recycle Bin summaries, including category count, candidate count, measured bytes, per-category action, irreversible warning, and category-specific impact notices.
- The one TUI confirmation authorizes both disclosed action groups. Fresh execution may change candidate counts and bytes, but it must not introduce an action type that was not disclosed.

## Execution, failure, and cancellation

- Shared Clean completes fresh resolution and all applicable preflight work before mutation. A global safety or configuration failure performs no deletion.
- Recycle Bin capacity is checked for the Recycle Bin portion. Recoverable Recycle Bin actions execute first; irreversible permanent actions execute last.
- Category-, candidate-, or volume-local failures do not block unrelated safe siblings. Completed actions are not rolled back.
- A permanent recursive failure after mutation may have started is `failed` with `permanent_delete_failed`, contributes zero `permanently_deleted_bytes`, warns about possible partial deletion, and never falls back to the Recycle Bin.
- Cooperative cancellation stops recursive traversal and prevents new candidates from starting. An interrupted permanent candidate is `canceled`, contributes zero permanent bytes, warns when partial deletion may have occurred, and is not rolled back.
- Permanent deletion is ordinary filesystem removal only. Foal does not overwrite, shred, wipe free space, or promise forensic non-recoverability.

## Result and History contract

- Successful items record their actual `action`.
- `permanently_deleted_bytes` is measured logical content successfully deleted permanently.
- `recycle_bin_moved_bytes` is measured logical content successfully moved to the Recycle Bin.
- `affected_bytes` is the sum of those fields. It means processed content, not guaranteed physical space released.
- Failed or canceled partially mutated permanent candidates retain their original measured bytes, attempted action, and outcome in History, but add zero to successful permanent bytes.

## Explicit exclusions

- No secure erasure, automatic elevation, process stopping, Recycle Bin fallback, rollback promise, or user-defined executable rules.
- No permanent deletion for the six Recycle Bin categories above until a separate eligibility decision and tests replace the current policy.

## Rule addition checklist

Every new executable cleanup rule must provide all of the following before registration:

- one resolver adapter bound at the private canonical category registration point; do not add caller-side category-ID dispatch or parallel family booleans;
- an explicit planned action and permanent-delete eligibility decision;
- evidence and rationale covering ownership, exact layout, regenerability, and excluded state;
- fresh resolution, Protection, reparse-point, and immediate validation behavior;
- applicable running-application or shared-runtime gates;
- rebuild, re-download, offline, or other impact notices;
- any eager-preview impact notice bound on the same private category registration rather than a downstream category switch;
- the intended TUI initial-selection behavior derived from the same policy;
- contract tests for the action, authorization, failure, cancellation, Result, and History semantics.
