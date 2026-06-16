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
A recognized class of skipped-by-default Windows cleanup opportunity that review-only discovery observes and measures but never executes or counts as Potential space. The implemented catalog is `user_temp`, `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache`, `d3d_shader_cache`, `nvidia_dx_cache`, and Chrome `browser_cache`. Each category carries its own observation rule: an idle-age threshold for unbounded user-owned temp entries (see Idle temp opportunity), plain existence for regenerating system caches whose age conveys no safety signal, or complete browser profile cache inspection after running-application detection confirms the browser is idle. Edge browser caches are excluded until their measurement slice is implemented; the Recycle Bin is permanently excluded; administrator-only roots are permission boundaries; and a cache that an external developer tool's own command would clean is surfaced as a Review suggestion, not an opportunity category, so these sources never double-report the same bytes.
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
The aggregate count and observed bytes of skipped-by-default discovery results persisted for a Clean dry-run session without retaining the discovered non-Foal paths. Clean execution history excludes these non-executable opportunities.
_Avoid_: opportunity path history, execution item, deletion record

**Opportunity detail section**:
The detailed candidate list section that records full skipped-by-default opportunity review data while remaining a non-authoritative companion artifact that Clean execution never consumes.
_Avoid_: execution manifest, opt-in selection file, deletion input

**Review-only opportunity scan**:
The skipped-by-default discovery scan used by Clean dry-run and the Clean TUI, but omitted from Clean execution so non-executable observations cannot affect or delay the confirmed cleanup path.
_Avoid_: execute-time opportunity scan, shared deletion input, implicit cleanup

**Opportunity inspection limit**:
The deterministic per-entry ceiling of 100,000 safely inspected descendants used by review-only opportunity scanning. Entries that exceed the ceiling, cannot be fully inspected, or are interrupted are reported as inspection incomplete and excluded from opportunity totals.
_Avoid_: wall-clock timeout, partial opportunity, estimated complete scan

**Review clues**:
Read-only cleanup hints that Foal surfaces for manual investigation without treating them as cleanup candidates.
_Avoid_: cleanup candidates, executable actions

**Human report labels**:
Plain ASCII presentation-only labels used in non-JSON output to make preview state, skipped state, clean state, clues, and review suggestions easier to scan.
_Avoid_: Unicode symbols, JSON status codes, execution semantics

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
Structured, non-authoritative next steps that point at an external tool's own command (or manual investigation) which Foal surfaces but never executes or counts as a Foal cleanup action. They are part of the JSON clean contract so automation, human output, and the TUI all see the same suggestions; being structured does not make them executable.
_Avoid_: cleanup actions, delegated execution, Foal-owned deletion of the referenced cache

**Tool cache query probe**:
A bounded, read-only execution of an allowlisted developer tool's own query subcommand (for example `npm config get cache` or `go env GOCACHE`) that Clean uses only to resolve the displayed cache path for a Review suggestion. Each probe is restricted to a built-in tool allowlist, runs only non-mutating query subcommands, and is bounded by a per-call context timeout. A probe that is not allowlisted, fails, or times out yields no path and never blocks the preview. This is the one deliberate exception to Clean's otherwise execution-free preview, and it never runs a tool's cleanup command.
_Avoid_: running tool cleanup commands, executing arbitrary PATH binaries, unbounded execution, treating probe output as cleanup authorization, probing during Clean execution

**Potential space**:
The bytes represented by Foal default candidates in a clean preview, excluding skipped-by-default items, review clues, external tool suggestions, and permission-boundary skips.
_Avoid_: total hinted space, external savings estimate

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

**Clean preview read model**:
A shared representation of clean preview sections, candidates, skipped-by-default items, review clues, suggestions, protection rules, notices, totals, and detailed-list metadata for JSON, human output, and future TUI consumers.
_Avoid_: CLI string builder as model, TUI-owned cleanup model

**TUI review surface**:
The interactive Foal interface for browsing existing command read models, comparing preview sections, and navigating review evidence without owning cleanup, uninstall, path-safety, or execution decisions.
_Avoid_: TUI-owned cleanup engine, TUI execution model, replacement command path, future-tense framing

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
Top-level Foal main menu items that expose the implemented command map, where Clean, Uninstall, Status, and History open read-only TUI views over their existing read models, while Analyze and future extensions remain command navigation placeholders until their views are designed.
_Avoid_: pretending every command has a completed TUI, launching destructive flows, hiding unavailable capability

**Command viewer**:
A shared read-only TUI shell that renders one command's existing report or read model as scrollable text with reload, without per-command interaction logic or any execution affordance.
_Avoid_: per-command TUI platform, editable report, execution surface

**Foal TUI brand frame**:
The visual shell for Foal's interactive surfaces, using Foal-owned ASCII branding, a Windows preview-first tagline, scan-friendly command descriptions, and compact keyboard hints without copying Mole's product wording, Mac positioning, or optimize-first promise.
_Avoid_: Mole brand clone, Mac maintenance wording, decorative UI that obscures safety state

**Clean TUI preview view**:
The first TUI review surface slice, focused on browsing the existing clean preview read model for `foal clean --dry-run` sections, totals, candidates, skipped items, review clues, notices, and suggestions.
_Avoid_: multi-command TUI platform, new scanner rules, TUI cleanup execution

**Read-only TUI action model**:
A TUI interaction boundary where navigation, filtering, expansion, scrolling, reload, and clipboard-copy review affordances are allowed, while cleanup execution, uninstaller execution, process stopping, elevation prompts, and leftover deletion are absent, and where browsing itself records no history sessions and writes no companion files.
_Avoid_: execute button, confirmation flow, destructive TUI action, browsing-as-operation history noise

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
