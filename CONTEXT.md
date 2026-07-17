# Foal

Foal is a Windows-native cleanup CLI context focused on preview-first cleanup, explicit execution, and conservative defaults.

## Language

**Mole-inspired report**:
A human-readable Foal report that borrows Mole's grouped preview style while preserving Foal's Windows-native safety model and conservative default cleanup boundary.
_Avoid_: Mole for Windows, feature parity, default rule expansion

**Default candidates**:
Cleanup items that Foal may preview by default and may later execute through the Recycle Bin after explicit confirmation.
_Avoid_: aggressive defaults, hidden cleanup

**Foal-owned temp sandbox candidate**:
A top-level current-user Temp entry selected by the default `foal_owned_temp_sandboxes` rule using the current `foal-` or `Foal-` name prefix. The prefix expresses intended ownership but does not prove origin or inactivity, so the rule retains Recycle Bin policy until a separate ownership marker, strict structure, and non-active lifecycle proof are designed.
_Avoid_: prefix proves ownership, default implies permanent deletion, active sandbox cleanup

**Skipped by default**:
Recognized cleanup opportunities that Foal may report with size, count, status, or review commands, but does not include in default execution.
_Avoid_: default-enabled cache rules

**Clean skipped-by-default discovery**:
A read-only Clean discovery capability that identifies explainable Windows cleanup opportunities outside Foal's frozen default candidate set and reports them as skipped by default, without making them executable or counting them as Potential space.
_Avoid_: default rule expansion, opt-in execution rule, cleanup authorization

**Opportunity category**:
A recognized class of skipped-by-default Windows cleanup opportunity that review-only discovery observes and measures but never executes or counts as Potential space. The implemented catalog is `user_temp`, `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache`, `d3d_shader_cache`, `nvidia_dx_cache`, `amd_gpu_shader_caches`, `intel_gpu_shader_cache`, Chrome/Edge/Firefox `browser_cache`, and idle Application cache categories `vscode_cache` and `cursor_cache`. Each category carries its own observation rule: an idle-age threshold for unbounded user-owned temp entries (see Idle temp opportunity), plain existence for regenerating system caches whose age conveys no safety signal, complete browser profile cache inspection after running-application detection confirms the browser is idle, or exact allowlisted Application cache roots after the owning application is idle before and after inspection. When opted in, `d3d_shader_cache`, `nvidia_dx_cache`, `amd_gpu_shader_caches`, `intel_gpu_shader_cache`, Chrome/Edge/Firefox `browser_cache`, and Application caches `vscode_cache` and `cursor_cache` use Permanent deletion; `explorer_thumbnail_cache` and `inet_cache` retain Recycle Bin policy until their whole-root candidates are replaced by proven exact allowlists. The Recycle Bin is permanently excluded from opportunity discovery; administrator-only roots are permission boundaries; and a cache that an external developer tool's own command would clean is surfaced as a Review suggestion, not an opportunity category, so these sources never double-report the same bytes.
_Avoid_: default candidate, executable category, recycle bin treated as an opportunity, tool-owned cache duplicated as both suggestion and opportunity

**User temp opportunity**:
A non-Foal-owned top-level entry in the current user's Windows temporary directory that Clean may inspect and report through skipped-by-default discovery, but never treats as a default candidate or includes in Potential space. When opted in it retains Recycle Bin policy because idle age does not prove that arbitrary temporary content is regenerable.
_Avoid_: default temp candidate, recursively discovered arbitrary temp path, safe-to-delete claim

**Idle temp opportunity**:
A user temp opportunity whose latest observed modification is at least seven days old, making it eligible for skipped-by-default reporting while still conveying no cleanup authorization.
_Avoid_: safe to delete, expired file, default candidate

**Latest observed modification**:
The newest modification time found across a user temp opportunity and all of its safely inspectable descendants. If that inspection is incomplete, Foal does not classify the entry as an idle temp opportunity.
_Avoid_: top-level directory timestamp, assumed inactivity, partial-scan age

**Observed opportunity bytes**:
The measured bytes represented by skipped-by-default discovery results. This value is reported separately for review and is never included in Potential space.
_Avoid_: reclaimable bytes, cleanable space, Potential space

**Opportunity history summary**:
The aggregate count and observed bytes of non-opted-in skipped-by-default discovery results persisted for a Clean dry-run session without retaining the discovered non-Foal paths. Clean execution history excludes these non-executable, non-opted-in opportunities; an opportunity that has been opted in becomes an opt-in candidate and its executed path is recorded in execution history like any deleted candidate.
_Avoid_: opportunity path history (for non-opted-in), execution item, deletion record

**Opportunity detail section**:
The detailed candidate list section that records full skipped-by-default opportunity review data while remaining a non-authoritative companion artifact that Clean execution never consumes.
_Avoid_: execution manifest, opt-in selection file, deletion input

**Review-only opportunity scan**:
The skipped-by-default discovery scan used by Clean dry-run and the Clean TUI. For non-opted-in categories it is omitted from Clean execution so non-executable observations cannot affect or delay the confirmed cleanup path; an opted-in category is re-scanned at execute time because its results have become executable opt-in candidates, still subject to fresh-scan validation and protection suppression.
_Avoid_: execute-time scan for non-opted-in categories, shared deletion input, implicit cleanup

**Opportunity inspection limit**:
The deterministic per-entry ceiling of 100,000 safely inspected descendants used by review-only opportunity scanning. Entries that exceed the ceiling, cannot be fully inspected, or are interrupted are reported as inspection incomplete and excluded from opportunity totals.
_Avoid_: wall-clock timeout, partial opportunity, estimated complete scan

**Review clues**:
Read-only cleanup hints that Foal surfaces for manual investigation without treating them as cleanup candidates.
_Avoid_: cleanup candidates, executable actions

