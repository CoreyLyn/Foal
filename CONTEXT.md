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
A recognized class of skipped-by-default Windows cleanup opportunity that review-only discovery observes and measures but never executes or counts as Potential space. The implemented catalog is `user_temp`, `crash_dumps`, `windows_error_reporting`, `explorer_thumbnail_cache`, `inet_cache`, `d3d_shader_cache`, `nvidia_dx_cache`, `nvidia_gl_cache`, `amd_gpu_shader_caches`, `intel_gpu_shader_cache`, Chrome/Edge/Firefox `browser_cache`, and idle Application cache categories `vscode_cache`, `cursor_cache`, `vscode_insiders_cache`, `vscodium_cache`, `windsurf_cache`, `trae_cache`, `obsidian_cache`, and `vrchat_cache`. Each category carries its own observation rule: an idle-age threshold for unbounded user-owned temp entries (see Idle temp opportunity), plain existence for regenerating system caches whose age conveys no safety signal, complete browser profile cache inspection after running-application detection confirms the browser is idle, or exact allowlisted Application cache roots after the owning application is idle before and after inspection. When opted in, `d3d_shader_cache`, `nvidia_dx_cache`, `nvidia_gl_cache`, `amd_gpu_shader_caches`, `intel_gpu_shader_cache`, Chrome/Edge/Firefox `browser_cache`, and Application caches `vscode_cache`, `cursor_cache`, `vscode_insiders_cache`, `vscodium_cache`, `windsurf_cache`, `trae_cache`, `obsidian_cache`, and `vrchat_cache` use Permanent deletion; `explorer_thumbnail_cache` and `inet_cache` retain Recycle Bin policy until their whole-root candidates are replaced by proven exact allowlists. The Recycle Bin is permanently excluded from opportunity discovery; administrator-only roots are permission boundaries; and a cache that an external developer tool's own command would clean is surfaced as a Review suggestion, not an opportunity category, so these sources never double-report the same bytes.
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

**Clean TUI dual-channel presentation**:
A presentation-only encoding on the category-first Clean TUI that separates reclaimable-byte magnitude from deletion-action risk so the same red danger cue is never used for both. Magnitude uses tiered emphasis on trusted byte tokens only; risk uses confirmation grouping, permanent-deletion warnings, and a selection-footer permanent notice without per-row action prefixes on the preview list.
_Avoid_: red means large, whole-row magnitude color, magnitude implies danger, color as sole risk cue, execution authorization by color, per-row perm/bin chrome

**Clean TUI magnitude emphasis**:
Presentation-only styling of a trusted measured or affected byte token by absolute size using 1024-based thresholds: neutral below 100 MiB, attention (amber/yellow) from 100 MiB inclusive to 1 GiB exclusive, and strong (orange, not pure red) at 1 GiB and above. Zero-byte, empty, skipped, and unfinished/pending values receive no magnitude color; bold may remain when color is unavailable (`NO_COLOR` or weak terminals).
_Avoid_: red for gigabytes, relative free-disk thresholds, unfinished-byte estimates, whole-row tint from size, freed-disk claim

**Clean TUI permanent-selection notice**:
A footer sentence shown only while the current Clean TUI cleanup selection includes at least one permanent-delete category, stating that the selection includes permanent deletion, distinct from magnitude totals and from confirmation's irreversible warning. Preview category rows do not prefix planned-action markers.
_Avoid_: always-visible permanent chrome, red magnitude total, silent permanent selection, per-row perm/bin labels

**TUI restricted token styling**:
A narrow presentation exception that may apply ANSI/style sequences to individual path-free tokens (byte sizes, confirmation warnings) after a plain-text frame is built, while keeping the plain frame the source of truth for tests and copy. It does not authorize whole-row rainbow styling or embed escapes inside asserted contract text.
_Avoid_: arbitrary mid-line decoration, styled strings as test oracles, style-owned domain state

**Report category**:
A presentation grouping that organizes mixed Clean review states by a user-recognizable domain such as `System`, `User essentials`, `Browsers`, `Developer tools`, or `Applications`. A Report category may contain default candidates, skipped-by-default opportunities, running-application skips, review clues, suggestions, or inspection diagnostics; the category never changes an item's execution eligibility, JSON status, or contribution to Potential space.
_Avoid_: cleanup rule group, execution authorization, JSON status

**Permission boundary notice**:
A human-readable notice that explains protected or administrator-only locations were skipped without recommending elevation as the normal path.
_Avoid_: full preview prompt, automatic elevation, run as administrator recommendation

**Windows system cleanup handoff**:
A path-free, byte-free, non-executable refinement of the Administrator permission boundary that directs the user to Windows Settings > System > Storage > Temporary files or Cleanup recommendations for Windows-managed space not covered by an explicit Windows servicing action. It is labeled Windows-managed and Not measured by Foal, excluded from Potential space, and never becomes executable merely because Windows may be able to clean the referenced storage.
_Avoid_: JSON recommendation, persisted recommendation, estimated system-cleanable bytes, implicit servicing authorization

**Windows servicing action**:
An explicitly confirmed Clean action that delegates component selection and mutation to the Windows servicing stack instead of treating system files as Foal deletion candidates. Its first component-store capability uses only read-only `AnalyzeComponentStore` and mutating `StartComponentCleanup`; execution requires a fresh successful English analysis that reports both a positive reclaimable-package count and a cleanup recommendation. Preview and analysis surfaces report package count and recommendation only—never guessed reclaimable bytes or DISM store-composition sizes as cleanable size; the Clean selection row may say size unknown without inventing a byte figure. After a completed mutation, Result may attach a Servicing free-space observation only. Path-based Protection is not applicable because Windows, not Foal, owns an undisclosed servicing mutation set; exact selection and action authorization are the servicing controls. Clean TUI presents it initially unselected as `analysis_required`; a distinct user analysis action is required before it can become selectable, and execution never trusts that preview analysis. Foal prevents its own concurrent servicing helpers with a global mutex but treats CBS/DISM—not process presence—as the authority for Windows servicing concurrency and never stops servicing/update processes or services. The helper resolves the fixed system `dism.exe` through Windows APIs, validates its ordinary non-reparse identity before and after opening, and launches it directly with fixed argument arrays—never through PATH, environment overrides, CWD, `cmd.exe`, PowerShell, or a shell command string. In a mixed run, servicing is always the final action group; it may be canceled before mutation starts, but after servicing starts cancellation is recorded while Windows finishes and the actual exit outcome remains authoritative. Windows owns the servicing transaction, while Foal must never translate servicing output into file paths for Recycle Bin or Permanent deletion.
_Avoid_: WinSxS file deletion, DISM report as candidate manifest, ResetBase, SPSuperseded, Remove-Package, user-supplied DISM arguments, Recycle Bin component cleanup, Foal-owned package removal

