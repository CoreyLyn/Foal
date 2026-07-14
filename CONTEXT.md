# Foal

Foal is a Windows-native cleanup CLI context focused on preview-first cleanup, explicit execution, and conservative defaults.

## Language

**Mole-inspired report**:
A human-readable Foal report that borrows Mole's grouped preview style while preserving Foal's Windows-native safety model and conservative default cleanup boundary.
_Avoid_: Mole for Windows, feature parity, default rule expansion

**Default candidates**:
Cleanup items that Foal may preview by default and may later execute through the Recycle Bin after explicit confirmation.
_Avoid_: aggressive defaults, hidden cleanup

**Skipped by default**:
Recognized cleanup opportunities that Foal may report with size, count, status, or review commands, but does not include in default execution.
_Avoid_: default-enabled cache rules

**Clean skipped-by-default discovery**:
A read-only Clean discovery capability that identifies explainable Windows cleanup opportunities outside Foal's frozen default candidate set and reports them as skipped by default, without making them executable or counting them as Potential space.
_Avoid_: default rule expansion, opt-in execution rule, cleanup authorization

**Opportunity category**:
A recognized class of skipped-by-default Windows cleanup opportunity that review-only discovery observes and measures but never executes or counts as Potential space. The implemented catalog is `user_temp`, `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache`, `d3d_shader_cache`, `nvidia_dx_cache`, Chrome/Edge `browser_cache`, and idle Application cache categories `vscode_cache` and `cursor_cache`. Each category carries its own observation rule: an idle-age threshold for unbounded user-owned temp entries (see Idle temp opportunity), plain existence for regenerating system caches whose age conveys no safety signal, complete browser profile cache inspection after running-application detection confirms the browser is idle, or exact allowlisted Application cache roots after the owning application is idle before and after inspection. The Recycle Bin is permanently excluded; administrator-only roots are permission boundaries; and a cache that an external developer tool's own command would clean is surfaced as a Review suggestion, not an opportunity category, so these sources never double-report the same bytes.
_Avoid_: default candidate, executable category, recycle bin treated as an opportunity, tool-owned cache duplicated as both suggestion and opportunity

**User temp opportunity**:
A non-Foal-owned top-level entry in the current user's Windows temporary directory that Clean may inspect and report through skipped-by-default discovery, but never treats as a default candidate or includes in Potential space.
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
Presentation-only symbols used by interactive Foal views to make review state easier to scan. In the Clean TUI, markers emphasize safety semantics: default candidates remain previewed Recycle Bin candidates, skipped-by-default opportunities remain non-executable review data, review clues and suggestions remain manual investigation prompts, running/protected/permission skips remain boundaries, and diagnostics remain recoverable inspection evidence. A positive marker must not imply cleanup authorization or completed deletion.
_Avoid_: JSON status code, cleanup authorization, execution result, safe-to-delete signal

**TUI compact item labels**:
Presentation-only item summaries used by interactive Foal views to keep grouped review lists scannable. The Clean TUI may show a short item name, marker, count, size, or status by default while keeping full paths and contract fields available through expansion or existing detailed review surfaces. Compact labels do not remove evidence from the read model, do not hide protected-path suppression behavior, and do not make browser profile or cache-directory paths part of the default summary.
_Avoid_: lossy read model, path suppression bypass, browser profile listing by default, execution manifest

**Report category**:
A presentation grouping that organizes mixed Clean review states by a user-recognizable domain such as `System`, `User essentials`, `Browsers`, or `Developer tools`. A Report category may contain default candidates, skipped-by-default opportunities, running-application skips, review clues, suggestions, or inspection diagnostics; the category never changes an item's execution eligibility, JSON status, or contribution to Potential space.
_Avoid_: cleanup rule group, execution authorization, JSON status

**Permission boundary notice**:
A human-readable notice that explains protected or administrator-only locations were skipped without recommending elevation as the normal path.
_Avoid_: full preview prompt, automatic elevation, run as administrator recommendation

**Protection rules**:
Foal's active cleanup safety boundaries, including default Windows path-safety rules and user-defined deny-only entries loaded from `%APPDATA%\Foal\protection.txt` or `FOAL_PROTECTION_FILE`. Each valid absolute local path protects itself and its subtree using normalized, case-insensitive, path-component-aware matching. Protected path-backed review discoveries disappear before totals and downstream projection; a Review suggestion without a resolved cache path is not matched by interpreting its command text.
_Avoid_: cleanup authorization, allow-only model

**Detailed candidate list**:
A human-readable companion file for clean preview reports that records candidates, skipped items, review clues, and reasons without authorizing later execution.
_Avoid_: execution manifest, deletion input