**Human report labels**:
Plain ASCII presentation-only labels used in non-JSON output to make preview state, skipped state, clean state, clues, and review suggestions easier to scan.
_Avoid_: Unicode symbols, JSON status codes, execution semantics

**TUI status markers**:
Presentation-only symbols used by interactive Foal views to make review state easier to scan. The Clean TUI category-first preview uses `…` for waiting, an animated spinner for scanning, `✓` for a complete scan with candidates, `–` for empty, `⊘` for a safety skip, and `!` for partial, incomplete, or failed diagnostic states; the `>` cursor and `[x]`/`[ ]` selection remain separate, and no status marker implies cleanup authorization.
_Avoid_: JSON status code, byte-derived progress percentage, cleanup authorization, execution result, safe-to-delete signal

**TUI compact item labels**:
Presentation-only item summaries used by interactive Foal views to keep grouped review lists scannable. The Clean TUI may show a short item name, marker, count, size, or status while keeping full paths and contract fields in CLI, JSON, history, or other existing detailed review surfaces; compact labels do not remove evidence from shared models, bypass protected-path suppression, or make browser profile and cache-directory paths part of the primary TUI.
_Avoid_: lossy read model, path suppression bypass, browser profile listing by default, execution manifest

**Report category**:
A presentation grouping that organizes mixed Clean review states by a user-recognizable domain such as `System`, `User essentials`, `Browsers`, or `Developer tools`. A Report category may contain default candidates, skipped-by-default opportunities, running-application skips, review clues, suggestions, or inspection diagnostics; the category never changes an item's execution eligibility, JSON status, or contribution to Potential space.
_Avoid_: cleanup rule group, execution authorization, JSON status

**Permission boundary notice**:
A human-readable notice that explains protected or administrator-only locations were skipped without recommending elevation as the normal path.
_Avoid_: full preview prompt, automatic elevation, run as administrator recommendation

**Protection rules**:
Foal's active cleanup safety boundaries, including default Windows path-safety rules and user-defined deny-only entries loaded from `%APPDATA%\Foal\protection.txt` or `FOAL_PROTECTION_FILE`. Each valid absolute local path protects itself and its subtree using normalized, case-insensitive, path-component-aware matching; protected candidates disappear before reclaimable totals and path-bearing projection, while the Clean TUI eager preview may retain only a path-free category exclusion count and `skipped` or `partial` state, and a Review suggestion without a resolved cache path is never matched by interpreting command text.
_Avoid_: cleanup authorization, allow-only model, protected path disclosure, protected-byte total

**Detailed candidate list**:
A human-readable companion file for clean preview reports that records candidates, skipped items, review clues, and reasons without authorizing later execution.
_Avoid_: execution manifest, deletion input

**Review suggestions**:
Structured, non-authoritative next steps that point at an external tool's own command (or manual investigation) which Foal surfaces but never executes by default and never counts as a Foal cleanup action by default. A developer-tool cache suggestion may become an opt-in candidate whose canonical cleanup rule supplies its Planned deletion action, but Foal never runs the tool's own cleanup command. They remain part of the JSON and human Clean preview contracts; the category-first Clean TUI intentionally presents only canonical cleanup categories and does not duplicate these non-executable suggestions in its primary flow. Being structured does not make them executable by default.
_Avoid_: cleanup actions, delegated execution, running the referenced tool's cleanup command, Foal-owned deletion of the referenced cache without opt-in

**Tool cache query probe**:
A bounded, read-only execution of an allowlisted developer tool's own query subcommand (for example `npm config get cache` or `go env GOCACHE`) that Clean uses only to resolve the displayed cache path for a Review suggestion. Each probe is restricted to a built-in tool allowlist, runs only non-mutating query subcommands, and is bounded by a per-call context timeout. A probe that is not allowlisted, fails, or times out yields no path and never blocks the preview, except Bun: when `bun pm cache` fails, times out, or yields no usable existing path, Review discovery may fall back to Bun's official env/default roots while `bun` is on PATH. This is the one deliberate exception to Clean's otherwise execution-free report preview, and it never runs a tool's cleanup command. The category-first Clean TUI does not invoke these probes because Review suggestions are outside its cleanup-category list.
_Avoid_: running tool cleanup commands, executing arbitrary PATH binaries, unbounded execution, treating probe output as cleanup authorization, probing during Clean execution

**Potential space**:
The bytes represented by Foal default candidates in a clean preview, excluding skipped-by-default items, review clues, external tool suggestions, and permission-boundary skips.
_Avoid_: total hinted space, external savings estimate

**Planned deletion action**:
The shared Clean deletion action assigned to a cleanup item before execution confirmation and shown as part of that confirmation, using stable value `move_to_recycle_bin` or `delete_permanently`. The same selected category has the same planned action through CLI and TUI; execution may complete, skip, or fail that action, but must never silently substitute a different deletion action.
_Avoid_: presentation-specific deletion semantics, execution-time action inference, fallback deletion, hidden permanent deletion

**Permanent deletion**:
An irreversible planned deletion action that uses ordinary filesystem removal to bypass the Recycle Bin and is available only to cleanup items whose permanent-delete eligibility has been explicitly established. It is never inferred from Recycle Bin capacity, configuration, or operation failure and must be visible before confirmation. It does not overwrite data, shred files, wipe free space, or promise forensic non-recoverability.
_Avoid_: secure erase, Recycle Bin overflow fallback, retry as permanent, implicit disk-space recovery

