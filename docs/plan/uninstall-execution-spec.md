## Problem Statement

Foal can review installed Windows applications and related evidence (`foal uninstall`), but it cannot complete Mole-style uninstall work: remove a still-installed app and clean only the high-confidence leftovers the user already saw. Users who want that job today must use a third-party uninstaller or manually delete folders, while Foal stays preview-only and cannot turn a confirmed selection into Official uninstaller invocation, optional process stop, Portable directory removal, or a Confirmed leftover path set under Protection rules and History.

## Solution

Ship Uninstall execution aligned with Mole’s product split and ADRs 0026–0028: when apps are still installed, users multi-select them, preview once, confirm once, then Foal runs Official uninstaller invocation (quiet then interactive), optionally stops processes only when authorized, and deletes only revalidated members of the Confirmed leftover path set (Possible leftovers / app-owned high confidence). Portable directory removal is the exception path when no uninstall command exists but a trusted install location is known. CLI and TUI share one core Execute path; dry-run remains default; orphaned residue, registry purge, Store/MSIX, package managers, and Force Removal stay out of this slice.

## User Stories

1. As a Windows power user, I want Foal uninstall to remove still-installed apps plus their high-confidence leftovers, so that I get Mole-like complete removal without a separate GUI uninstaller suite.
2. As a cautious user, I want uninstall to stay preview-only until I explicitly authorize execution, so that browsing the app list never mutates my system.
3. As a CLI user, I want `foal uninstall` (and `--json`) to keep working as review output, so that scripts and habits built on preview do not break.
4. As a CLI user, I want `--execute` to be required for mutation, so that accidental runs cannot uninstall software.
5. As a CLI user, I want process stopping to require a separate authorization flag, so that Foal does not kill apps unless I allow it.
6. As a CLI user, I want permanent deletion (portable install trees) to require `--allow-permanent`, so that irreversible deletes match Clean’s authorization style.
7. As a TUI user, I want to multi-select installed apps from the Uninstall view, so that I can batch-remove several apps like Mole.
8. As a TUI user, I want one confirmation that lists uninstall actions and leftover paths, so that I understand the full plan before anything runs.
9. As a TUI user, I want confirmation to supply the same authorizations as CLI flags (execute, stop process, permanent), so that CLI and TUI stay equivalent.
10. As a user, I want quiet uninstall tried before an interactive vendor wizard, so that scripted and terminal flows stay smooth when possible.
11. As a user, I want interactive uninstall to run when quiet fails or is missing, so that stubborn apps can still be removed.
12. As a user, I want apps without an uninstall command but with a trusted install location to be removable only after permanent authorization, so that portable-style apps are covered without guessing.
13. As a user, I want failed official uninstalls to skip leftover deletion and not fall back to deleting the install tree, so that a broken uninstaller cannot become silent force-delete.
14. As a user, I want leftover deletion limited to Possible leftovers disclosed at confirmation, so that Foal never deletes paths I did not see.
15. As a user, I want post-uninstall leftover work to revalidate paths and only delete a subset of the confirmed set, so that already-cleaned or protected paths are not forced.
16. As a user, I want orphaned residue, shared-state concerns, and unknown state excluded from Uninstall execution, so that low-confidence data is never batch-deleted as part of uninstall.
17. As a user, I want user-profile leftovers moved to the Recycle Bin by default, so that mistaken leftover cleanup is recoverable.
18. As a user, I want portable whole-directory removal to be permanent and separately authorized, so that large install trees do not silently use the Recycle Bin policy incorrectly.
19. As a user, I want Protection rules to suppress leftover (and portable) targets, so that paths in protection.txt are never deleted by uninstall.
20. As a user, I want apps that need administrator rights grouped and disclosed before confirmation, so that UAC is expected rather than surprising mid-batch.
21. As a user, I want Uninstall to be allowed to request UAC when needed, so that machine-wide desktop apps can actually uninstall.
22. As a user, I want Clean, Purge, and other commands to remain non-elevating, so that Uninstall’s elevation exception does not weaken the rest of Foal.
23. As a user, I want running apps flagged in preview/confirm, so that I can decide whether to authorize process stop.
24. As a user, I want process stop off by default, so that open work is not killed without consent.
25. As a user, I want batch uninstall to continue other selected apps when one fails, so that one bad uninstaller does not abort the whole selection.
26. As a user, I want clear per-app success, skip, and failure reasons in the result, so that I know what happened.
27. As a user, I want History to record which apps were targeted, outcomes, and leftover paths touched, so that I can audit destructive uninstall work later.
28. As a user, I want Foal itself and a small hard denylist excluded from selection, so that I cannot uninstall critical or self-referential entries by mistake.
29. As a user, I want system-component-style discovery filters retained, so that hidden Windows components stay out of the normal list.
30. As a user, I want only traditional registry Uninstall-key desktop apps in this slice, so that Store/MSIX and package-manager complexity does not block the first useful path.
31. As a user, I want no registry leftover editing in this slice, so that Foal does not act like an aggressive registry cleaner.
32. As a user, I want no Force Removal for broken apps in this slice, so that high-risk delete-without-uninstaller stays a later explicit design.
33. As a user, I want dry-run to show planned official uninstall vs portable removal and planned leftover actions, so that I can review without `--execute`.
34. As a JSON automator, I want stable fields for execution policy, authorizations, per-app outcomes, and leftover outcomes, so that tools can parse uninstall runs.
35. As a user, I want already-uninstalled orphan cleanup to remain Clean/Orphaned residue review, so that Uninstall does not steal Mole’s clean-side leftover job.
36. As a developer reading the product, I want TUI to call shared Uninstall execution rather than own uninstaller logic, so that safety stays in one place.
37. As a user after confirmation, I want the leftover path set frozen as a ceiling, so that a later rescan cannot add new deletes I never confirmed.
38. As a user, I want leftover deletion only after the uninstaller reports success (exit success), so that cancelled or failed uninstall wizards do not trigger cleanup.
39. As a user, I want install-location and uninstall-command evidence included in discovery for execution planning, so that Foal can choose Official uninstaller invocation vs Portable directory removal.
40. As a user on non-Windows, I want uninstall to remain a skipped/unsupported preview story, so that Windows-only mutation is not faked elsewhere.