**Review suggestions**:
Structured, non-authoritative next steps that point at an external tool's own command (or manual investigation) which Foal surfaces but never executes by default and never counts as a Foal cleanup action by default. A developer-tool cache suggestion may become an opt-in candidate that Foal deletes through its own Recycle Bin action, but Foal never runs the tool's own cleanup command. They are part of the JSON clean contract so automation, human output, and the TUI all see the same suggestions; being structured does not make them executable by default.
_Avoid_: cleanup actions, delegated execution, running the referenced tool's cleanup command, Foal-owned deletion of the referenced cache without opt-in

**Tool cache query probe**:
A bounded, read-only execution of an allowlisted developer tool's own query subcommand (for example `npm config get cache` or `go env GOCACHE`) that Clean uses only to resolve the displayed cache path for a Review suggestion. Each probe is restricted to a built-in tool allowlist, runs only non-mutating query subcommands, and is bounded by a per-call context timeout. A probe that is not allowlisted, fails, or times out yields no path and never blocks the preview, except Bun: when `bun pm cache` fails, times out, or yields no usable existing path, Review discovery may fall back to Bun's official env/default roots while `bun` is on PATH. This is the one deliberate exception to Clean's otherwise execution-free preview, and it never runs a tool's cleanup command.
_Avoid_: running tool cleanup commands, executing arbitrary PATH binaries, unbounded execution, treating probe output as cleanup authorization, probing during Clean execution

**Potential space**:
The bytes represented by Foal default candidates in a clean preview, excluding skipped-by-default items, review clues, external tool suggestions, and permission-boundary skips.
_Avoid_: total hinted space, external savings estimate

**Opt-in candidate**:
A cleanup item that is normally a skipped-by-default opportunity or a developer-tool Review suggestion, but that the user has explicitly opted in to clean through the Recycle Bin for the current run only. An opt-in candidate is never a default candidate: the default candidate set stays frozen, and opt-in never becomes default. Opt-in candidates still pass fresh-scan validation, protection-rule suppression, and running-application gating at execute time, and are never deleted by running an external tool's own cleanup command. Developer-tool examples include npm, go, pip, cargo, NuGet HTTP and global packages, corepack, uv cache (`uv-cache`), Bun cache (`bun-cache`), structured Playwright browser installations (`playwright-browsers`), and Puppeteer browser installations (`puppeteer-browsers`); Application cache opportunities `vscode_cache` and `cursor_cache` also become Opt-in candidates when selected for the current run, independently of each other.
_Avoid_: default candidate, default-enabled rule, permanent deletion, tool-command delegation

**Opt-in candidate resolution**:
The step that turns an opt-in plan into the concrete Opt-in candidate paths for a run, performed fresh for both dry-run preview and execute so preview and execute resolve the same candidate set rather than execute trusting dry-run's resolved paths. Only opted-in categories are scanned; non-opted-in categories stay omitted from execute. A Browser cache opt-in candidate resolves to individual regenerating cache directories (`Cache`, `Code Cache`, `GPUCache`) per profile, not the browser `User Data` root, because only those directories are deletable. A structured developer-cache category fresh-resolves roots and fresh-discovers child candidates through the same shared seam; Execute never trusts Dry-run child paths.
_Avoid_: execute trusting dry-run resolved paths, scanning non-opted-in categories at execute, browser User Data root as an opt-in candidate, mode-specific candidate resolution

**Structured developer-cache child discovery**:
The optional private policy bound on a canonical developer-cache catalog entry that, under each resolved and unprotected root, enumerates independent child Opt-in candidates instead of treating the root as a single candidate. Shared Clean opt-in resolution applies Windows path normalization, deduplication, strict-root containment, directory-only acceptance, reparse/symlink rejection, per-child Protection, and Opportunity inspection ceiling measurement. Categories without this policy keep whole-root behavior. Public catalog projections stay path-free and never expose resolvers, allowlists, structural matchers, or executable paths.
_Avoid_: whole-root deletion of mixed-state trees, public path/allowlist catalog fields, Dry-run path manifests for Execute, recursive name guessing outside a fail-closed policy

**Structured downloadable developer-cache artifact**:
A re-downloadable installation or similar disposable artifact under a developer-tool cache root that Foal may reclaim only when a private structured child discovery policy can prove its layout. Unknown layouts, metadata, profile/state directories, incomplete installations, the cache root itself, product parents that must be preserved, regular files, links/junctions/reparse points, and paths outside the resolved root are excluded by construction until an explicit policy and test update authorizes them (ADR 0011). Shipped structured categories include `playwright-browsers` and `puppeteer-browsers`. For `puppeteer-browsers`: resolve a non-blank `PUPPETEER_CACHE_DIR` or the current user's home `.cache\puppeteer` root, then accept only allowlisted product directories (`chrome`, `chrome-headless-shell`, `firefox`) and Windows platform-version installation directories (`win32-*` / `win64-*`). The Puppeteer root and product parents are never candidates; Foal never reads Puppeteer project config, package.json, CWD, or package-manager state and never runs Puppeteer/npx commands.
_Avoid_: proximity-based deletion under a tool root, fail-open version-looking names, root-as-candidate for mixed-state caches, project-local Puppeteer discovery