**Permanent-delete eligibility**:
A cleanup-rule property stating that every candidate proven by that rule is a Regenerable cleanup artifact and may use Permanent deletion. Eligibility requires precise rule evidence, complete inspection, no user-authored or diagnostic state, and no unknown layout or reparse point; age, a cache-like name, or location under Temp is insufficient. Shared Clean derives the planned action automatically, and CLI and TUI derive the same action for the same category.
_Avoid_: path-name guess, age implies disposability, TUI-owned classification, per-item deletion-method toggle

**Cleanup rule deletion policy**:
The mandatory `planned_action` declaration on every executable canonical cleanup rule, using stable value `move_to_recycle_bin` or `delete_permanently`. A new rule is incomplete without an explicit action, permanent-delete eligibility rationale, and contract tests; catalog validation rejects a missing or unknown action, and no default is inferred from category, path, or caller. Public category summaries, Dry-run candidates, execution results, History, and the TUI shared read model project this one source of truth without a parallel boolean.
_Avoid_: implicit default action, optional eligibility decision, surface override

**Regenerable cleanup artifact**:
A precisely identified cache, index, downloaded runtime, or other derived artifact that contains no user-authored, diagnostic, configuration, history, or login state and can be recreated or downloaded again. Rebuild, network, or offline cost makes the category higher-impact and requires Opt-in plus an impact notice, but does not by itself remove permanent-delete eligibility.
_Avoid_: arbitrary temporary file, crash evidence, mixed-state root, safe because unused

**Permanent deletion authorization**:
The explicit per-run acknowledgement required before any planned Permanent deletion may execute. The Clean TUI confirmation authorizes the disclosed permanent-delete portion of its exact selection. CLI dry-run always reports true Planned deletion actions without authorization, while CLI execute requires `--allow-permanent`; missing authorization skips permanent-delete candidates with `permanent_deletion_not_authorized`, continues authorized Recycle Bin work, and never changes the planned action.
_Avoid_: ordinary execute implies permanent deletion, persistent consent, fallback to Recycle Bin, dry-run requires authorization

**Permanently deleted bytes**:
The measured logical bytes of candidates successfully removed by Permanent deletion. It is an action outcome, not a guarantee of equal physical free-space gain because hard links, compression, sparse allocation, and filesystem behavior may differ.
_Avoid_: guaranteed freed bytes, Recycle Bin bytes, preview estimate

**Recycle Bin moved bytes**:
The measured logical bytes of candidates successfully moved to the Recycle Bin. These bytes count as affected work but not released disk space while the Recycle Bin retains the items.
_Avoid_: permanently deleted bytes, freed space, Recycle Bin capacity

**Affected bytes**:
The sum of Permanently deleted bytes and Recycle Bin moved bytes for successful actions. It describes total processed content and must not be labeled as released or reclaimed disk space.
_Avoid_: freed bytes, permanent deletion total, preview candidate bytes

**Mixed-action Clean execution**:
A confirmed Clean run containing both Recycle Bin and Permanent deletion actions. Shared Clean completes fresh resolution and every applicable safety preflight before mutation, executes recoverable Recycle Bin actions first and irreversible Permanent deletion last, aborts all mutation on a global safety failure, and otherwise isolates category or volume failures so safe siblings may continue without rollback.
_Avoid_: permanent deletion before preflight, fail-all on one candidate, rollback promise, action fallback

**Partial permanent deletion failure**:
A failed Permanent deletion candidate for which some descendants may already have been irreversibly removed before an error stopped the operation. It is recorded as `failed` with `permanent_delete_failed`, contributes no Permanently deleted bytes because completion cannot be measured reliably, warns that partial deletion may have occurred, and never triggers rollback or Recycle Bin fallback.
_Avoid_: skipped before mutation, successful bytes estimate, rollback, retry through Recycle Bin

**Opt-in candidate**:
A cleanup item that is normally a skipped-by-default opportunity or a developer-tool Review suggestion, but that the user has explicitly opted in to clean for the current run only. Its canonical cleanup rule supplies the Planned deletion action; Permanent deletion additionally requires per-run authorization. An opt-in candidate is never a default candidate: the default candidate set stays frozen, and opt-in never becomes default. Opt-in candidates still pass fresh-scan validation, protection-rule suppression, and running-application gating at execute time, and are never deleted by running an external tool's own cleanup command. Developer-tool examples include npm, pnpm store (`pnpm-cache`), yarn cache (`yarn-cache`), go, pip, cargo, NuGet HTTP and global packages, corepack, uv cache (`uv-cache`), Bun cache (`bun-cache`), structured Playwright browser installations (`playwright-browsers`), Puppeteer browser installations (`puppeteer-browsers`), Electron download cache (`electron-cache`), JetBrains IDE caches (`jetbrains-ide-caches`), and Visual Studio regenerable caches (`visual-studio-caches`); Application cache opportunities `vscode_cache` and `cursor_cache` also become Opt-in candidates when selected for the current run, independently of each other.
_Avoid_: default candidate, default-enabled rule, caller-chosen deletion method, tool-command delegation

**Opt-in candidate resolution**:
The step that turns an opt-in plan into the concrete Opt-in candidate paths for a run, performed fresh for both dry-run preview and execute so preview and execute resolve the same candidate set rather than execute trusting dry-run's resolved paths. CLI dry-run and Execute resolve only opted-in categories; the Clean TUI eager preview scan may use the same shared resolution seam to measure every opt-in category before selection, but returns only path-free category results and does not alter Clean opt-in selection. Execute still scans only opted-in categories and never trusts preview paths. A Browser cache opt-in candidate resolves to individual regenerating cache directories per profile (Chromium: `Cache`, `Code Cache`, `GPUCache`; Firefox: `cache2`), not the browser profile or User Data root, because only those directories are deletable; `browser_cache` has Permanent-delete eligibility after its browser-idle-before-and-after gate succeeds. A structured developer-cache category fresh-resolves roots and fresh-discovers child candidates through the same shared seam; Execute never trusts Dry-run child paths.
_Avoid_: execute trusting dry-run resolved paths, scanning non-opted-in categories at execute, browser User Data root as an opt-in candidate, mode-specific candidate resolution