## Implementation Decisions

- Product boundaries follow ADR 0026 (still-installed apps only), ADR 0027 (execution model), and ADR 0028 (Uninstall elevation exception only).
- Primary seam is Uninstall package **Review** (preview) plus **Execute** (mutation). CLI and TUI only parse input, collect confirmation/authorizations, and call that seam—same pattern as Clean execute and Purge execute.
- Do not implement Uninstall mutation inside the Clean execute pipeline or as Clean categories. Do not implement path safety, Protection, uninstaller launch, or deletion inside the TUI layer.
- Extend application discovery for traditional desktop apps to capture uninstall command strings (quiet and interactive), install location when present, and enough identity to select and history-record apps stably (beyond display name alone where registry identity exists).
- Classification at plan time:
  - Has uninstall command → Official uninstaller invocation (prefer quiet, then interactive).
  - No uninstall command + trusted install location → Portable directory removal (requires permanent authorization).
  - Neither → not executable; report skip with stable reason.
  - Uninstall hard exclusion / system filters → not selectable or rejected at execute with stable reason.
- Confirmation builds an immutable **Confirmed leftover path set** from Possible leftovers (app-owned, high confidence) for selected apps only. Shared-state, unknown, and Orphaned residue never enter the set.
- Execute pipeline (conceptual phases): authorize → load Protection (fail closed on load errors like Clean) → for each selected app in stable order: optional process stop if authorized and needed → official uninstall or portable removal → on success only, revalidate confirmed leftovers for that app → Recycle Bin move for leftover files; permanent only for authorized portable trees → record per-app and per-path outcomes → History session.
- Success gate for leftovers: uninstaller process exit success (C1). Do not require registry key disappearance. Do not expand leftover set after uninstall (M3).
- Deletion: reuse shared delete/path-safety capabilities used by Clean/Purge (Recycle Bin vs permanent). Leftovers default to Recycle Bin. Portable install-tree removal is permanent and skipped without permanent authorization (no silent Recycle Bin fallback that changes disclosed action).
- Process stop: off unless CLI flag or TUI confirmation authorization is set; if running and not authorized, skip uninstall for that app with clear reason (or equivalent fail-closed policy consistent with disclosure—document stable skip reason in contracts).
- Elevation: group apps that likely need admin before confirmation (V3). Uninstall may request UAC (E1/ADR 0028). Other commands unchanged.
- CLI contract (names may be bikeshed only if tests stay consistent): dry-run default; `--execute`; separate process-stop authorization; `--allow-permanent` for portable permanent removal. JSON must expose planned vs actual actions and skip reasons.
- TUI: replace read-only-only Uninstall path with multi-select + confirmation + result, calling the same Execute; presentation may be path-compact where Clean TUI already prefers path-free UX, but confirmation must still make leftover scope understandable (paths available via existing JSON/detail patterns if the primary list is compact).
- History: full session + item records for apps and leftover paths (H1), command surface distinguishable from Clean/Purge.
- First-slice scope fixed: traditional registry uninstall apps only; no registry mutation; no Force Removal; no Store/MSIX; no winget/scoop/choco primary path; no automatic elevation outside Uninstall.
- Update user-facing help/command descriptions when execution ships so they no longer claim uninstall is permanently preview-only; until ship, preview-only remains correct runtime behavior.
- Domain vocabulary: Official uninstaller invocation, Portable directory removal, Confirmed leftover path set, Uninstall hard exclusion, Uninstall elevation exception, Possible leftovers, Orphaned residue, Protection rules.

