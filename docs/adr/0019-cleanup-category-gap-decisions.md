# Cleanup category gap decisions after market comparison

After comparing Foal's Clean matrix to Windows built-in tools (Disk Cleanup, Storage Sense), third-party cleaners (CCleaner-class, BleachBit, etc.), and Mole-style developer cleanup, we locked product scope for missing categories: stay open to discussing boundary breaks, then deliberately reaffirm no elevation, no Recycle Bin emptying, no user-content cleanup, and no privacy/registry/secure-erase surface; expand only proven regenerable caches and an independent project-artifact purge flow.

## Do (approved backlog, implementation order)

1. **Firefox under existing `browser_cache`** — same regenerable-cache-only allowlist policy and idle gates as Chrome/Edge; no separate category; Chromium forks (Brave, Opera, …) deferred.
2. **pnpm store and yarn cache** — developer-tool opt-in with exact roots, permanent-delete eligibility when proven, re-download impact notices; not Docker/Scoop/Chocolatey.
3. **AMD and Intel GPU/shader caches** — symmetric to `d3d_shader_cache` / `nvidia_dx_cache` after path and layout evidence; no vendor-merged mega-category.
4. **Visual Studio caches** — shipped as `visual-studio-caches` (ADR 0020): independent category, exact ComponentModelCache/Roslyn allowlist, devenv idle gate; no whole-ProgramData scan.
5. **`explorer_thumbnail_cache` / `inet_cache` exact allowlists** — refine discovery first; reassess permanent-delete eligibility only after Regenerable proof; keep Recycle Bin until then.
6. **Independent project-artifact purge flow** — **Shipped** as `foal purge` (explicit root(s), dry-run default, `--execute --allow-permanent`, v1 allowlist aligned with analyze; Protection and dangerous-root rejection). Never default Clean disk-wide scan; not a Clean catalog default/opt-in row.

## Do not

| Gap | Decision | Why |
| --- | --- | --- |
| Automatic or optional UAC elevation | Never elevate for Clean | Keeps skip-on-permission and permission-boundary notices; admin-only work stays out of executable Clean |
| Empty Recycle Bin | Permanently excluded | Foal's safety net is move-to-Recycle-Bin; emptying undercuts it (extends ADR 0006) |
| Downloads / Desktop / Documents / Pictures | Never Clean targets | User-authored content; age ≠ disposable |
| Browser cookies, history, credentials, sessions, download lists | Never | Not regenerable cleanup artifacts; privacy product, not Foal |
| Consumer/office app cache catalogs (Office, Teams, Slack, Discord, Steam, …) | No catalog | Avoid CCleaner-style app sprawl; weak layout proof |
| Registry cleaning, Prefetch wipe, secure erase / free-space wipe | Never | Low benefit / performance harm / different product promise than ordinary deletion |
| Windows.old, Windows Update Cleanup, Delivery Optimization, system `%WINDIR%\Temp` as executable Clean | Not executable | Require elevation; surface only as existing permission-boundary notices, not fake opportunities with byte totals |
| Default or Clean-matrix deep scan of `node_modules` / `target` / … | Not via ordinary Clean | Use shipped `foal purge` with explicit root(s); not a Clean catalog row |

## Do not — 2026-07 additions (single-machine disk-audit follow-up)

| Gap | Decision | Why |
| --- | --- | --- |
| JianyingPro (CapCut CN) `User Data\Cache\bytebench` | No category | Single-machine observation, no vendor cache contract; nested in a mixed-state cache root that may hold project-linked media proxies (cache-like name ≠ disposability) |
| DevEco Studio `tmp\patch-update` update jars | No category | IntelliJ-platform fork outside the `jetbrains-ide-caches` catalog (same exclusion class as Android Studio); no updater naming contract comparable to Grok residue (ADR 0022) |
| DySDKController `sdklog` / dy_service `p2pLog` unbounded logs | No category | Logs are diagnostic state (never Regenerable), closed-source writers with no rotation contract; log bloat is `foal analyze` insight, not Clean's job |
| GoogleUpdater `crx_cache` | No category | Lives under `Program Files (x86)` (pathsafe-rejected tree) and is the differential-update baseline — an active working set the updater GCs itself, not dead cache |

Machine-wide `C:\ProgramData` caches are no longer a blanket exclusion: see ADR 0030 for the Machine-wide cache category controls and its first two categories.

## Considered options

- **Allow elevation so Foal can match Disk Cleanup system volume** — rejected: new execution model, UAC, and test surface outweigh category work; system tools already cover those paths.
- **Opt-in empty Recycle Bin** — rejected: same-run or follow-up emptying races with Foal's recovery path and confuses mixed-action semantics.
- **Storage Sense-style Downloads age cleanup** — rejected: false-positive cost dominates reclaimable bytes.
- **Browser privacy opt-in category** — rejected: redefines Foal as a privacy cleaner.
- **Broad Applications list (Office, Steam, …)** — rejected: inventory race without per-app regenerable proof.
- **Project artifacts as Clean `--opt-in` category** — rejected in favor of an independent purge-style flow with an explicit user root.

## Consequences

- Market "GB freed" comparisons against Disk Cleanup for update leftovers remain intentionally weak; product copy should not claim parity with system temporary-files cleanup.
- Contributor temptation to add Recycle Bin empty, Windows.old, or registry clean should cite this ADR and ADR 0006/0018 rather than reopen by default.
- Firefox, pnpm/yarn, GPU vendors, Visual Studio, and thumbnail/inet refinement each still need their own evidence, tests, and (where surprising) focused ADRs or plan notes before registration where not already shipped.
- `docs/plan/project-artifact-clues.md` read-only analyze/Clean-pointer slice remains valid; project-artifact deletion is the shipped independent `foal purge` command, not an expansion of default Clean discovery.
