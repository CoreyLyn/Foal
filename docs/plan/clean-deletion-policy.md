# Plan: Clean deletion policy

This is the implemented Clean deletion policy. Shared Clean assigns each executable category an explicit planned action, executes mixed actions with per-run permanent authorization, and records split actual-action totals. ADR 0018 records the two deletion actions and ADR 0029 adds the `invoke_windows_servicing` action; this document is the canonical category matrix.

## Core policy

- Every executable canonical cleanup rule must explicitly declare exactly one planned action: `move_to_recycle_bin`, `delete_permanently`, or `invoke_windows_servicing`. Registration must fail when the action is missing or unknown. `invoke_windows_servicing` (ADR 0029) is action-neutral: it delegates a non-file Windows servicing operation and never produces a deletion candidate or byte estimate. Only `winsxs_component_store` declares it, covering read-only analysis and the composite component-store cleanup.
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
| `explorer_thumbnail_cache` | Opt-in | Not proven | No | `move_to_recycle_bin` | Exact `thumbcache_*.db` and `iconcache_*.db` regular files under current-user `%LOCALAPPDATA%\Microsoft\Windows\Explorer` only; parent Explorer, ETL logs, `RecommendationsFilterList.json`, nested paths, and legacy `%LOCALAPPDATA%\IconCache.db` are never candidates. Missing matches ⇒ empty (no whole-root fallback). Permanent deferred (Explorer locking / always-on). Evidence: `docs/research/explorer-thumbnail-and-inet-cache-allowlists.md`. |
| `inet_cache` | Opt-in | Not proven | No | `move_to_recycle_bin` | Exact real directories `%LOCALAPPDATA%\Microsoft\Windows\INetCache\IE` and `...\INetCache\Low\IE` only; never whole INetCache, `Content.IE5`/`Low\Content.IE5` junctions, `Low` parent, `SuggestedSites.dat`, `Virtualized`, Office `Content.*`, or `thumbnails`. Missing both dirs ⇒ empty (no whole-root fallback). Permanent deferred (active WinINET / mixed-parent history). Evidence: `docs/research/explorer-thumbnail-and-inet-cache-allowlists.md`. |
| `nvidia_installer_cache` | Opt-in (exact-selection-only) | Not proven | No | `move_to_recycle_bin` | Strictly validated completed legacy NVIDIA display-driver download-task directories under the fixed `C:\ProgramData\NVIDIA Corporation\Downloader` root only. A candidate requires a bounded, unique `status.json` record (`status == 2`, `downloadType == 1`), non-empty version/checksum/`fileLocation`, an HTTPS `download.nvidia.com` origin, a single ordinary direct-child payload matching its checksum with a valid NVIDIA Authenticode signature, no reparse points/alternate streams/extra hard links/extra entries/recent writes/active references, and idle NVIDIA process/service state before and after (active or unknown skips the whole category; Clean never stops NVIDIA). Exact-selection-only: excluded from `all`, group tokens, and TUI Select All; never `delete_permanently` and never a Recycle Bin fallback to permanent. Evidence: `docs/research/nvidia-installer-downloader-cache.md`. |
| `d3d_shader_cache` | Opt-in | Proven | Yes | `delete_permanently` | Regenerating shader cache under the exact current-user root. |
| `nvidia_dx_cache` | Opt-in | Proven | Yes | `delete_permanently` | Regenerating NVIDIA DX cache under the exact current-user root. |
| `amd_gpu_shader_caches` | Opt-in | Proven | Yes | `delete_permanently` | Exact allowlisted current-user AMD driver shader cache children under Local `AMD` (`DxCache`, `DxcCache`, `Dx9Cache`, `OglCache`, `VkCache`) plus optional LocalLow `AMD\DxCache`; never parent `AMD` or Adrenalin UI siblings. Evidence: `docs/research/amd-intel-gpu-shader-caches.md`. |
| `intel_gpu_shader_cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact current-user LocalLow `Intel\ShaderCache` only; never Local `Intel` parent, ProgramData, or service-profile copies. Evidence: `docs/research/amd-intel-gpu-shader-caches.md`. |
| `browser_cache` | Opt-in | Proven | Yes | `delete_permanently` | Only allowlisted Chrome/Edge (`Cache`/`Code Cache`/`GPUCache`/`Service Worker\CacheStorage`) and Firefox (`cache2`) profile cache roots; never whole `Service Worker`, `ScriptCache`, or `Database`. Each browser must be idle before and after complete inspection. Evidence: `docs/research/chromium-service-worker-cachestorage.md`. |
| `vscode_cache` | Opt-in | Proven | Yes | `delete_permanently` | Only allowlisted regenerating roots under the standard Code directory; Code must be idle before and after inspection. Re-fetch impact remains visible. |
| `cursor_cache` | Opt-in | Proven | Yes | `delete_permanently` | Only allowlisted regenerating roots under the standard Cursor directory; Cursor must be idle before and after inspection. Re-fetch impact remains visible. |
| `vscode_insiders_cache` | Opt-in | Proven | Yes | `delete_permanently` | Same allowlist under `%APPDATA%\Code - Insiders`; Insiders (`Code - Insiders.exe`) must be idle before and after inspection. Independent of Stable VS Code. |
| `vscodium_cache` | Opt-in | Proven | Yes | `delete_permanently` | Same allowlist under `%APPDATA%\VSCodium`; VSCodium (`VSCodium.exe`) must be idle before and after inspection. Evidence: BleachBit official VS Code-family cleaner. |
| `windsurf_cache` | Opt-in | Proven | Yes | `delete_permanently` | Same allowlist under `%APPDATA%\Windsurf`; Windsurf (`Windsurf.exe`) must be idle before and after inspection. Evidence: BleachBit official VS Code-family cleaner. |
| `trae_cache` | Opt-in | Proven | Yes | `delete_permanently` | Same allowlist under `%APPDATA%\Trae`; Trae (`Trae.exe`), a VS Code fork, must be idle before and after inspection. Independent of the other editors. |
| `npm-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact npm content-addressed cache; existing resolver and shared-runtime caveats remain. |
| `pnpm-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact pnpm content-addressable store root from env/default only; shared-runtime (Node); re-download/hardlink impact disclosed. Never project `node_modules`. |
| `yarn-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact Yarn global cache root (`YARN_CACHE_FOLDER` or `%LOCALAPPDATA%\Yarn\Cache`); shared-runtime (Node); re-download/offline impact disclosed. Never project-local `.yarn/cache`. |
| `go-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact Go build cache; it can be rebuilt, with rebuild cost disclosed. |
| `go-modcache` | Opt-in | Proven | Yes | `delete_permanently` | Exact Go module download cache (`GOMODCACHE`, else first `GOPATH` entry `pkg\mod`, else `%USERPROFILE%\go\pkg\mod`); re-download impact and private/offline restore risk disclosed. Separate from `go-cache`. Distinctive-process idle gate shares Go identity. Never other GOPATH/pkg siblings, GOROOT, tool binaries, or project `vendor/`. |
| `pip-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact pip download/build cache; packages may need to be downloaded or rebuilt. |
| `cargo-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact allowlisted regenerable roots under non-blank `CARGO_HOME` else `~\.cargo`: `registry\cache` (downloaded `.crate` archives) and `registry\src` (unpacked crate sources). Missing allowlisted children = silent absence (no parent/`.cargo` fallback). Never `bin\`, `config.toml`, `credentials.toml`, `.crates.toml`/`.crates2.json`, `registry\index`, whole `.cargo`, project `target\`, or `git\*` (deferred). Distinctive-process idle gate (`cargo.exe`). Re-download/re-extract impact disclosed. Evidence: Cargo Book Cargo Home (`registry/cache`, `registry/src`). |
| `nuget-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact regenerating NuGet caches; existing resolver and impact notices remain. |
| `nuget-global-packages` | Opt-in | Proven | Yes | `delete_permanently` | Restorable package cache, but offline, private-source, removed, or inaccessible packages may not restore; show a high-impact warning. |
| `corepack-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact Corepack download cache; package-manager artifacts must be downloaded again. |
| `uv-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact uv/uvx regenerating cache only; retain fail-closed idle gating and the upstream direct-cache-modification warning. |
| `bun-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact Bun cache; dependencies may need to be downloaded again. |
| `playwright-browsers` | Opt-in | Proven | Yes | `delete_permanently` | Only complete allowlisted versioned browser installations; exclude MCP profiles, metadata, parents, and hermetic `PLAYWRIGHT_BROWSERS_PATH=0`. |
| `puppeteer-browsers` | Opt-in | Proven | Yes | `delete_permanently` | Only allowlisted product/platform-version installations; exclude root and product parents, and retain shared-runtime policy. |
| `electron-cache` | Opt-in | Proven | Yes | `delete_permanently` | Exact Electron download cache root only; never scan legacy or project-local state. |
| `jetbrains-ide-caches` | Opt-in | Proven | Yes | `delete_permanently` | Only exact `caches`, `index`, and Rider `resharper-host` children under supported product-version roots; exclude Local History and require independent product idle gates. |
| `visual-studio-caches` | Opt-in | Proven | Yes | `delete_permanently` | Only exact `ComponentModelCache` under current-user 14.0+ instance hives and shared `Roslyn` under `%LOCALAPPDATA%\Microsoft\VisualStudio`; devenv idle-before-and-after; exclude Settings/Extensions/ProgramData/solutions. |
| `grok-build-update-residue` | Opt-in | Proven | Yes | `delete_permanently` | Exact ordinary files under `$GROK_HOME\bin` only: lowercase `grok.exe.old`, `agent.exe.old`, and anchored `*.exe.old.<pid>-<seq>.old` with non-empty decimal fields. One logical Grok Build identity (`grok.exe`/`agent.exe`) must be idle before and after discovery. Direct `downloads\grok-*` files are update witnesses only (never candidates); recent/future/unknown timestamps or unreadable downloads fail closed. Not a `dev-caches` member; selected via exact name, selection group `cli-agents`, or `all`. Evidence: ADR 0021/0022, `docs/plan/grok-build-update-residue.md`. |
| `obsidian_cache` | Opt-in | Proven | Yes | `delete_permanently` | Plain-Electron 6-root allowlist (`Cache`, `Code Cache`, `GPUCache`, `DawnCache`, `DawnGraphiteCache`, `DawnWebGPUCache`) under `%APPDATA%\obsidian`; Obsidian (`Obsidian.exe`), a non-editor Electron app under the Applications report category, must be idle before and after inspection. Excludes `CachedData`, `CachedExtensionVSIXs`, and state/config/bundle (`obsidian.json`, `*.asar`, Local Storage, IndexedDB, Service Worker, Preferences). Independent idle gate; selected via exact name, selection group `app-caches`, or `all` - never `dev-caches`. |
| `winsxs_component_store` | Opt-in (exact-selection-only) | Not applicable | No | `invoke_windows_servicing` | Windows component store (WinSxS). Never a file candidate or byte estimate: Foal delegates read-only analysis to the Windows servicing stack through a capability-limited elevated helper. Exact-selection-only (excluded from `all`, every group token, and TUI Select All). An exact CLI dry-run opt-in requests `DISM /Online /Cleanup-Image /AnalyzeComponentStore /English /NoRestart` under UAC and deletes nothing; default Dry-run, group tokens, and TUI entry never analyze it or trigger UAC. Component-store cleanup (mutation) requires `--execute`, exact selection, and the dedicated per-run `--allow-servicing` authorization (independent of `--allow-permanent`); when authorized, the elevated helper runs a fresh analysis and starts `DISM /Online /Cleanup-Image /StartComponentCleanup /English /NoRestart` in one session, only when reclaimable packages are positive and cleanup is recommended, and always as the final action group. Missing authorization skips with `windows_servicing_not_authorized` and no UAC. Evidence: ADR 0029, `docs/research/nvidia-installer-downloader-cache.md` (servicing evidence in ADR 0029). |
| `administrator_only_caches` | Permission boundary | Not executable | No | None | Notice only; no automatic elevation and no cleanup authorization. |

## Authorization and confirmation

- Dry-run reports the true planned action without requiring authorization.
- CLI execution requires `--allow-permanent` in addition to `--execute` for permanent actions. Without it, permanent candidates are skipped with `permanent_deletion_not_authorized`; authorized Recycle Bin work continues.
- CLI Windows servicing mutation requires `--execute`, exact selection of `winsxs_component_store`, and the dedicated per-run `--allow-servicing` (independent of `--allow-permanent`, never implied by it). Missing `--allow-servicing` skips the category with `windows_servicing_not_authorized` and never opens UAC; an exact dry-run opt-in may still request UAC for read-only analysis only. `nvidia_installer_cache` needs no permanent or servicing authorization: `--execute` plus its exact opt-in moves the verified package to the Recycle Bin.
- The TUI starts with the 31 eligible rows described above selected when safely measurable (1 default + 30 permanent). Its one confirmation view separates Permanent deletion and Recycle Bin summaries, including category count, candidate count, measured bytes, per-category action, irreversible warning, and category-specific impact notices.
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
- No permanent deletion for the seven Recycle Bin categories above (`foal_owned_temp_sandboxes`, `user_temp`, `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache`, and `nvidia_installer_cache`) until a separate eligibility decision and tests replace the current policy. `nvidia_installer_cache` is `Not proven`, so `--allow-permanent` never promotes it and a Recycle Bin capacity failure never falls back to permanent deletion.
- `winsxs_component_store` never deletes files beneath `WinSxS`, never estimates reclaimable bytes, and never elevates the whole run: it delegates to the Windows servicing stack through a capability-limited elevated helper and requires the dedicated per-run `--allow-servicing` authorization (independent of `--allow-permanent`).

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