**Structured developer-cache child discovery**:
The optional private policy bound on a canonical developer-cache catalog entry that, under each resolved and unprotected root, enumerates independent child Opt-in candidates instead of treating the root as a single candidate. Shared Clean opt-in resolution applies Windows path normalization, deduplication, strict-root containment, directory-only acceptance, reparse/symlink rejection, per-child Protection, and Opportunity inspection ceiling measurement. Categories without this policy keep whole-root behavior. Public catalog projections stay path-free and never expose resolvers, allowlists, structural matchers, or executable paths.
_Avoid_: whole-root deletion of mixed-state trees, public path/allowlist catalog fields, Dry-run path manifests for Execute, recursive name guessing outside a fail-closed policy

**Structured downloadable developer-cache artifact**:
A re-downloadable installation or similar disposable artifact under a developer-tool cache root that Foal may reclaim only when a private structured child discovery policy can prove its layout. Unknown layouts, metadata, profile/state directories, incomplete installations, the cache root itself, product parents that must be preserved, regular files, links/junctions/reparse points, and paths outside the resolved root are excluded by construction until an explicit policy and test update authorizes them (ADR 0011). Shipped structured categories include `playwright-browsers`, `puppeteer-browsers`, product-scoped `jetbrains-ide-caches`, and `visual-studio-caches`; Playwright and Puppeteer browser artifacts have Permanent-delete eligibility, while every other category declares its own Cleanup rule deletion policy. For `puppeteer-browsers`: resolve a non-blank `PUPPETEER_CACHE_DIR` or the current user's home `.cache\puppeteer` root, then accept only allowlisted product directories (`chrome`, `chrome-headless-shell`, `firefox`) and Windows platform-version installation directories (`win32-*` / `win64-*`). The Puppeteer root and product parents are never candidates; Foal never reads Puppeteer project config, package.json, CWD, or package-manager state and never runs Puppeteer/npx commands.
_Avoid_: proximity-based deletion under a tool root, fail-open version-looking names, root-as-candidate for mixed-state caches, project-local Puppeteer discovery

**Playwright browsers opt-in**:
A skipped-by-default Developer tools opt-in category (`playwright-browsers`) with Permanent-delete eligibility that reclaims only complete versioned browser-component directories under the global Playwright browsers root. Root resolution uses non-blank `PLAYWRIGHT_BROWSERS_PATH` unless its trimmed value is exactly `0` (hermetic: no global candidate); otherwise the current user's standard Windows Local AppData `ms-playwright` root. Discovery is one direct-child level only: allowlisted `chromium`, `chromium_headless_shell`, `firefox`, `webkit`, `ffmpeg`, and `winldd` names with numeric revisions and `INSTALLATION_COMPLETE` evidence. Each revision is an independent Opt-in candidate; the root is never a candidate. Permanently excluded: every `mcp-*` Profile/state directory, `.links`, `b`, unknown layouts, incomplete installs, regular files, links/junctions/reparse points, CWD/`node_modules`/package-manager stores, and any path outside the resolved root. Shared-runtime policy: Foal does not attribute Node/Python/Chrome/Firefox processes to Playwright, inspect command lines, stop processes, or run Playwright/npx/package-manager commands. Default Dry-run and Execute omit resolution until explicit category, `dev-caches`, or `all`; the Clean TUI eager preview scan may measure the category before selection without selecting or authorizing it for cleanup.
_Avoid_: whole-root ms-playwright deletion, MCP profile cleanup, hermetic project-local browser scan, process stopping, Playwright CLI garbage collection

**Electron cache opt-in**:
A skipped-by-default Developer tools opt-in category (`electron-cache`) with Permanent-delete eligibility that reclaims Electron's downloaded binary cache root. Root resolution uses a non-blank `electron_config_cache` override; otherwise the current user's standard Windows Local AppData `electron\Cache` root. Blank/whitespace override falls back to the default. Only the resolved cache root is a candidate (whole-root); missing or empty roots produce no reclaimable candidate. Permanently excluded from discovery: legacy `~\.electron`, CWD, repositories, `node_modules`, package manifests, registry data, installed Electron applications, project configuration, and unknown sibling directories. Shared-runtime policy: Foal does not attribute Node/Electron processes to this cache, inspect command lines, stop processes, or claim reliable cleanup while a download/install is active. Execute never invokes Electron, npm, npx, package-manager, or third-party cleanup commands. Default Dry-run and Execute omit resolution until explicit category, `dev-caches`, or `all`; the Clean TUI eager preview scan may measure the category before selection without selecting or authorizing it for cleanup. Preview includes a path-free impact note that cached Electron binaries may need to be downloaded again and that offline/custom-cache workflows may be affected. No Electron cleanup command is invented for the non-TUI Review suggestion surface.
_Avoid_: legacy `.electron` scan, project-local Electron discovery, process stopping, Electron/npm/npx command execution, Recycle Bin action

**Product-scoped developer-cache root**:
A resolved developer-cache root that carries one logical application identity for independent idle-before-and-after gating inside a single public cleanup category. Product-version roots under `jetbrains-ide-caches` are product-scoped: each catalogued IntelliJ-platform product maps to one logical application identity and exact Windows launcher process names (IntelliJ IDEA Ultimate/Community → `intellij_idea`; PyCharm Professional/Community → `pycharm`; WebStorm, PhpStorm, RubyMine, CLion, DataGrip, DataSpell, GoLand, RustRover, Aqua, MPS, Writerside → matching product identities; Rider → `rider` with Rider-only `resharper-host`). A running or unknown product discards only that product's roots and measured children; other products in the same category remain independently reclaimable. Public Clean results stay category-based (`jetbrains-ide-caches`); product prefixes, launchers, and root paths are private policy (ADR 0017).
_Avoid_: one global JetBrains gate, substring product matching, public product-path result schema, process command-line attribution