**Clean servicing elevation exception**:
The product rule that an explicitly requested Windows servicing analysis or confirmed Windows servicing action may request administrator consent for an isolated, capability-limited elevated helper. Default Dry-run and entry into the Clean TUI never trigger UAC; CLI analysis requires exact category opt-in, and TUI analysis requires a distinct user action before the initially unselected category can become selectable. Analysis authorization is not retained for execution. The coordinating Foal process and all ordinary Clean path deletion remain non-elevated; denial or helper failure produces path-free servicing state and never authorizes Foal to bypass path safety or delete Windows system files itself.
_Avoid_: elevated Clean process, elevated mixed-action batch, arbitrary elevated command, silent UAC, product-wide elevation, elevation authorizes system-path deletion

**Servicing helper protocol**:
A one-request, versioned Windows named-pipe exchange between the non-elevated coordinator and the isolated elevated Foal helper. It authorizes only built-in capability `analyze_component_store` or composite `execute_component_store_cleanup`, uses a one-time nonce and restricted ACL with peer process/executable validation, contains no executable, shell arguments, or filesystem path, and returns only a structured Servicing operation record. The composite capability performs fresh analysis and guarded cleanup in one elevated transaction; no standalone cleanup capability exists. UAC/IPC handshake is bounded, but DISM operations have no forced wall-clock timeout; analysis may be canceled before mutation, while cleanup is never force-terminated after it starts.
_Avoid_: temporary request file, arbitrary command IPC, reusable helper session, raw DISM output, unauthenticated internal mode

**Windows servicing authorization**:
The explicit per-run CLI acknowledgement `--allow-servicing` required in addition to `--execute`, exact canonical-category selection, and UAC consent before a Windows servicing action may mutate the system. Servicing categories are excluded from `all`, existing group tokens, and TUI Select All; they require exact CLI selection or individual TUI analysis and selection. Authorization is independent of Permanent deletion authorization; absence skips servicing without prompting for UAC, while the Clean TUI confirmation supplies equivalent authorization for an explicitly analyzed and selected servicing category.
_Avoid_: execute implies servicing, permanent authorization implies servicing, UAC alone authorizes servicing, persistent consent

**Servicing operation record**:
A path-free Result or History record for one Windows servicing category, carrying its Planned action, fixed capability, parsed analysis evidence, stable outcome (`ready`, `no_work`, `completed`, `skipped`, `failed`, or pre-mutation `canceled`), cancellation-request state, optional DISM exit code, restart-required state, optional Servicing free-space observation when a mutation completed, and stable reason when applicable. It is distinct from file candidates and deletion items, contributes only servicing counts plus that optional observation, and never claims Foal-deleted or reclaimable file bytes; cancellation requested after mutation starts does not replace the actual completed or failed outcome.
_Avoid_: empty-path deletion item, zero-byte candidate, WinSxS path list, servicing bytes in affected bytes, persisted DISM output

**Servicing free-space observation**:
The optional path-free non-negative free-space increase on the volume that hosts Windows, measured only around a **completed exit-0** `StartComponentCleanup` (after free minus before free; excluding the preceding fresh analysis and all earlier Clean action groups). Present only when the mutation completed with exit code 0—where reclaim happens in-process—and the measured delta is ≥ 0; a negative delta or non-completed outcomes omit the observation rather than inventing reclaimed bytes. A restart-required (3010) success omits the numeric observation entirely, because the actual reclaim occurs after reboot and an immediate delta would understate it; that outcome instead carries a path-free restart-required state (space reclaimed after restart) with no byte figure. When present it is stored on the Servicing operation record in Result and History and shown on final reports from that same record. A measured delta of exactly 0 is still recorded in Result/History (preserving the distinction between measured-zero and not-measured), but presentation treats zero as no observation: no observation line is rendered and no Mixed cleanup impact is shown (the report collapses to Affected-only). It is an approximate external observation of disk change during that mutation, not a path inventory or Foal-owned deletion total, and never merges into Potential space, Opt-in reclaimable bytes, Selected category bytes, Affected bytes, Recycle Bin moved bytes, or Permanently deleted bytes. On any surface it is a de-emphasized approximate token—always carrying an `≈` prefix, excluded from Danger-aware magnitude encoding's tiered emphasis on trusted byte tokens, explicitly marked observed/approximate, and positioned below the precise Affected bytes—so a noisy observation is never read as an exact deletion measurement.
_Avoid_: reclaimable bytes, affected bytes, cleaned size guarantee, WinSxS path measure, candidate bytes, servicing in cleanup total, whole-run free-space delta, analysis-included delta, negative cleaned size, restart-required immediate delta, TUI-only ephemeral size

**Mixed cleanup impact (presentation)**:
A human-facing, approximate sum shown on final Clean execute reports when a Servicing free-space observation is present and greater than zero: path-deletion Affected bytes plus that observation. It is presentation-only guidance labeled approximate; it does not replace Affected bytes, does not rewrite deletion totals, and is omitted or collapses to Affected-only when no observation is present or the observation is zero.
_Avoid_: Affected bytes, reclaimable bytes, exact total cleaned, single JSON source of truth for impact

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

**Planned action**:
The shared Clean action assigned to a cleanup item or servicing operation before execution confirmation and shown as part of that confirmation, using stable value `move_to_recycle_bin`, `delete_permanently`, or `invoke_windows_servicing`. The same selected category has the same planned action through CLI and TUI; execution may complete, skip, fail, or be canceled, but must never silently substitute a different action.
_Avoid_: Planned deletion action, presentation-specific semantics, execution-time action inference, fallback action, hidden servicing invocation

**NVIDIA completed download task**:
A legacy GeForce Experience display-driver payload under the fixed `C:\ProgramData\NVIDIA Corporation\Downloader` root whose immediate 32-hex task directory is uniquely bound to a bounded legacy `status.json` record and passes the complete version-specific completion, checksum, NVIDIA signature, single-file containment, idle, stability, freshness, Protection, reparse-point, and immediate-revalidation policy. It is not a general NVIDIA cache classification: the Downloader root, application-update tasks, `latest`, `PostProcessing`, metadata, `Installer2`, and the newer `NVIDIA app\UpdateFramework` lifecycle are permanently outside this scope. Because NVIDIA publishes neither a stable cross-version removal contract nor an exhaustive writer set, present evidence permits only an exact-selection, Not-proven `move_to_recycle_bin` action. The category is initially unselected and excluded from `all`, every group token, and TUI Select All. Any relevant NVIDIA application, container, helper, overlay, installer, or service being active—or process/service state being unknown—skips the entire category; frequent false skips are an accepted safety cost, and Clean never stops those processes or services. Failure or uncertainty at any other gate likewise produces no candidate.
_Avoid_: all-token expansion, every 32-hex directory, `status == 2` alone, narrow writer guess, stopping NVIDIA processes, whole Downloader cleanup, NVIDIA App UpdateFramework cleanup, Installer2 cleanup, permanent deletion, Proven