**Playwright browsers opt-in**:
A skipped-by-default Developer tools opt-in category (`playwright-browsers`) that reclaims only complete versioned browser-component directories under the global Playwright browsers root. Root resolution uses non-blank `PLAYWRIGHT_BROWSERS_PATH` unless its trimmed value is exactly `0` (hermetic: no global candidate); otherwise the current user's standard Windows Local AppData `ms-playwright` root. Discovery is one direct-child level only: allowlisted `chromium`, `chromium_headless_shell`, `firefox`, `webkit`, `ffmpeg`, and `winldd` names with numeric revisions and `INSTALLATION_COMPLETE` evidence. Each revision is an independent Opt-in candidate; the root is never a candidate. Permanently excluded: every `mcp-*` Profile/state directory, `.links`, `b`, unknown layouts, incomplete installs, regular files, links/junctions/reparse points, CWD/`node_modules`/package-manager stores, and any path outside the resolved root. Shared-runtime policy: Foal does not attribute Node/Python/Chrome/Firefox processes to Playwright, inspect command lines, stop processes, or run Playwright/npx/package-manager commands. Default Dry-run and Execute omit resolution until explicit category, `dev-caches`, `all`, or Clean TUI selection for that run only.
_Avoid_: whole-root ms-playwright deletion, MCP profile cleanup, hermetic project-local browser scan, process stopping, Playwright CLI garbage collection


**Opt-in reclaimable bytes**:
The bytes represented by opt-in candidates in a clean preview or execution, reported as a total separate from `Potential space` and `Observed opportunity bytes`. Opt-in reclaimable bytes are never merged into `Potential space`, and `Observed opportunity bytes` excludes any opportunity that has become an opt-in candidate for the run.
_Avoid_: Potential space, observed opportunity bytes, total hinted space

**Project artifact clue**:
A review clue for rebuildable project directories or build outputs that Foal may surface only through explicit analysis or future opt-in flows.
_Avoid_: default project scan, default clean candidate

**Running application skip**:
A skipped-by-default report state for cleanup opportunities tied to currently running applications or services, especially sync clients, browsers, IDEs, AI tools, containers, and virtualization tools.
_Avoid_: close-and-clean prompt, default candidate

**Running application detection**:
A read-only three-state check used before and after inspecting application-owned caches: `running` means Foal does not inspect or measure the cache and reports a Running application skip, `idle` means Foal may measure it as skipped-by-default review data, and `unknown` means Foal safely skips inspection and reports a recoverable diagnostic. An unknown result never implies that the application is idle. For a supported multi-process browser, any matching browser process makes the whole browser `running`; Foal does not infer per-profile idleness from process command lines. If the application becomes running or unknown during inspection, Foal discards the measured review data and reports the safe skip instead.
_Avoid_: unknown treated as idle, process stopping, close-and-clean prompt

**Browser cache opportunity**:
A skipped-by-default, path-backed review discovery for a supported browser's `Cache`, `Code Cache`, and `GPUCache` directories, measured only after Running application detection confirms the browser is idle. The first supported browsers are Google Chrome and Microsoft Edge. Foal uses the current user's browser data root as the existence boundary: a missing `User Data` root is silently absent, while an existing root with a missing, unreadable, or invalid `Local State` profile catalog produces an unknown result. Foal does not use installation discovery or guess profile directories by scanning `User Data`. JSON represents one Browser cache opportunity per browser with total observed bytes, profile count, and profile-specific cache detail; human output shows the browser summary, while detailed review surfaces may expand the profile paths. A browser summary is reported only when every identified profile can be inspected completely; any incomplete profile inspection discards the whole browser's measured result rather than presenting a partial total. If any profile cache path is protected by Protection rules, Foal suppresses the entire browser opportunity before totals and downstream projection instead of presenting a partial browser summary. A recognized cache directory that does not exist contributes zero bytes and is not an incomplete inspection; a browser whose complete recognized cache total is zero produces no Opportunity. Each existing recognized cache directory uses the standard 100,000-descendant Opportunity inspection limit, and an unsafe, unreadable, canceled, or over-limit inspection invalidates the browser summary. Cookies, history, credentials, extensions, download records, Service Worker data, and whole browser profile directories are excluded.
_Avoid_: browser data, browsing history, cookies, credentials, default candidate