**JetBrains IDE caches opt-in**:
A skipped-by-default Developer tools opt-in category (`jetbrains-ide-caches`) with Permanent-delete eligibility that reclaims only exact `caches` and `index` child directories (plus Rider-only exact `resharper-host`) under the current user's standard `%LOCALAPPDATA%\JetBrains\<Product><Version>` system roots for supported IntelliJ-platform IDEs. Supported standard-layout products for anchored 2020.1+ version layouts: IntelliJ IDEA Ultimate (`IntelliJIdea`) and Community (`IdeaIC`); PyCharm Professional (`PyCharm`) and Community (`PyCharmCE`); WebStorm, PhpStorm, RubyMine, CLion, DataGrip, DataSpell, GoLand, RustRover, Aqua, MPS, Writerside, and Rider (`Rider`, with Rider-only `resharper-host`). Fleet, Air, Gateway/Client, Android Studio, and other non-catalog architectures stay excluded. Each allowlisted child is an independent Opt-in candidate; the JetBrains parent and product-version system roots are never candidates. Permanently excluded: Local History (`LocalHistory`, `fileHistory`), VCS Log, JCEF, plugins, logs, coverage, projects/data-source state, full-line models, tmp/splash/metadata, Toolbox/Installations/Daemon/Shared/dotPeek/ReSharper non-IDE roots, pre-2020 layouts, unknown products/children, regular files, and reparse points. Foal never reads configuration roots, install directories, Toolbox state, registry, CWD, projects, `idea.system.path`, properties files, process command lines, or window titles, and never invokes or stops JetBrains software. Distinctive-process product-scoped idle-before-and-after gating applies per logical product. Default Dry-run and Execute omit resolution until explicit category, `dev-caches`, or `all`; Clean TUI eager preview may measure the unselected category. Preview includes a path-free impact notice that indexes will rebuild and the next startup/project open may be slower.
_Avoid_: whole product-version root deletion, Local History cleanup, custom `idea.system.path` discovery, Invalidate Caches command execution, standalone ReSharper root cleanup, Fleet/Air/Gateway guessing

**Visual Studio caches opt-in**:
A skipped-by-default Developer tools opt-in category (`visual-studio-caches`) with Permanent-delete eligibility that reclaims only exact regenerable roots under the current user's standard `%LOCALAPPDATA%\Microsoft\VisualStudio` parent: shared exact `Roslyn`, and exact `ComponentModelCache` under each anchored 14.0+ instance/version directory (`major.minor` or `major.minor_<hex>`). The VisualStudio parent, instance hives, Settings, Extensions, Packages, MEFCacheBackup, template caches, WebView2Cache, ProgramData, Roaming settings, solutions, and unknown siblings are never candidates. Distinctive-process idle-before-and-after gating uses logical application `visual_studio` (`devenv.exe`) only and is independent of VS Code/Cursor. Foal never reads install dirs, vswhere, registry, solutions, command lines, or window titles, and never invokes or stops Visual Studio. Default Dry-run and Execute omit resolution until explicit category, `dev-caches`, or `all`; Clean TUI may measure and initially select the permanent category when measurable. Preview includes a path-free impact notice that MEF/Roslyn caches rebuild and the next startup or solution load may be slower (ADR 0020).
_Avoid_: whole VisualStudio parent deletion, settings/extensions wipe, ProgramData packages, instance-level unproven siblings, VS Code/Cursor cross-gating

**NuGet global packages opt-in**:
A high-impact Developer tools opt-in category (`nuget-global-packages`) with Permanent-delete eligibility for the exact `NUGET_PACKAGES` or standard current-user `.nuget\packages` root. It never scans project-local package directories or infers paths from NuGet configuration; dotnet and NuGet must be idle. Preview must warn that builds will restore packages again and that offline, private-source, removed, or otherwise inaccessible packages may not be recoverable.
_Avoid_: NuGet HTTP cache, project packages folder, low-impact cleanup, guaranteed re-download

**Opt-in reclaimable bytes**:
The bytes represented by opt-in candidates in a clean preview or execution, reported as a total separate from `Potential space` and `Observed opportunity bytes`. Opt-in reclaimable bytes are never merged into `Potential space`, and `Observed opportunity bytes` excludes any opportunity that has become an opt-in candidate for the run.
_Avoid_: Potential space, observed opportunity bytes, total hinted space

**Project artifact clue**:
A review clue for rebuildable project directories or build outputs. Foal may label matching **top children** of an analyzed path via `foal analyze` (`classification=project_artifact_clue`), and Clean dry-run may show a presentation-only pointer toward `foal analyze` / `foal purge`. Clues never become Clean candidates or contribute to Potential space.
_Avoid_: default project scan, default clean candidate, ordinary Clean opt-in row

**Project artifact purge flow**:
The shipped independent command `foal purge`: recursive discovery of allowlisted rebuildable directories under one or more **user-supplied** roots only (v1 names: `node_modules`, `target`, `dist`, `build`, `.build`, `.next`, `__pycache__`; exact final component). Default is dry-run preview; mutation requires `--execute` plus per-run `--allow-permanent` for permanent deletion (ordinary filesystem removal, high-impact reinstall/rebuild notice). Not a Clean default or catalog opt-in row; no implicit multi-root config, installer purge, elevation, or process stopping (ADR 0019).
_Avoid_: default project scan, clean default candidate, automatic disk-wide purge, ordinary Clean opt-in row, Mole purge_paths parity