**NVIDIA GL cache category**:
A GPU shader-cache category (`nvidia_gl_cache`) covering only the current user's `%LOCALAPPDATA%\NVIDIA\GLCache` root, kept separate from `nvidia_dx_cache` so each category name matches exactly one root and evidence chain. It reuses the regenerating-system-cache observation rule (plain existence) and the same Permanent-delete eligibility rationale as the existing GPU shader caches: the driver rebuilds OpenGL shader caches automatically.
_Avoid_: expanding nvidia_dx_cache to multiple roots, whole `%LOCALAPPDATA%\NVIDIA` cleanup, driver store cleanup

**WeChat backup artifact**:
A user-data backup managed by WeChat's own migration and backup workflow. Tencent-signed legacy code proves that `BackupFiles` and `.bakdb` concepts exist, but current and legacy generations use different storage vocabulary and no first-party contract joins a complete local path, terminal state, companion metadata, and safe external deletion. User confirmation that another copy exists cannot prove that a path- or extension-matched item is the intended independent payload. Foal therefore does not ship `wechat_files` in this slice and may provide only a path-free direction to WeChat's own backup manager; ordinary `*.db`, `Backup.db`, `Msg`, attachments, account state, and whole roots remain excluded.
_Avoid_: WeChat cleanup category, chat database, cloud-verified backup, rebuildable cache, `.bakdb` proves payload, permanent deletion

**WPS cloud space release**:
A provider-owned operation that preserves a cloud item's namespace and identity while asking WPS or Windows Cloud Files to release its hydrated local content. It is not filesystem deletion and cannot be inferred from a `WPSDrive` path: WPS synchronization may propagate local deletion to cloud state, while only a verified in-sync Cloud Files placeholder can expose system-backed cloud-copy state. Foal does not ship `wps_cloud_cache` in the current slice; it may surface at most a path-free recommendation to use WPS's own Release space UI. Any future executable capability requires a distinct action and provider-identity contract and must report asynchronous release semantics without claiming immediate reclaimed bytes.
_Avoid_: WPSDrive cleanup category, Recycle Bin cloud eviction, path proves cloud copy, delete then resync, immediate reclaimed-byte claim

**OneDrive local diagnostic artifact**:
An exact Microsoft-evidenced log or disposable cache layout under the current user's `%LOCALAPPDATA%\Microsoft\OneDrive` installation state that would need to be considered independently from OneDrive account and synchronization configuration. Current evidence establishes no executable layout, so Foal does not ship `onedrive_cache` in this slice and may surface only a path-free OneDrive Free up space or Storage Sense recommendation. The entire `settings` subtree is permanently excluded even when a descendant name resembles a log or cache; `ListSync`, updater/installer state, mixed WebView profile data, and the mixed `logs` root are also excluded. OneDrive reset guidance is a provider workflow, not authorization for Foal to reproduce it by deleting settings or diagnostic state.
_Avoid_: OneDrive cleanup category, OneDrive settings cleanup, reset-by-deletion, ODL extension cleanup, account database cache, parent-root scan, cache-like name proves disposability

**Vendor software installation**:
An installed traditional desktop application, driver, service, shared runtime, or vendor suite represented by official uninstall registration and potentially by directories under Program Files. A vendor name or directory path does not prove bloatware or leftover status, and Recycle Bin movement does not preserve installer consistency. Foal does not ship a `vendor_bloatware` Clean category: selected installed software must go through `foal uninstall` and its official uninstaller, and only a successful uninstall may unlock the frozen, freshly revalidated Confirmed leftover subset already owned by that flow.
_Avoid_: bloatware cleanup category, Program Files directory removal, vendor allowlist proves leftover, Recycle Bin as uninstall, Clean process stopping

**Permanent deletion**:
An irreversible planned deletion action that uses ordinary filesystem removal to bypass the Recycle Bin and is available only to cleanup items whose permanent-delete eligibility has been explicitly established. It is never inferred from Recycle Bin capacity, configuration, or operation failure and must be visible before confirmation. It does not overwrite data, shred files, wipe free space, or promise forensic non-recoverability.
_Avoid_: secure erase, Recycle Bin overflow fallback, retry as permanent, implicit disk-space recovery

**Permanent-delete eligibility**:
A cleanup-rule property stating that every candidate proven by that rule is a Regenerable cleanup artifact and may use Permanent deletion. Eligibility requires precise rule evidence, complete inspection, no user-authored or diagnostic state, and no unknown layout or reparse point; age, a cache-like name, or location under Temp is insufficient. Shared Clean derives the planned action automatically, and CLI and TUI derive the same action for the same category.
_Avoid_: path-name guess, age implies disposability, TUI-owned classification, per-item deletion-method toggle

**Cleanup rule action policy**:
The mandatory `planned_action` declaration on every executable canonical cleanup rule or servicing registration, using a catalog-validated stable Planned action. A new rule is incomplete without an explicit action, action-specific eligibility rationale, and contract tests; no default is inferred from category, path, family, or caller. Public category summaries, Dry-run state, execution Result, History, and the TUI shared read model project this one source of truth without parallel family booleans. Path-backed deletion rules use `move_to_recycle_bin` or `delete_permanently`; capability-bound Windows servicing uses `invoke_windows_servicing` and never masquerades as a deletion item.
_Avoid_: caller-side category ID dispatch, family-derived action, default action, TUI action override, servicing encoded as deletion

**Regenerable cleanup artifact**:
A precisely identified cache, index, downloaded runtime, or other derived artifact that contains no user-authored, diagnostic, configuration, history, or login state and can be recreated or downloaded again. Rebuild, network, or offline cost makes the category higher-impact and requires Opt-in plus an impact notice, but does not by itself remove permanent-delete eligibility.
_Avoid_: arbitrary temporary file, crash evidence, mixed-state root, safe because unused

**CLI agent local artifact**:
A local file or directory produced by a terminal-based development agent whose cleanup classification is not yet proven. It may contain regenerable cache, downloaded runtime, logs, configuration, credentials, conversation history, project state, or recovery data; only separately proven Regenerable cleanup artifacts may later become cleanup candidates.
_Avoid_: CLI agent cache, whole agent home, safe-to-delete agent data, Application cache opportunity

**CLI agent cleanup category**:
A future product-scoped Clean category for one CLI agent whose exact regenerable children, exclusions, lifecycle, and safety gates have been independently proven. `cli-agents` may become a selection alias over such categories, but is never itself an execution category and never supplies shared path or deletion semantics.
_Avoid_: one CLI-agent mega-category, shared agent-home cleanup, generic AI cache

