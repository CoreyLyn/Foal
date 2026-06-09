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

**Review clues**:
Read-only cleanup hints that Foal surfaces for manual investigation without treating them as cleanup candidates.
_Avoid_: cleanup candidates, executable actions

**Human report labels**:
Plain ASCII presentation-only labels used in non-JSON output to make preview state, skipped state, clean state, clues, and review suggestions easier to scan.
_Avoid_: Unicode symbols, JSON status codes, execution semantics

**Permission boundary notice**:
A human-readable notice that explains protected or administrator-only locations were skipped without recommending elevation as the normal path.
_Avoid_: full preview prompt, automatic elevation, run as administrator recommendation

**Protection rules**:
Foal's active cleanup safety boundaries, including default Windows path-safety rules and any future user-defined protection entries.
_Avoid_: whitelist, allowlist-only model

**Detailed candidate list**:
A human-readable companion file for clean preview reports that records candidates, skipped items, review clues, and reasons without authorizing later execution.
_Avoid_: execution manifest, deletion input

**Review suggestions**:
Human-readable commands or next steps for external tools or manual investigation that Foal displays without executing or counting as Foal cleanup actions.
_Avoid_: cleanup actions, delegated execution

**Potential space**:
The bytes represented by Foal default candidates in a clean preview, excluding skipped-by-default items, review clues, external tool suggestions, and permission-boundary skips.
_Avoid_: total hinted space, external savings estimate

**Project artifact clue**:
A review clue for rebuildable project directories or build outputs that Foal may surface only through explicit analysis or future opt-in flows.
_Avoid_: default project scan, default clean candidate

**Running application skip**:
A skipped-by-default report state for cleanup opportunities tied to currently running applications or services, especially sync clients, browsers, IDEs, AI tools, containers, and virtualization tools.
_Avoid_: close-and-clean prompt, default candidate

**Clean preview read model**:
A shared representation of clean preview sections, candidates, skipped-by-default items, review clues, suggestions, protection rules, notices, totals, and detailed-list metadata for JSON, human output, and future TUI consumers.
_Avoid_: CLI string builder as model, TUI-owned cleanup model

**TUI review surface**:
A future interactive Foal interface for browsing existing command read models, comparing preview sections, and navigating review evidence without owning cleanup, uninstall, path-safety, or execution decisions.
_Avoid_: TUI-owned cleanup engine, TUI execution model, replacement command path

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
Top-level Foal main menu items that expose the implemented command map, with `Clean` entering the first real TUI preview view and other commands remaining command navigation or later TUI extension points until their read-only views are designed.
_Avoid_: pretending every command has a completed TUI, launching destructive flows, hiding unavailable capability

**Foal TUI brand frame**:
The visual shell for Foal's interactive surfaces, using Foal-owned ASCII branding, a Windows preview-first tagline, scan-friendly command descriptions, and compact keyboard hints without copying Mole's product wording, Mac positioning, or optimize-first promise.
_Avoid_: Mole brand clone, Mac maintenance wording, decorative UI that obscures safety state

**Clean TUI preview view**:
The first TUI review surface slice, focused on browsing the existing clean preview read model for `foal clean --dry-run` sections, totals, candidates, skipped items, review clues, notices, suggestions, and detailed-list metadata.
_Avoid_: multi-command TUI platform, new scanner rules, TUI cleanup execution

**Read-only TUI action model**:
A TUI interaction boundary where navigation, filtering, expansion, scrolling, and copy-oriented review affordances are allowed, while cleanup execution, uninstaller execution, process stopping, elevation prompts, and leftover deletion are absent.
_Avoid_: execute button, confirmation flow, destructive TUI action

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