**Running application skip**:
A skipped-by-default report state for cleanup opportunities tied to currently running applications or services, especially sync clients, browsers, IDEs, AI tools, containers, and virtualization tools.
_Avoid_: close-and-clean prompt, default candidate

**Running application detection**:
A read-only three-state check used before and after inspecting application-owned caches: `running` means Foal does not inspect or measure the cache and reports a Running application skip, `idle` means Foal may measure it as skipped-by-default review data, and `unknown` means Foal safely skips inspection and reports a recoverable diagnostic. An unknown result never implies that the application is idle. For a supported multi-process browser, any matching browser process makes the whole browser `running`; Foal does not infer per-profile idleness from process command lines. If the application becomes running or unknown during inspection, Foal discards the measured review data and reports the safe skip instead.
_Avoid_: unknown treated as idle, process stopping, close-and-clean prompt

**Browser cache opportunity**:
A skipped-by-default, path-backed review discovery for a supported browser's regenerable cache directories only, measured only after Running application detection confirms the browser is idle. Shipped browsers are Google Chrome, Microsoft Edge, and Mozilla Firefox under the single `browser_cache` category. Chrome and Edge reclaim `Cache`, `Code Cache`, and `GPUCache` under Chromium `User Data` profiles enumerated from `Local State`. Firefox reclaims only Local `cache2` under profiles enumerated from the current-user Roaming `profiles.ini` catalog (`%APPDATA%\Mozilla\Firefox`); missing catalog root is silent absence, and an existing root with a missing, unreadable, or invalid catalog produces an unknown result without guessing profile folders. Other Chromium forks remain deferred (ADR 0019). For Chrome/Edge, Foal uses the current user's browser data root as the existence boundary: a missing `User Data` root is silently absent, while an existing root with a missing, unreadable, or invalid `Local State` profile catalog produces an unknown result. Foal does not use installation discovery or guess profile directories by scanning AppData. JSON represents one Browser cache opportunity per browser with total observed bytes, profile count, and profile-specific cache detail; human output shows the browser summary, while detailed review surfaces may expand the profile paths. A browser summary is reported only when every identified profile can be inspected completely; any incomplete profile inspection discards the whole browser's measured result rather than presenting a partial total. If any profile cache path is protected by Protection rules, Foal suppresses the entire browser opportunity before totals and downstream projection instead of presenting a partial browser summary. A recognized cache directory that does not exist contributes zero bytes and is not an incomplete inspection; a browser whose complete recognized cache total is zero produces no Opportunity. Each existing recognized cache directory uses the standard 100,000-descendant Opportunity inspection limit, and an unsafe, unreadable, canceled, or over-limit inspection invalidates the browser summary. Cookies, history, credentials, extensions, download records, Service Worker data, form data, sessions, places, logins, and whole browser profile directories are permanently excluded for every browser. Playwright/Puppeteer tool-managed browser downloads stay outside Browser cache opportunity.

_Avoid_: browser data, browsing history, cookies, credentials, privacy cleaner, default candidate

**Application cache opportunity**:
A skipped-by-default, path-backed review discovery for regenerating caches owned by a non-browser application, measured only after Running application detection confirms the logical application is idle before and after inspection. Discovery uses one reusable private seam: a registered application policy plus an exact relative-root allowlist under the current user's standard Windows Roaming AppData base. Each existing allowlisted directory is an independent Opportunity or Opt-in candidate with its own path and bytes; Foal never selects roots by substring or recursive user-data enumeration. Complete categories are `vscode_cache` for Visual Studio Code (`visual_studio_code` / `Code.exe`) under `%APPDATA%\Code` and `cursor_cache` for Cursor (`cursor` / `Cursor.exe`) under `%APPDATA%\Cursor`, each with the same allowlisted roots `Cache`, `CachedData`, `CachedExtensionVSIXs`, `Code Cache`, `GPUCache`, `DawnGraphiteCache`, and `DawnWebGPUCache`; both categories have Permanent-delete eligibility, including the re-downloadable `CachedExtensionVSIXs` root whose impact notice remains mandatory. Editor categories, roots, and process identities are independent: running or selecting one editor never authorizes or suppresses the other. Missing or blank AppData or a missing application root is silent absence. Pre-inspection running, unknown, missing required state, or snapshot failure skips all roots for that application without measuring; post-inspection unsafe state discards every measured root and byte total for that application only. Incomplete or canceled inspection contributes no bytes for the interrupted root; non-canceled incomplete siblings may leave completed roots independently represented. Protection suppresses protected roots before totals and downstream projection without authorizing siblings. Settings, profiles, workspace/global storage, backups, installed extensions, Service Worker and web storage, Network/cookies/credentials, logs, Crashpad, and unknown directories are excluded. Cursor evidence is limited to the exact PRD allowlist under the standard root—do not broaden from VS Code ancestry. Portable mode, Insiders/forks, installation discovery, process command-line inspection, and `--user-data-dir` inference are out of scope.
_Avoid_: whole editor user-data cleanup, recursive AppData scanning, browser-named policy for non-browser apps, process stopping, default candidate, shared VS Code/Cursor gate

**Clean preview read model**:
A shared representation of clean preview sections, candidates, skipped-by-default items, review clues, suggestions, protection rules, notices, totals, and detailed-list metadata for JSON, human output, and future TUI consumers.
_Avoid_: CLI string builder as model, TUI-owned cleanup model

**TUI review surface**:
The interactive Foal interface for browsing existing command read models, comparing preview sections, navigating review evidence, and, for Clean only, orchestrating an explicitly confirmed action through the shared Clean execution path without owning cleanup or path-safety decisions.
_Avoid_: TUI-owned cleanup engine, replacement command path, implicit execution, uninstall execution

