# Product-scoped JetBrains IDE cache opt-in uses structured children and independent product gates

## Status

Accepted. **ADR 0018:** `jetbrains-ide-caches` uses planned action `delete_permanently` with per-run permanent authorization; product-scoped discovery, exclusions, and independent idle gates below remain in force. Recycle Bin capacity pre-checks do not apply to this permanent category.

## Context

Foal already reclaims structured developer-cache children (ADR 0011) and can gate distinctive-process tools before and after measurement. JetBrains IntelliJ-platform IDEs store regenerating indexes under `%LOCALAPPDATA%\JetBrains\<Product><Version>` alongside Local History, plugins, project state, and other non-disposable data. Whole-root deletion would destroy recovery data. Multiple editions and versions share launcher process names, so one running IDE must skip every root for that logical product without blocking a different idle JetBrains product.

Issue #207 introduced the internal product-scoped root/gating seam (`DevCacheRootScope.Application`, catalog `resolveRootScopes`). This decision ships the first complete public category on that seam.

## Decision

Register one Developer tools opt-in category `jetbrains-ide-caches` (label `JetBrains IDE caches`) with distinctive-process running-application policy and structured child discovery.

- Resolve only the current user's non-blank standard `%LOCALAPPDATA%\JetBrains` parent. Missing or blank Local AppData yields silent absence.
- Match direct children with an anchored private product catalog: prefix + `YYYY.N` where `N >= 1` and year/version is at least 2020.1. Catalogued products (deterministic order): IntelliJ IDEA Ultimate (`IntelliJIdea`) and Community (`IdeaIC`) → `intellij_idea` (`idea64.exe`, `idea.exe`); PyCharm Professional (`PyCharm`) and Community (`PyCharmCE`) → `pycharm` (`pycharm64.exe`, `pycharm.exe`); then WebStorm, PhpStorm, RubyMine, CLion, DataGrip, DataSpell, GoLand, RustRover, Aqua, MPS, Writerside with matching logical identities and 64/32-bit Windows launchers; Rider → `rider` (`rider64.exe`, `rider.exe`) with Rider-only exact `resharper-host` child. Longer prefixes win (e.g. `PyCharmCE` before `PyCharm`).
- Each product-version directory is a product-scoped root, never a candidate. Discover only exact `caches` and `index` children as independent Opt-in candidates (deterministic catalog/version/child order); Rider additionally discovers exact `resharper-host`.
- Permanent exclusions include Local History, file history, VCS Log, JCEF, plugins, logs, coverage, projects/data-source/editor/full-line/tmp/splash/metadata, unknown children, pre-2020 layouts, non-catalog products (Fleet/Air/Gateway/Client; Android Studio; Toolbox/Installations/Daemon/Shared/dotPeek/standalone ReSharper roots), regular files, and reparse points.
- Idle-before-and-after gates are independent per logical product. Running/unknown/missing state for one product skips every root for that product only; post-scan unsafe state discards only that product's measured children.
- Dry-run, Clean TUI eager preview, and Execute share the same fresh category resolution seam. Execute never trusts preview paths; every candidate is validated immediately before permanent deletion; permanent authorization is required; permanent deletion is never used as a Recycle Bin fallback for other categories.
- Default CLI Execute without opt-in performs no JetBrains root resolution or process detection. Clean TUI may measure the category while unselected under the existing TUI-only rule. Selection uses the canonical category identifier only.
- Foal never reads config roots, install dirs, Toolbox, registry, CWD, projects, `idea.system.path`, properties, command lines, or window titles, and never invokes or stops JetBrains software.
- Public JSON/human/TUI contracts stay ordinary Opt-in surfaces; product prefixes and launchers remain private Clean policy.

## Consequences

- Adding another compatible JetBrains IDE is a private catalog entry plus tests, not a new public category or pipeline.
- CONTEXT, AGENTS, README, and plan enumerations list `jetbrains-ide-caches` with the exact product and exclusion boundaries above.
- Tests use shared Dry-run/Execute as the primary seam, with table-driven edition prefixes, independent product gates, Protection, permanent-authorization and failure paths, cancellation, and shared Clean execution coverage.