**Grok Build update residue**:
An abandoned Windows executable backup created by Grok Build's updater beside an installed `bin\grok.exe` or `bin\agent.exe`. The candidate allowlist is exactly lowercase `grok.exe.old`, `agent.exe.old`, and their updater-generated `grok.exe.old.<pid>-<seq>.old` / `agent.exe.old.<pid>-<seq>.old` siblings where both variable fields contain only decimal digits. Every other `*.old`, `.rollback.bak`, directory, reparse point, executable, and plugin file is excluded. The category fails closed unless `grok.exe` and `agent.exe` are known idle before and after discovery, and no direct ordinary file whose name starts with `grok-` in `$GROK_HOME\downloads` has been written within one hour. A missing downloads directory means no observed update activity; an unreadable directory or unknown relevant timestamp skips the whole category. The exact candidate path and ordinary-file type are freshly revalidated before deletion. Its planned action is Permanent deletion with normal per-run authorization. Resolve an unset `GROK_HOME` to the current user's `%USERPROFILE%\.grok`; accept an override only when it is non-blank, absolute, canonicalizable, and safe. Blank, relative, unavailable, reparse-point, dangerous, or protected roots fail closed without falling back to the default or CWD. Only the direct `bin` child is inspected. CLI discovery requires exact `grok-build-update-residue`, `cli-agents`, or `all` opt-in; it is not part of `dev-caches`. Clean TUI may eagerly measure and initially select a measurable candidate under the normal removable permanent-category rule. `$GROK_HOME\downloads`, installer staging payloads, rollback backups, sessions, logs, configuration, and extensions are excluded.
_Avoid_: Grok downloads cache, old Grok versions, whole `.grok` cleanup, `*.old`

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
A cleanup item that is normally a skipped-by-default opportunity or a developer-tool Review suggestion, but that the user has explicitly opted in to clean for the current run only. Its canonical cleanup rule supplies the Planned deletion action; Permanent deletion additionally requires per-run authorization. An opt-in candidate is never a default candidate: the default candidate set stays frozen, and opt-in never becomes default. Opt-in candidates still pass fresh-scan validation, protection-rule suppression, and running-application gating at execute time, and are never deleted by running an external tool's own cleanup command. Developer-tool examples include npm, pnpm store (`pnpm-cache`), yarn cache (`yarn-cache`), go, pip, cargo, NuGet HTTP and global packages, corepack, uv cache (`uv-cache`), Bun cache (`bun-cache`), structured Playwright browser installations (`playwright-browsers`), Puppeteer browser installations (`puppeteer-browsers`), Electron download cache (`electron-cache`), JetBrains IDE caches (`jetbrains-ide-caches`), and Visual Studio regenerable caches (`visual-studio-caches`); Application cache opportunities `vscode_cache`, `cursor_cache`, `vscode_insiders_cache`, `vscodium_cache`, `windsurf_cache`, and `trae_cache` (Developer tools, via `dev-caches`) and `obsidian_cache` and `vrchat_cache` (Applications, via `app-caches`) also become Opt-in candidates when selected for the current run, independently of each other.
_Avoid_: default candidate, default-enabled rule, caller-chosen deletion method, tool-command delegation

**Opt-in candidate resolution**:
The step that turns an opt-in plan into the concrete Opt-in candidate paths for a run, performed fresh for both dry-run preview and execute so preview and execute resolve the same candidate set rather than execute trusting dry-run's resolved paths. CLI dry-run and Execute resolve only opted-in categories; the Clean TUI eager preview scan may use the same shared resolution seam to measure every opt-in category before selection, but returns only path-free category results and does not alter Clean opt-in selection. Execute still scans only opted-in categories and never trusts preview paths. A Browser cache opt-in candidate resolves to individual regenerating cache directories per profile (Chromium: `Cache`, `Code Cache`, `GPUCache`, `Service Worker\CacheStorage`; Firefox: `cache2`), not the browser profile or User Data root, because only those directories are deletable; `browser_cache` has Permanent-delete eligibility after its browser-idle-before-and-after gate succeeds. A structured developer-cache category fresh-resolves roots and fresh-discovers child candidates through the same shared seam; Execute never trusts Dry-run child paths.
_Avoid_: execute trusting dry-run resolved paths, scanning non-opted-in categories at execute, browser User Data root as an opt-in candidate, mode-specific candidate resolution

**Structured developer-cache child discovery**:
The optional private policy bound on a canonical developer-cache catalog entry that, under each resolved and unprotected root, enumerates independent child Opt-in candidates instead of treating the root as a single candidate. Shared Clean opt-in resolution applies Windows path normalization, deduplication, strict-root containment, directory-only acceptance, reparse/symlink rejection, per-child Protection, and Opportunity inspection ceiling measurement. Categories without this policy keep whole-root behavior. Public catalog projections stay path-free and never expose resolvers, allowlists, structural matchers, or executable paths.
_Avoid_: whole-root deletion of mixed-state trees, public path/allowlist catalog fields, Dry-run path manifests for Execute, recursive name guessing outside a fail-closed policy

**Structured downloadable developer-cache artifact**:
A re-downloadable installation or similar disposable artifact under a developer-tool cache root that Foal may reclaim only when a private structured child discovery policy can prove its layout. Unknown layouts, metadata, profile/state directories, incomplete installations, the cache root itself, product parents that must be preserved, regular files, links/junctions/reparse points, and paths outside the resolved root are excluded by construction until an explicit policy and test update authorizes them (ADR 0011). Shipped structured categories include `playwright-browsers`, `puppeteer-browsers`, product-scoped `jetbrains-ide-caches`, and `visual-studio-caches`; Playwright and Puppeteer browser artifacts have Permanent-delete eligibility, while every other category declares its own Cleanup rule action policy. For `puppeteer-browsers`: resolve a non-blank `PUPPETEER_CACHE_DIR` or the current user's home `.cache\puppeteer` root, then accept only allowlisted product directories (`chrome`, `chrome-headless-shell`, `firefox`) and Windows platform-version installation directories (`win32-*` / `win64-*`). The Puppeteer root and product parents are never candidates; Foal never reads Puppeteer project config, package.json, CWD, or package-manager state and never runs Puppeteer/npx commands.
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

**Machine-wide cache category**:
A Clean opt-in category whose candidates live under machine-shared roots such as `C:\ProgramData` and therefore affect every local user of the machine. It is permanently excluded from `all`, every group token, and TUI Select All: CLI discovery and execution require the exact category name, and the Clean TUI presents it initially unselected while still eagerly measuring it. Its preview carries a path-free impact notice that cleanup affects all users of this machine. Clean remains non-elevated for machine-wide categories: a root or candidate that cannot be read or deleted with current-user rights fails closed as a skip, and machine-wide scope never justifies elevation, ACL modification, or ownership changes.
_Avoid_: group-token expansion, TUI Select All inclusion, elevation for shared paths, current-user scope assumption, machine-wide implies servicing

**LGHUB download cache category**:
The first Machine-wide cache category (`lghub-cache`), reclaiming only ordinary files whose names are exactly 64 lowercase hexadecimal characters directly under the fixed `C:\ProgramData\LGHUB\cache` root — Logitech G HUB's content-addressed download blobs, re-downloaded on demand. Every other child (unknown names, directories, reparse points) and the root itself are excluded by construction. Idle gating covers LG HUB's processes and declared Windows services before and after inspection. Logitech publishes no removal contract — official troubleshooting that deletes this directory is supporting evidence, not a contract — so the planned action is `move_to_recycle_bin`; a Permanent upgrade requires separate evidence and its own decision.
_Avoid_: whole-root LGHUB cleanup, `depots` or other LGHUB siblings, permanent deletion, cache root as candidate