**Foal main menu**:
The top-level interactive TUI entry that appears when a user explicitly starts Foal's interactive mode, offering command navigation for clean, uninstall, analyze, status, and future read-only views while preserving each command's existing CLI and JSON contract.
_Avoid_: default execution hub, hidden command behavior change, feature-parity clone menu

**Interactive default entry**:
The no-argument `foal` behavior in an interactive terminal, launching the Foal main menu while preserving non-interactive and JSON-oriented command behavior for scripts, pipes, and automation.
_Avoid_: blocking non-TTY scripts, replacing help semantics everywhere, implicit command execution

**Main menu command entries**:
Top-level Foal main menu items that expose the implemented command map, where Clean opens its interactive preview and confirmed-action flow, Uninstall, Status, and History open read-only TUI views, and Analyze and future extensions remain command navigation placeholders until their views are designed.
_Avoid_: pretending every command has a completed TUI, implicit execution, hiding unavailable capability

**Command viewer**:
A shared read-only TUI shell that renders one command's existing report or read model as scrollable text with reload, without per-command interaction logic or any execution affordance.
_Avoid_: per-command TUI platform, editable report, execution surface

**Foal TUI brand frame**:
The visual shell for Foal's interactive surfaces, using Foal-owned ASCII branding, a Windows preview-first tagline, scan-friendly command descriptions, and compact keyboard hints without copying Mole's product wording, Mac positioning, or optimize-first promise.
_Avoid_: Mole brand clone, Mac maintenance wording, decorative UI that obscures safety state

**Clean TUI category-first preview**:
The primary interactive Clean surface for watching eager scan progress, navigating grouped path-free category rows, forming the exact cleanup selection, reviewing its selected total, and entering confirmation. The full dry-run report, filters, expansion controls, and candidate-path copying remain outside this primary surface while Clean preview and execution CLI/JSON contracts stay unchanged; optional path-free TUI execution provenance is an additive History contract instead.
_Avoid_: full-report browser, filter-first Clean UI, duplicated dry-run report, preview path browser, execution manifest

**Clean TUI fatal preview failure**:
A global safety or configuration failure detected before any category can be scanned, presented as a dedicated `Clean unavailable` surface rather than duplicated category failures. It exposes no selection, totals, confirmation, history, or raw path-bearing error and ends only by returning to the menu or quitting.
_Avoid_: per-category failure fanout, partial scan after failed safety configuration, execution affordance, raw path-bearing error

**Clean TUI focused category detail**:
A read-only contextual panel that follows the visible row cursor and explains the focused category's safely completed count and bytes, optional safety note, or partial, empty, skipped, incomplete, and failed reason without requiring another interaction mode. Safety notes are optional path-free evidence supplied by shared Clean within the existing impact-notice vocabulary; the TUI does not infer them from paths or invent one for every category. For partial state the panel may show an excluded sibling count and stable path-free reason, but never excluded paths, excluded bytes, or raw path-bearing errors. Disabled rows remain focusable for this explanation; the panel never exposes a full candidate-path list, changes selection, or authorizes cleanup.
_Avoid_: detail navigation mode, expansion toggle, candidate-path browser, cleanup authorization

**Clean TUI eager preview scan**:
A read-only, sequential measurement of every canonical default and opt-in cleanup category that begins when the Clean TUI opens. Browsing and Clean TUI cleanup selection remain available while it runs; changing selection never restarts scanning, unfinished categories contribute no bytes to the current known selected total, permission-boundary entries remain notices, and confirmed execution resolves selected categories fresh. Non-executable Review suggestions and review clues are not queue entries and do not trigger external tool query probes in this surface.
_Avoid_: blocking selection until scan completion, selection-triggered rescan, estimated unfinished bytes, implicit select all, cleanup authorization, execution manifest, persisted preview paths, permission-boundary cleanup scan

**Clean TUI category scan outcome**:
The terminal, path-free preview result for one eagerly scanned cleanup category: `complete`, `partial`, `empty`, `skipped`, `incomplete`, or `failed`. `complete` carries one or more safely measured candidates even when their bytes are zero, `partial` carries only safely completed candidates and remains selectable with excluded-sibling diagnostics, `empty` means no candidates were found, and the other outcomes contribute no reclaimable bytes; every scannable category must reach an outcome before confirmation becomes available.
_Avoid_: partial result presented as complete, unfinished-byte estimate, scan outcome as cleanup authorization