## Testing Decisions

- Good tests assert external behavior: Result/JSON fields, skip reasons, which paths were deleted or preserved, History contents, and authorization gates—not private helper structure or exact Windows API call sequences.
- Prefer the highest seam: unit/integration tests against Uninstall **Execute** (and Review plan output) with injected fakes for uninstaller runner, process stop, elevate, filesystem/delete adapters, and Protection file contents. Add CLI contract tests that flags map to the same authorizations. TUI tests cover selection/confirm wiring only, not re-implementing uninstall logic.
- Prior art: Clean execute tests with injected recycle-bin adapters and history recorders; Purge execute + `--allow-permanent` CLI tests; existing Uninstall Review/report JSON tests; leftover footprint unit tests with fake directory listers.
- Required behavior cases (non-exhaustive): preview without execute mutates nothing; execute without permanent skips portable trees; execute without process authorization skips or refuses running apps per policy; quiet then interactive ordering; failed uninstaller does not delete leftovers; confirmed leftover set never expands; protected paths skipped; hard exclusions rejected; batch continues after one failure; History written on execute; elevation grouping visible in plan/result metadata; orphaned residue never deleted by Execute.
- Platform: Windows behavior tests with fakes; non-Windows remains skip/preview without mutation.

## Out of Scope

- Store / MSIX / AppX uninstall.
- winget, Scoop, Chocolatey, or other package-manager uninstall as a primary path.
- Registry leftover scanning or deletion.
- Force Removal for broken or missing uninstallers (beyond Portable directory removal eligibility).
- Deleting Orphaned residue, shared-state concerns, or unknown state via Uninstall.
- Secure erasure, automatic elevation for Clean/Purge, or product-wide elevation defaults.
- Making Analyze or Clean own uninstall execution.
- Mole feature parity for macOS-only concepts (bundle id graphs, LaunchAgents, Dock, Homebrew cask) beyond the agreed Windows mapping.
- Changing Clean’s default candidate matrix or deletion policy.
- Shipping execution without tests for authorization and non-expanding leftover ceilings.

## Further Notes

- Seams agreed: single product seam at Uninstall Review/Execute with injectable ports; CLI/TUI are adapters only.
- Decisions crystallized in grill-with-docs: W3, L1, D3, P3, E1, S1, U3, X2, B2, C1, R1, F3, H1, T1, A2, V3, M3, Y2, Z2.
- Glossary updates live in CONTEXT.md; design ADRs are 0026, 0027, 0028.
- Runtime code is still preview-only until this spec is implemented; agents must not document execution as shipped before the contract tests pass.
- Suggested implementation order inside the issue: discovery fields + plan model → Execute with fakes → leftover ceiling + Protection + delete → History → CLI flags/JSON → TUI multi-select/confirm → docs/help cutover.