**Thunder update download category**:
A Machine-wide cache category (`thunder-update-download`) whose candidates are direct children of the fixed `C:\ProgramData\Thunder Network\XLLiveUD\Download` root, Thunder's resident updater download directory. A downloaded update package has consumption state (pending vs installed) that cannot be read externally, so every candidate requires a 30-day stability window: the Latest observed modification across the child and all safely inspectable descendants must be at least 30 days old, and incomplete inspection disqualifies that child. Idle gating covers the XLLiveUD service and Thunder's declared processes and services before and after inspection; any running or unknown state skips the whole category. Reparse points and the root itself are never candidates, and no recursive layout guessing occurs below the direct-child level. The planned action is permanently `move_to_recycle_bin`: update packages carry no regeneration contract, Recycle Bin restore is the recovery path if a pending package is ever selected, and Thunder's updater re-downloads on demand.
_Avoid_: permanent deletion, fresh-download cleanup, consumed-state inference, recursive layout guessing, service stopping

**Windows system temp category**:
A Machine-wide cache category (`windows-temp`) whose candidates are stale direct children of the shared system temp directory `%SystemRoot%\Temp`, resolved from the `SystemRoot` environment variable (blank/relative/UNC values are silent absence). It is the machine-shared analogue of `user_temp`: a direct child (file or directory) becomes a candidate only when its Latest observed modification across the child and all safely inspectable descendants is at least 14 days old; unknown or future timestamps fail closed, and incomplete inspection disqualifies that child. The root itself is never a candidate and reparse points are never candidates or traversed. It requires a narrow, category-owned PathSafe carve-out for exactly the resolved `%SystemRoot%\Temp` subtree — the rest of the Windows tree stays rejected, and Protection rules still apply inside. It is exact-selection-only (excluded from `all`, every group token, and TUI Select All; preview carries a path-free machine-wide impact notice). Non-elevated fail-closed: an unreadable root enumeration skips the whole category and a per-item access denial is a per-item skip, so partial reclaim is expected. The planned action is permanently `move_to_recycle_bin`; Foal never elevates, edits ACLs, queries services, or permanently deletes for this category. See ADR 0030 / ADR 0032.
_Avoid_: permanent deletion, elevation, ACL edits, service queries, whole-Windows-tree relaxation, root deletion