**Clean TUI cleanup selection**:
The exact per-run set of canonical default and opt-in category identifiers chosen for Clean TUI confirmation. Default categories and every category with Permanent-delete eligibility start selected but may be cleared; Recycle Bin opt-in categories start unselected. The current catalog therefore starts with 21 selected categories and leaves `user_temp`, `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, and `inet_cache` unselected. Confirmation authorizes exactly the visible selection and must not silently add a category or consume preview paths; an `empty`, `skipped`, `incomplete`, or `failed` outcome removes and disables that category for the current scan. CLI additive Opt-in remains unchanged.
_Avoid_: every opt-in starts unselected, hidden default cleanup, selected path list, execution manifest, persistent cleanup profile

**Clean TUI selected preview bytes**:
The safely completed preview bytes summed across the current Clean TUI cleanup selection, including selected default and opt-in categories in `complete` or `partial` state. While selected categories are waiting or scanning, their unknown bytes are excluded and the UI reports those categories separately as pending rather than representing them as zero. Empty, skipped, incomplete, and failed categories contribute zero and cannot remain selected; this total does not replace Potential space or Opt-in reclaimable bytes, and confirmed execution may resolve a different value.
_Avoid_: Potential space, exact execution bytes, unfinished-byte estimate, failed-scan estimate

**Clean opt-in selection**:
The opt-in subset of the Clean TUI cleanup selection. It never contains candidate paths; categories with Permanent-delete eligibility start present while Recycle Bin opt-ins start absent, and the user may narrow or expand the exact set before confirmation. CLI Opt-in remains explicit and additive.
_Avoid_: every opt-in starts absent, execution manifest, selected path list, persistent opt-in profile, implicit path authorization

**Clean execution confirmation**:
The single separate TUI view that reviews the exact Clean TUI cleanup selection before execution; entering it performs no cleanup, and only a second Enter authorizes the shared Clean execution path to resolve and validate fresh candidates. It separately reports Permanent deletion and Recycle Bin category, candidate, and byte totals, labels every selected category with its Planned deletion action, states that permanent deletion is not recoverable, and retains category-specific impact notices. It becomes available only when the selection is non-empty and every scannable category has a Clean TUI category scan outcome. One confirmation authorizes both disclosed action groups; fresh resolution may change candidates and bytes but cannot introduce an undisclosed action type.
_Avoid_: second permanent-delete dialog, executing preview paths, undisclosed action type, one-key accidental cleanup, browsing-as-confirmation

**Clean execution progress**:
Observation-only shared Clean events for the current execution phase and path-free per-selected-category states such as `rechecking`, `ready`, `cleaning`, `empty`, `cleaned`, `partial`, `skipped`, `failed`, and `canceled`, without candidate paths or byte-derived percentages. Progress is not part of the JSON result and never authorizes candidates or drives safety decisions; the final Result and history remain authoritative.
_Avoid_: TUI-inferred progress, byte-derived percentage, candidate path stream, execution manifest, progress as cleanup authorization, rollback promise

**Clean execution category outcome**:
The terminal, path-free projection of one selected category's fresh execution: `empty`, `cleaned`, `partial`, `skipped`, `failed`, or `canceled`. `partial` means at least one item succeeded alongside any excluded, skipped, failed, or canceled item; action-specific and Affected bytes count only successful operations, processed-category progress counts only terminal categories, and item-level Result and history remain authoritative.
_Avoid_: single-state masking of mixed outcomes, preview-derived outcome, failed bytes counted as affected, category outcome replacing item history

**Clean execution cancellation**:
A cooperative stop request made after confirmed Clean execution begins, with no promise to roll back completed Recycle Bin or Permanent deletion operations. Permanent recursive removal checks cancellation during traversal and stops before starting further candidates; an interrupted candidate is `canceled`, contributes no Permanently deleted bytes, and warns when partial irreversible deletion may have occurred. The TUI keeps waiting for the shared final Result, which remains authoritative for completed, skipped, failed, canceled, and partial-operation History outcomes.
_Avoid_: force quit, rollback promise, abandoning final Result, discarding partial-operation history

**Clean execution result view**:
The terminal Clean TUI surface that projects the shared final Result into path-free empty, cleaned, partial, skipped, failed, and canceled category outcomes plus Permanently deleted bytes, Recycle Bin moved bytes, and Affected bytes. It never labels Affected bytes as released disk space. Item-level Result and history record the actual action and remain authoritative, including `context_canceled` skipped outcomes. The view ends the current preview and selection session; returning to the main menu discards that stale state, and entering Clean again starts a new eager preview scan.
_Avoid_: restoring pre-execution preview, progress-derived result, automatic repeat execution, stale selection reuse

**Clean TUI execution provenance**:
Optional, path-free history metadata that identifies `surface=tui`, `selection_mode=exact`, and the canonical selected category identifiers in stable display-and-scan order for confirmed Clean execution. It is a backward-compatible additive History JSON contract, does not fabricate CLI arguments, restore unselected defaults, or replace the normal item-level execution outcomes retained by history.
_Avoid_: synthetic CLI invocation, implicit default authorization, preview paths in command metadata, selection omitted from history

**Aggregate Recycle Bin capacity pre-check**:
A fail-closed Clean safety check that establishes Recycle Bin recoverability for all selected candidates together on each volume before confirmed execution begins.
_Avoid_: per-item-only capacity assurance, assumed capacity, overflow to permanent deletion

**Clean TUI action model**:
A four-stage TUI interaction boundary (eager category-first preview → exact selection with measured totals → separate confirmation → shared execution/result) where browsing and selection remain side-effect free, the first slice exposes no retry or rescan, and Clean alone may transition through explicit confirmation to the shared Clean execution path; uninstaller execution, process stopping, elevation prompts, and leftover deletion remain absent.
_Avoid_: TUI-owned execution engine, implicit cleanup, browsing-as-operation history noise, non-Clean execution, deferred retry documented as current

**Uninstall preview report**:
A human-readable presentation surface rendered directly over the uninstall preview read model, mirroring the Mole-inspired report style while keeping uninstall preview-only and read-only.
_Avoid_: uninstall execution plan, uninstall manifest, leftover deletion list

**Possible leftovers**:
Filesystem paths Foal confidently associates with one discovered, still-installed application (app-owned, high confidence) that would likely remain after an uninstall, surfaced for read-only review only. Lower-confidence findings are split off into shared-state concerns or unknown state rather than reported here.
_Avoid_: deletion candidate, orphan residue of an already-removed application, implying the application is already gone

**Orphaned residue**:
Filesystem paths that look like application data but are not tied to any currently discovered installed application, surfaced as low-confidence read-only review clues unless a future explicit rule proves stronger ownership.
_Avoid_: possible leftovers, deletion candidate, app-owned footprint, safe-to-clean residue

**Not-inspected state**:
A report state asserting that Foal did not examine a discovery category at all, kept distinct from an inspected-but-empty result so the report never implies an examination that did not happen.
_Avoid_: none found, no leftovers, empty result
