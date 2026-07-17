# Structured downloadable developer-cache artifacts use catalog-owned child discovery

> **Status note (ADR 0018):** Structured categories such as `playwright-browsers` and `puppeteer-browsers` (and product-scoped `jetbrains-ide-caches` via ADR 0017, `visual-studio-caches` via ADR 0020) use catalog planned action `delete_permanently`. Discovery, fail-closed layout policy, and shared opt-in resolution in this ADR remain in force. Aggregate Recycle Bin capacity pre-checks apply only when a category's planned action is `move_to_recycle_bin`.

Foal already reclaims whole-root developer-tool caches through explicit opt-in, env/default root resolution, Protection, fresh Dry-run/Execute resolution, immediate validation, and action-specific preflight including aggregate Recycle Bin capacity checks for Recycle Bin work (ADR 0008, qualified by ADR 0018). Some developer caches are not safe to treat as a single root: they mix re-downloadable installations with profiles, metadata, product parents, or other state that must never be authorized by proximity under the same tree.

This decision deepens the private canonical developer-cache registration seam with an **optional structured child candidate discovery policy**. Categories without the policy keep today's whole-root behavior: each resolved root is one Opt-in candidate. Categories that register a policy never turn the root into a candidate. Shared Clean opt-in resolution fresh-resolves roots, asks the policy for child paths under each unprotected root, then applies shared safety: Windows path normalization, deduplication, strict-root containment, directory-only acceptance, reparse/symlink rejection, per-child Protection, Opportunity inspection ceiling measurement, and independent sibling outcomes.

Public catalog projections stay path-free. Resolvers, environment names, default roots, product/component allowlists, structural matchers, and installation-evidence rules remain private Clean policy. Dry-run and Execute use the same seam; Execute never trusts Dry-run paths. Default Execute without the category selected performs no root resolution and no child discovery for that category. Surviving children reuse existing immediate validation, the category's catalog planned action (including permanent authorization when required), aggregate per-volume Recycle Bin capacity pre-checks only for Recycle Bin work, and normal opt-in result/history contracts.

**Fail-closed layout policy:** structured downloadable developer-cache artifacts are eligible only when a Foal-owned policy can prove the path is a re-downloadable installation shape under a resolved root. Unknown prefixes, unknown products, metadata directories, profile/state trees, incomplete installations, regular files, links/junctions/reparse points, the root itself, and any path outside the root are excluded by construction. An upstream layout change must produce no candidates until the private policy and tests are deliberately updated. Protection removes candidates only and never authorizes siblings or expands allowlists. Root resolution alone never authorizes deletion.

This decision established the reusable seam and vocabulary. Concrete structured categories include `playwright-browsers` (registers `resolvePlaywrightBrowserPaths` and `discoverPlaywrightBrowserChildren` through `developerCacheEntryWithChildren`) and `puppeteer-browsers` (registers `resolvePuppeteerCachePaths` and `discoverPuppeteerBrowserChildren` via the same helper, exposing independent Windows platform-version installations only and keeping the root and product parents non-candidates). Further frameworks still register their own resolver and child policy under the same seam; they must not invent a parallel cleanup engine.

## Considered options

- **Keep only whole-root developer-cache candidates** - rejected: roots that mix disposable binaries with user state cannot be reclaimed without either deleting state or remaining invisible.
- **Hard-code framework-specific scanners outside the catalog** - rejected: would fork discovery, Protection, and Execute paths and drift from shared opt-in contracts.
- **Treat any version-looking directory under a tool root as disposable** - rejected: fails open on unknown layouts and risks profiles, metadata, and future state.
- **Trust Dry-run discovered children at Execute** - rejected: violates the fresh-scan invariant; installs may appear or disappear between preview and confirmation.

## Consequences

- Private `categoryCatalogEntry` may bind `discoverChildren` for developer-cache categories; existing categories remain whole-root when the field is nil.
- Shared opt-in resolution becomes the only external execution seam for structured children.
- Tests inject structured discovery under developer-cache resolution so Windows trees can be exercised without real user caches or premature public categories.
- `CONTEXT.md` holds structured downloadable developer-cache vocabulary; `playwright-browsers` and `puppeteer-browsers` add their identifiers, impact notices, and docs under this decision without reopening the architecture. Further framework slices follow the same pattern.
- Default candidate freeze, catalog-owned planned actions (ADR 0018), no automatic elevation, and no third-party cleanup command execution remain unchanged.