**Windows Update download cache category**:
A Machine-wide cache category (`windows-update-download-cache`) whose candidates are stale direct children of the Windows Update download staging directory `%SystemRoot%\SoftwareDistribution\Download`, resolved from the `SystemRoot` environment variable (blank/relative/UNC values are silent absence). A direct child (file or directory) becomes a candidate only when its Latest observed modification across the child and all safely inspectable descendants is at least 30 days old (the download payload's consumption state is not externally readable, so a long observable quiet period stands in for it); unknown or future timestamps fail closed, and incomplete inspection disqualifies that child. The root itself is never a candidate and reparse points are never candidates or traversed. It is gated on the Windows Update service stack (`wuauserv`, `bits`, `dosvc`, `UsoSvc`) being observably idle, queried read-only via SCM before discovery and again after measurement: any non-`Stopped` state is running, any query failure is unknown, and running or unknown at either observation skips the whole category with the stable path-free reason `windows_update_services_active`. Foal never starts, stops, or reconfigures those services. It requires a narrow, category-owned PathSafe carve-out for exactly the resolved `%SystemRoot%\SoftwareDistribution\Download` subtree — the `DataStore` and `ReportingEvents` siblings and the rest of the Windows tree stay rejected, and Protection rules still apply inside. It is exact-selection-only (excluded from `all`, every group token, and TUI Select All; preview carries a path-free machine-wide impact notice plus that Windows re-downloads anything still needed). Non-elevated fail-closed: an unreadable root enumeration skips the whole category and a per-item access denial is a per-item skip, so partial reclaim is expected. The planned action is permanently `move_to_recycle_bin` (an ordinary path-backed deletion, never the servicing helper); Foal never elevates, edits ACLs, mutates service state, or permanently deletes for this category. See ADR 0030 / ADR 0033.
_Avoid_: permanent deletion, elevation, service control, DataStore/ReportingEvents cleanup, DISM/servicing, whole-Windows-tree relaxation, root deletion

**Electron updater residue category**:
An opt-in Applications report-category (`electron-updater-residue`) that reclaims stale electron-builder Windows updater installer payloads under direct `%LOCALAPPDATA%` children whose names end with `-updater` (directories only, no reparse points). A matched directory participates only when every direct child is on a structural allowlist, failing closed on any unknown child: ordinary files `installer.exe` and `current.blockmap`, plus an optional `pending` directory containing only `update-info.json`, `current.blockmap`, and ordinary `*.exe`. Candidates are the individual allowlisted files (never the `-updater` directory or `pending` itself); `update-info.json` is a candidate only alongside a sibling payload `.exe` in `pending`, so a `pending` containing only `update-info.json` yields no candidates. A per-directory 24-hour quiet window skips the whole directory (stable reason `electron_update_recent`) when any allowlisted file was written within 24h, has a future/unknown timestamp, or has unreadable metadata. Policy is `shared-runtime-not-attributable` (no process detection, no elevation). It is selected via exact name, `app-caches`, `all`, and Clean TUI; never via `dev-caches` or `cli-agents`. Fresh structural revalidation runs immediately before mutation and Protection is deny-only. The planned action is permanently `move_to_recycle_bin`; Foal never permanently deletes for this category. See ADR 0031.
_Avoid_: permanent deletion, elevation, process detection/stopping, Squirrel layout cleanup, app data/config cleanup, whole-LOCALAPPDATA relaxation, root or directory deletion

**Opt-in reclaimable bytes**:
The bytes represented by opt-in candidates in a clean preview or execution, reported as a total separate from `Potential space` and `Observed opportunity bytes`. Opt-in reclaimable bytes are never merged into `Potential space`, and `Observed opportunity bytes` excludes any opportunity that has become an opt-in candidate for the run.
_Avoid_: Potential space, observed opportunity bytes, total hinted space

**Analyze (directory insight)**:
A read-only Foal command that measures an analysis root's directory totals and top children by size, and may attach only proven high-confidence classification clues. It never deletes, never contributes to Potential space, and is not a Clean opportunity scanner or recursive project-artifact finder.
_Avoid_: cleanup opportunity scanner, project scanner, disk-wide cleaner, Mole disk analyzer parity

**Analysis root**:
The single path Analyze measures. CLI omission means the process current working directory after absolute resolution; an explicit Analyze path or TUI drive choice may select a local Windows volume root. This read-only volume-root allowance belongs only to Analyze and never relaxes Clean or Purge dangerous-root validation; UNC roots and unsupported path forms still fail closed.
_Avoid_: implied multi-root, Clean/Purge volume-root authorization, UNC disk analysis, volume selection as cleanup consent

**Analyze drive entry**:
The Analyze TUI landing view that lists local fixed and removable Windows drive letters with inexpensive volume label, filesystem, total-space, and free-space metadata, without recursively scanning them. Mapped network drives, optical drives, UNC, and device paths are excluded. It initially focuses `C:` when present, otherwise the first available local volume; unavailable local volumes remain visible but cannot be entered. Choosing an available drive starts read-only analysis at that volume root; it does not select files or authorize any mutation.
_Avoid_: current-working-directory landing view, multi-drive aggregate, cleanup drive picker, implied deletion scope

**Analyze incomplete scan**:
An Analyze run that stopped because it hit the same 100,000-descendant Opportunity inspection limit (or cooperative cancellation) before finishing the tree. Totals and top children then describe only what was safely inspected; the result must not be presented as a complete directory size. JSON and human/TUI surfaces use top-level `status=incomplete` for this case (not `ok` with a side flag, and not a hard command error).
_Avoid_: silent partial totals, estimated full-tree size, treating over-limit as full success, Analyze-specific higher ceiling by default, incomplete as non-zero crash for ordinary over-limit stops

**Analyze child measurement**:
One independently bounded, read-only recursive measurement of a browse location's direct child for the Analyze TUI browser. Measurement starts only after the user enters that location; sibling drives and locations are not prefetched. Each child has its own 100,000-descendant inspection limit. Children progress separately through scanning, complete, partial, incomplete, or skipped states so one large or unreadable subtree cannot masquerade as a complete location. Partial means traversal ended with unreadable descendants; incomplete means the inspection limit or cancellation stopped traversal; skipped means the direct child itself could not be measured or was a non-traversed reparse point. Scanning, partial, and incomplete rows may show an explicitly approximate share of currently observed bytes; partial and incomplete sizes are `>=` lower bounds, but their percentages are never presented as mathematical lower bounds while the location total remains unknown. Exact percentages appear only when the location total is complete.
_Avoid_: startup whole-machine scan, sibling-drive prefetch, one shared location budget, unreadable subtree presented as complete, partial or incomplete bytes presented as exact, incomplete percentage presented as a guaranteed lower bound, unknown child treated as zero, measurement as cleanup eligibility

**Analyze logical bytes**:
The sum of filesystem-reported logical file lengths used consistently by Analyze CLI, JSON, and TUI comparisons. It is distinct from allocated disk space, may differ for compressed, sparse, or hard-linked content, and is never claimed to reconcile with volume used or free space.
_Avoid_: allocated bytes, physical disk usage, reclaimed space, exact complement of volume free space

**Analyze browse location**:
The directory currently open in the Analyze TUI browser. Entering it triggers measurement of its direct children, while entering a child creates a new browse location and a new on-demand measurement view.
_Avoid_: cleanup scope, selected candidate set, background-prefetched directory, multi-root aggregate

**Analyze browse-session cache**:
The temporary in-memory set of completed child measurements retained while navigating within one Analyze TUI session. Leaving a location cancels its unfinished work; returning reuses completed results and resumes missing measurements, while an explicit refresh discards that location's cached measurements and starts again. The cache is never persisted or used by cleanup execution.
_Avoid_: background scan after navigation, History record, cleanup manifest, stale result after explicit refresh

**Analyze focused child detail**:
A compact read-only explanation for the currently focused TUI child, including its state, observed logical bytes, file and directory counts, skipped count, and aggregated stable skip reasons. It does not expand descendant paths or turn diagnostic evidence into cleanup guidance.
_Avoid_: descendant error log, candidate detail, raw path list, cleanup recommendation

**Analyze human report**:
The non-JSON presentation of an Analyze result that surfaces the same core insight as JSON: analysis root, complete-or-incomplete status, totals including bytes, top children with size/kind/classification clues, and a skipped summary. It remains read-only guidance and must not invent cleanup actions or Potential space. Top children stay a fixed top-10 by bytes (name tie-break); no user-facing `--top` knob in this design slice.
_Avoid_: JSON-only detail, Mole-style cleanup opportunity report, execute/delete affordances, configurable top-N in the first slice

**Analyze protection non-intervention**:
User Protection rules do not suppress, skip, or reshape Analyze measurement. Analyze still skips only filesystem barriers (permission, reparse, missing, read errors). Protection continues to deny cleanup candidates and path-backed review discoveries on Clean/purge only.
_Avoid_: protection hides disk usage, analyze respects protection as scan deny-list, read-only scan implies delete authorization

**Analyze TUI browser**:
A read-only interactive view entered through Analyze drive entry for comparing and scrolling through every direct child of the current browse location, then moving into or back out of local directories, including Windows-managed directories when readable. Children continuously re-rank by their latest observed size while measurement runs, using name as the tie-break; the cursor remains bound to the selected path and the viewport follows it across rank changes. Files remain visible but non-navigable. Hidden and system children participate in measurement and remain visible with presentation-only identification; they are never framed as cleanup opportunities. Enter moves into a directory; Escape moves to its parent, then from a volume root to drive entry, and from drive entry to the Foal main menu. Navigation changes only what Analyze measures; reparse points, UNC and device paths remain non-navigable, and the browser never selects cleanup targets or exposes delete, cleanup, file-open, or file-preview actions. The CLI and JSON Analyze report retain their fixed Top 10 projection.
_Avoid_: Analyze TUI viewer, mini-Clean, cleanup selection, delete picker, TUI-owned scan engine, ad hoc Trash deletion from Analyze, system-directory cleanup authorization

**Analyze classification clue**:
A high-confidence review label attached to a measured direct child of an analysis root or TUI browse location (today: `project_artifact_clue` only). Clues explain "what this large child looks like"; they are not cleanup candidates, purge selections, or Potential space. Near-term design keeps this single classification; no expanded name allowlist and no new clue kinds without separate proof.
_Avoid_: cleanup candidate, opt-in category, reclaimable bytes, nested deep-scan hit, large-file or old-download clue types in this slice

**Analyze purge handoff**:
Read-only next-step copy on the Analyze human report and Analyze TUI browser when at least one project artifact clue is present and the current root independently passes Purge root safety validation, pointing at `foal purge <root>` for explicit-root preview and permanent reclaim. A volume root or Windows-managed tree never receives an unusable Purge handoff merely because a child name matches the clue allowlist. The handoff never launches purge, selects candidates, or authorizes deletion from Analyze.
_Avoid_: one-click purge from Analyze, Analyze-owned deletion, treating clues as purge selection

**Analyze history non-recording**:
Analyze runs do not create Foal History sessions. History remains for confirmed or mutating cleanup-class operations; repeatable read-only directory insight is not operation history.
_Avoid_: analyze session history, path-bearing analyze audit log, treating insight as cleanup provenance

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
A read-only three-state check used before and after inspecting application-owned caches: `running` means Foal does not inspect or measure the cache and reports a Running application skip, `idle` means Foal may measure it as skipped-by-default review data, and `unknown` means Foal safely skips inspection and reports a recoverable diagnostic. An unknown result never implies that the application is idle. For a supported multi-process browser, any matching browser process makes the whole browser `running`; Foal does not infer per-profile idleness from process command lines. If the application becomes running or unknown during inspection, Foal discards the measured review data and reports the safe skip instead. A registered logical application may declare exact Windows service names alongside process names: services are queried read-only through the Service Control Manager, any declared service not in the stopped state makes the application `running`, and a failed service query yields `unknown`. Foal never starts, stops, or reconfigures a service to make a cache inspectable.
_Avoid_: unknown treated as idle, process stopping, close-and-clean prompt, service stopping, service state guessed from process list

**Browser cache opportunity**:
A skipped-by-default, path-backed review discovery for a supported browser's regenerable cache directories only, measured only after Running application detection confirms the browser is idle. Shipped browsers are Google Chrome, Microsoft Edge, and Mozilla Firefox under the single `browser_cache` category. Chrome and Edge reclaim `Cache`, `Code Cache`, `GPUCache`, and `Service Worker\CacheStorage` under Chromium `User Data` profiles enumerated from `Local State` (never whole `Service Worker`, `ScriptCache`, or `Database`). Firefox reclaims only Local `cache2` under profiles enumerated from the current-user Roaming `profiles.ini` catalog (`%APPDATA%\Mozilla\Firefox`); missing catalog root is silent absence, and an existing root with a missing, unreadable, or invalid catalog produces an unknown result without guessing profile folders. Other Chromium forks remain deferred (ADR 0019). For Chrome/Edge, Foal uses the current user's browser data root as the existence boundary: a missing `User Data` root is silently absent, while an existing root with a missing, unreadable, or invalid `Local State` profile catalog produces an unknown result. Foal does not use installation discovery or guess profile directories by scanning AppData. JSON represents one Browser cache opportunity per browser with total observed bytes, profile count, and profile-specific cache detail; human output shows the browser summary, while detailed review surfaces may expand the profile paths. A browser summary is reported only when every identified profile can be inspected completely; any incomplete profile inspection discards the whole browser's measured result rather than presenting a partial total. If any profile cache path is protected by Protection rules, Foal suppresses the entire browser opportunity before totals and downstream projection instead of presenting a partial browser summary. A recognized cache directory that does not exist contributes zero bytes and is not an incomplete inspection; a browser whose complete recognized cache total is zero produces no Opportunity. Each existing recognized cache directory uses the standard 100,000-descendant Opportunity inspection limit, and an unsafe, unreadable, canceled, or over-limit inspection invalidates the browser summary. Cookies, history, credentials, extensions, download records, whole `Service Worker` parent plus `ScriptCache`/`Database` (registration/script state), form data, sessions, places, logins, and whole browser profile directories are permanently excluded for every browser; Chromium `Service Worker\CacheStorage` remains an allowlisted regenerable root. Playwright/Puppeteer tool-managed browser downloads stay outside Browser cache opportunity.

_Avoid_: browser data, browsing history, cookies, credentials, privacy cleaner, default candidate

**Application cache opportunity**:
A skipped-by-default, path-backed review discovery for regenerating caches owned by a non-browser application, measured only after Running application detection confirms the logical application is idle before and after inspection. Discovery uses one reusable private seam: a registered application policy plus an exact relative-root allowlist under that application's single declared current-user AppData base (`roaming`, `local`, or `locallow`); the base is per-application policy, not a seam-wide constant, and shipped editor categories plus `obsidian_cache` all declare the Roaming base. Each existing allowlisted directory is an independent Opportunity or Opt-in candidate with its own path and bytes; Foal never selects roots by substring or recursive user-data enumeration. Complete categories are `vscode_cache` for Visual Studio Code (`visual_studio_code` / `Code.exe`) under `%APPDATA%\Code`, `cursor_cache` for Cursor (`cursor` / `Cursor.exe`) under `%APPDATA%\Cursor`, `vscode_insiders_cache` for VS Code Insiders (`visual_studio_code_insiders` / `Code - Insiders.exe`) under `%APPDATA%\Code - Insiders`, `vscodium_cache` for VSCodium (`vscodium` / `VSCodium.exe`) under `%APPDATA%\VSCodium`, `windsurf_cache` for Windsurf (`windsurf` / `Windsurf.exe`) under `%APPDATA%\Windsurf`, and `trae_cache` for Trae (`trae` / `Trae.exe`) under `%APPDATA%\Trae`, each with the same shared editor allowlisted roots `Cache`, `CachedData`, `CachedExtensionVSIXs`, `Code Cache`, `GPUCache`, `DawnGraphiteCache`, and `DawnWebGPUCache`; `obsidian_cache` for Obsidian (`obsidian` / `Obsidian.exe`) under `%APPDATA%\obsidian` is a non-editor Electron app that carries its own plain-Electron allowlist (`Cache`, `Code Cache`, `GPUCache`, `DawnCache`, `DawnGraphiteCache`, `DawnWebGPUCache`) excluding `CachedData` and `CachedExtensionVSIXs`; and `vrchat_cache` for VRChat (`vrchat` / `VRChat.exe`), application directory `VRChat\VRChat` under the LocalLow base, with the single-root allowlist `Cache-WindowsPlayer` — downloaded avatar/world content that VRChat re-downloads on demand, carrying a mandatory re-download impact notice; VRChat's in-game Clear Cache button is a GUI feature, not an external tool command, so the category remains an opportunity rather than a Review suggestion. Editor categories sit under the Developer tools report category and expand via `dev-caches`; `obsidian_cache` and `vrchat_cache` sit under the Applications report category and expand via `app-caches`. All have Permanent-delete eligibility, including the re-downloadable `CachedExtensionVSIXs` root whose impact notice remains mandatory. Application categories, roots, and process identities are independent: running or selecting one application never authorizes or suppresses another. Missing or blank AppData or a missing application root is silent absence. Pre-inspection running, unknown, missing required state, or snapshot failure skips all roots for that application without measuring; post-inspection unsafe state discards every measured root and byte total for that application only. Incomplete or canceled inspection contributes no bytes for the interrupted root; non-canceled incomplete siblings may leave completed roots independently represented. Protection suppresses protected roots before totals and downstream projection without authorizing siblings. Settings, profiles, workspace/global storage, backups, installed extensions, Service Worker and web storage, Network/cookies/credentials, logs, Crashpad, and unknown directories are excluded. Cursor evidence is limited to the exact PRD allowlist under the standard root—do not broaden from VS Code ancestry. Portable mode, Insiders/forks, installation discovery, process command-line inspection, and `--user-data-dir` inference are out of scope.
_Avoid_: whole editor user-data cleanup, recursive AppData scanning, browser-named policy for non-browser apps, process stopping, default candidate, shared VS Code/Cursor gate, Roaming-only seam assumption

**Clean preview read model**:
A shared representation of clean preview sections, candidates, skipped-by-default items, review clues, suggestions, protection rules, notices, totals, and detailed-list metadata for JSON, human output, and future TUI consumers.
_Avoid_: CLI string builder as model, TUI-owned cleanup model

**TUI review surface**:
The interactive Foal interface for browsing existing command read models, comparing preview sections, navigating review evidence, and, for Clean and Uninstall, orchestrating an explicitly confirmed action through the shared command execution path without owning cleanup, uninstall, or path-safety decisions.
_Avoid_: TUI-owned cleanup engine, TUI-owned uninstaller runner, replacement command path, implicit execution

**Foal main menu**:
The top-level interactive TUI entry that appears when a user explicitly starts Foal's interactive mode, offering command navigation for clean, uninstall, analyze, status, and related views while preserving each command's existing CLI and JSON contract.
_Avoid_: default execution hub, hidden command behavior change, feature-parity clone menu

**Interactive default entry**:
The no-argument `foal` behavior in an interactive terminal, launching the Foal main menu while preserving non-interactive and JSON-oriented command behavior for scripts, pipes, and automation.
_Avoid_: blocking non-TTY scripts, replacing help semantics everywhere, implicit command execution

**Main menu command entries**:
Top-level Foal main menu items that expose the command map: Clean opens its interactive preview and confirmed-action flow; Uninstall opens multi-select confirmed uninstall through shared Uninstall execution; Analyze opens its dedicated read-only drive-entry and on-demand ranked browser; Status and History use read-only command viewers. The Uninstall TUI is an adapter only: it collects multi-select app names, one confirmation that discloses official vs portable plans, confirmed leftover scope, process-stop opt-in, permanent authorization, and admin-need grouping, then calls the same shared Uninstall Execute path as the CLI; it owns no uninstaller launch, path safety, Protection, elevation, or deletion logic.
_Avoid_: pretending every command has a completed TUI, implicit execution, hiding unavailable capability, Analyze as a cleanup or purge surface

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
The exact per-run set of canonical default and opt-in category identifiers chosen for Clean TUI confirmation. Default categories and every category with Permanent-delete eligibility start selected but may be cleared; Recycle Bin opt-in categories start unselected. The current catalog therefore starts with 37 selected categories (1 default + 36 permanent, including `user_temp`, `crash_dumps`, `explorer_thumbnail_cache`, `inet_cache`, `nvidia_gl_cache`, `vrchat_cache`, `trae_cache` and `obsidian_cache`) and leaves `windows_error_reporting` unselected (plus the Recycle Bin opt-ins: the exact-selection-only `nvidia_installer_cache`, `lghub_cache`, `thunder_update_download`, the machine-wide `windows-temp`, and the machine-wide `windows-update-download-cache`, and the Standard-selection `electron-updater-residue`). Confirmation authorizes exactly the visible selection and must not silently add a category or consume preview paths; an `empty`, `skipped`, `incomplete`, or `failed` outcome removes and disables that category for the current scan. CLI additive Opt-in remains unchanged.
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
Observation-only shared Clean events for the current execution phase, optional path-free `ActiveCategory` (canonical category id at resolve/mutate boundaries; empty for aggregate Recycle Bin safety and completion), optional path-free mid-flight `CompletedCategory` provisional terminals (`empty`/`cleaned`/`partial`/`skipped`/`failed`/`canceled` with honest deleted/skipped counts and successful affected bytes when already recorded), and path-free per-selected-category states such as `waiting`, `rechecking`, `ready`, `cleaning`, plus those terminals, without candidate paths or byte-derived percentages. Progress is not part of the JSON result and never authorizes candidates or drives safety decisions; provisional mid-flight terminals are emitted only after shared Clean finishes that category's resolve+mutate work for the run (or resolve proves empty/skip with no further work) and are completely overwritten by the authoritative final Result; history remains authoritative.
_Avoid_: TUI-inferred progress, byte-derived percentage, candidate path stream, execution manifest, progress as cleanup authorization, rollback promise, inventing success bytes

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
A four-stage Clean-specific TUI interaction boundary (eager category-first preview → exact selection with measured totals → separate confirmation → shared execution/result) where browsing and selection remain side-effect free and the first slice exposes no retry or rescan. This term does not constrain the separately implemented Uninstall TUI flow or the read-only Analyze browser.
_Avoid_: TUI-owned execution engine, implicit cleanup, browsing-as-operation history noise, conflating Clean and Uninstall authorization, deferred retry documented as current

**Uninstall preview report**:
A human-readable presentation surface rendered directly over the uninstall preview read model, mirroring the Mole-inspired report style for dry-run and review before any mutation.
_Avoid_: silent execution log, Mole for Windows parity claim

**Possible leftovers**:
Filesystem paths Foal confidently associates with one discovered, still-installed application (app-owned, high confidence) that would likely remain after an uninstall. They are the only leftover class that may enter a Confirmed leftover path set under Uninstall execution; shared-state and unknown findings stay out.
_Avoid_: orphan residue of an already-removed application, automatic deep-scan leftovers, implying the application is already gone

**Orphaned residue**:
Filesystem paths that look like application data but are not tied to any currently discovered installed application, surfaced as low-confidence read-only review clues and owned by Clean-side or review surfaces—not by Uninstall execution.
_Avoid_: possible leftovers, uninstall execution target, app-owned footprint, safe-to-clean residue

**Not-inspected state**:
A report state asserting that Foal did not examine a discovery category at all, kept distinct from an inspected-but-empty result so the report never implies an examination that did not happen.
_Avoid_: none found, no leftovers, empty result

**Uninstall execution**:
The confirmed mutation path for selected still-installed applications: invoke each app's Official uninstaller when available (or Portable directory removal when eligible), optionally stop processes only with explicit authorization, then remove only revalidated members of the Confirmed leftover path set under Protection rules and History recording.
_Avoid_: preview-only forever, Clean category, orphan bulk delete, TUI-owned uninstaller engine

**Official uninstaller invocation**:
Running a registry-advertised uninstall command for a traditional desktop application, preferring a quiet uninstall string when present and falling back to the interactive uninstaller when quiet fails or is absent.
_Avoid_: deleting Program Files as the primary uninstall mechanism, package-manager uninstall as the first-slice default

**Portable directory removal**:
An exceptional uninstall path used only when no uninstall command exists and a trusted install location is known, requiring explicit permanent-deletion authorization and never used as a silent fallback after a failed official uninstaller in the first execution slice.
_Avoid_: force removal of broken apps, InstallLocation guess without evidence, Recycle Bin for whole install trees by default

**Confirmed leftover path set**:
The frozen upper bound of high-confidence Possible leftover paths disclosed at Uninstall confirmation; after a successful Official uninstaller (or eligible Portable directory removal), Foal may delete only a revalidated subset of that set and must never add paths that were not confirmed.
_Avoid_: post-uninstall deep rescan expansion, orphaned residue inclusion, registry leftover purge

**Uninstall hard exclusion**:
An application Foal never offers for Uninstall execution (including Foal itself and a small fixed denylist), distinct from discovery filters that hide system components from the install list.
_Avoid_: user Protection path, optional skip, soft warning only

**Uninstall elevation exception**:
The product rule that Uninstall execution may request Windows administrator consent (UAC) when a selected app needs it, while Clean, Purge, and other commands remain non-elevating and report permission failures as skips.
_Avoid_: automatic elevation for Clean, silent privilege escalation, product-wide elevation default
