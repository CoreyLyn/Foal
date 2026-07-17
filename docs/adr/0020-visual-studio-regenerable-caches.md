# Visual Studio regenerable cache opt-in uses exact allowlisted children and devenv idle gating

## Status

Accepted. Category `visual-studio-caches` uses planned action `delete_permanently` with per-run permanent authorization. Discovery, exclusions, and devenv idle gates below remain in force. Recycle Bin capacity pre-checks do not apply to this permanent category.

## Context

ADR 0019 approved a Visual Studio caches category with an exact allowlist, idle gate, and no whole-ProgramData scan. Full Visual Studio (not VS Code/Cursor) stores regenerable MEF and Roslyn caches under the current user's `%LOCALAPPDATA%\Microsoft\VisualStudio` tree beside settings, extensions, packages, and other non-disposable state. Whole-parent deletion would wipe configuration. Process identity is `devenv.exe`, independent of `Code.exe` / `Cursor.exe`.

Evidence for the allowlist:

- Microsoft Q&A and MEF tooling document deleting `%LOCALAPPDATA%\Microsoft\VisualStudio\<instance>\ComponentModelCache` while Visual Studio is closed (`devenv.exe` absent); the component model cache rebuilds on next launch.
- Microsoft Q&A documents deleting the shared `%LOCALAPPDATA%\Microsoft\VisualStudio\Roslyn` folder as a regenerable compiler/analyzer cache (rebuild cost only).

Unproven or high-risk siblings (Settings, Extensions, PackageCache, MEFCacheBackup, template caches, WebView2Cache, instance-level Roslyn, ProgramData packages) stay excluded until separate evidence.

## Decision

Register one Developer tools opt-in category `visual-studio-caches` (label `Visual Studio caches`) with distinctive-process running-application policy and structured child discovery.

- Resolve only the current user's non-blank standard `%LOCALAPPDATA%\Microsoft\VisualStudio` parent as one product-scoped root gated by logical application `visual_studio` (`devenv.exe`). Missing or blank Local AppData or a missing parent yields silent absence.
- Discover only exact allowlisted regenerable children:
  - shared exact `Roslyn` under the parent;
  - exact `ComponentModelCache` under each direct child whose name is an anchored instance/version directory `major.minor` or `major.minor_<hex>` with major ≥ 14 (VS 2015+).
- The VisualStudio parent, instance hives, Settings, Extensions, Packages, MEFCacheBackup, template caches, WebView2Cache, ProgramData, Roaming settings, solutions, and every unknown sibling are never candidates.
- Idle-before-and-after gate uses `devenv.exe` only. Running/unknown/missing state fails closed for the whole category. Running VS Code or Cursor never authorizes or suppresses Visual Studio.
- Dry-run, Clean TUI eager preview, and Execute share the same fresh category resolution seam. Execute never trusts preview paths; every candidate is validated immediately before permanent deletion; permanent authorization is required.
- Default CLI Execute without opt-in performs no Visual Studio root resolution or process detection. Clean TUI may measure the category while applying normal permanent initial-selection rules. Selection uses the canonical category identifier only.
- Foal never reads install dirs, vswhere, registry, Roaming settings, solutions, command lines, or window titles, and never invokes or stops Visual Studio.

## Consequences

- Broadening the allowlist (for example template caches or WebView2) requires new evidence and tests; fail closed until then.
- CONTEXT, AGENTS, README, and plan enumerations list `visual-studio-caches` with the exact boundaries above.
- Tests use shared Dry-run/Execute as the primary seam, with instance-name tables, independent editor gates, Protection, permanent authorization, failure paths, cancellation, and catalog registration coverage.