**Application cache opportunity**:
A skipped-by-default, path-backed review discovery for regenerating caches owned by a non-browser application, measured only after Running application detection confirms the logical application is idle before and after inspection. Discovery uses one reusable private seam: a registered application policy plus an exact relative-root allowlist under the current user's standard Windows Roaming AppData base. Each existing allowlisted directory is an independent Opportunity or Opt-in candidate with its own path and bytes; Foal never selects roots by substring or recursive user-data enumeration. Complete categories are `vscode_cache` for Visual Studio Code (`visual_studio_code` / `Code.exe`) under `%APPDATA%\Code` and `cursor_cache` for Cursor (`cursor` / `Cursor.exe`) under `%APPDATA%\Cursor`, each with the same allowlisted roots `Cache`, `CachedData`, `CachedExtensionVSIXs`, `Code Cache`, `GPUCache`, `DawnGraphiteCache`, and `DawnWebGPUCache`. Editor categories, roots, and process identities are independent: running or selecting one editor never authorizes or suppresses the other. Missing or blank AppData or a missing application root is silent absence. Pre-inspection running, unknown, missing required state, or snapshot failure skips all roots for that application without measuring; post-inspection unsafe state discards every measured root and byte total for that application only. Incomplete or canceled inspection contributes no bytes for the interrupted root; non-canceled incomplete siblings may leave completed roots independently represented. Protection suppresses protected roots before totals and downstream projection without authorizing siblings. Settings, profiles, workspace/global storage, backups, installed extensions, Service Worker and web storage, Network/cookies/credentials, logs, Crashpad, and unknown directories are excluded. Cursor evidence is limited to the exact PRD allowlist under the standard root—do not broaden from VS Code ancestry. Portable mode, Insiders/forks, installation discovery, process command-line inspection, and `--user-data-dir` inference are out of scope.
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

**Fo command alias**:
The short interactive convenience alias for the Foal CLI, intended to launch the same command surface as `foal` while keeping `foal` as the canonical command name in product identity, help text, JSON contracts, and documentation.
_Avoid_: legacy compatibility alias, renamed canonical command, separate behavior surface

**Interactive default entry**:
The no-argument `foal` and `fo` behavior in an interactive terminal, launching the Foal main menu while preserving non-interactive and JSON-oriented command behavior for scripts, pipes, and automation.
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

**Clean TUI preview view**:
The Clean TUI state for browsing the Clean preview read model, category summaries, totals, candidates, skipped items, review clues, notices, and suggestions before any execution confirmation.
_Avoid_: multi-command TUI platform, new scanner rules, execution without confirmation, preview path manifest

**Clean TUI report presentation**:
A scan-friendly presentation mode for the Clean TUI that renders the Clean preview read model as grouped report content, with compact visual status markers and summary-first review context while preserving preview semantics and shared command behavior.
_Avoid_: new discovery behavior, JSON status change, preview treated as execution, cleanup completion claim

**Clean opt-in selection**:
The per-run set of opt-in category identifiers chosen in the Clean TUI for preview and later confirmation. It never contains candidate paths, remains empty by default, and a select-all action is an explicit selection rather than a new cleanup default.
_Avoid_: execution manifest, selected path list, persistent opt-in profile, implicit select all

**Clean execution confirmation**:
The deliberate TUI transition that confirms a Clean opt-in selection and authorizes the shared Clean execution path to resolve and validate fresh candidates for that selection.
_Avoid_: executing preview paths, one-key accidental cleanup, browsing-as-confirmation

**Clean execution progress**:
Observation-only stages emitted by shared Clean execution for fresh candidate scanning, aggregate Recycle Bin safety checks, Recycle Bin operations, and completion. Progress is not part of the JSON result and never authorizes candidates or drives safety decisions.
_Avoid_: TUI-inferred progress, execution manifest, progress as cleanup authorization, rollback promise

**Aggregate Recycle Bin capacity pre-check**:
A fail-closed Clean safety check that establishes Recycle Bin recoverability for all selected candidates together on each volume before confirmed execution begins.
_Avoid_: per-item-only capacity assurance, assumed capacity, overflow to permanent deletion

**Clean TUI action model**:
A TUI interaction boundary where browsing and selection remain side-effect free, while Clean alone may transition through explicit confirmation to the shared Clean execution path; uninstaller execution, process stopping, elevation prompts, and leftover deletion remain absent.
_Avoid_: TUI-owned execution engine, implicit cleanup, browsing-as-operation history noise, non-Clean execution

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
