# Extend the Application cache opportunity to non-editor applications

## Status

Accepted. The `Applications` report category, the `app-caches` selection group, and the first non-editor Electron application (Obsidian) shipped with issue #281.

## Context

ADR 0019 locked product scope for missing Clean categories and explicitly rejected a broad Applications catalog (Office, Teams, Slack, Discord, Steam, ...) plus any user-supplied or remote rule store, because neither carried per-application regenerable proof. An earlier `light-c` attempt at a broad application sweep retreated after it deleted Claude desktop session data, confirming that "looks like a cache" is not regenerable proof. ADR 0019's "Do not" table still governs: no consumer/office app cache catalog without proof, no CCleaner-style app sprawl, no rule store.

The Application cache opportunity seam (ADR 0018) already proves per-application exact regenerating cache roots for the VS Code-family editors and Trae: each application has its own policy, Roaming AppData folder, logical process identity, and a per-policy relative-root allowlist, all under the `Developer tools` report category and the `dev-caches` opt-in group. The seam already accepts a per-policy allowlist, so a non-editor Electron application that regenerates the same plain-Electron cache layout can be proven independently without a seam change.

Obsidian is a non-editor Electron note-taking application. Its `%APPDATA%\obsidian` user-data root holds the standard plain-Electron regenerating-cache layout (`Cache`, `Code Cache`, `GPUCache`, `DawnCache`, `DawnGraphiteCache`, `DawnWebGPUCache`) alongside state, configuration, and bundle artifacts (`obsidian.json`, the `app.asar` bundle, Local Storage, IndexedDB, Service Worker, Preferences). It is not a developer tool, so grouping it under `Developer tools` would mislead users.

## Decision

Extend the Application cache opportunity to non-editor applications under a new `Applications` report category and a new `app-caches` selection group, distinct from `Developer tools` / `dev-caches`. Obsidian is the first such application.

This is narrow per-application proof, not a broad sweep:

- Each non-editor application requires its own registered policy with its exact Roaming AppData folder, logical process identity (`obsidian` / `Obsidian.exe`), and a proven exact relative-root allowlist. Foal never scans a user-data tree by substring and never selects roots recursively.
- Obsidian carries its own plain-Electron allowlist of single-segment regenerating roots only: `Cache`, `Code Cache`, `GPUCache`, `DawnCache`, `DawnGraphiteCache`, `DawnWebGPUCache`. It excludes `CachedData` and `CachedExtensionVSIXs`, because a non-editor note-taking application has no V8 code-cache or VSIX re-download proof, and it excludes every state/config/bundle artifact: `obsidian.json`, the `*.asar` bundle, Local Storage, IndexedDB, Service Worker, Preferences, User, and logs. Multi-segment roots (for example `Service Worker\CacheStorage`) are deferred, not approximated.
- Each application uses Permanent deletion (ADR 0018) with `application-idle-before-and-after-inspection` gating: the owning application must be idle before and after inspection. Pre-inspection running, unknown, missing, or snapshot-failure state skips all roots for that application without measuring; post-inspection unsafe state discards every measured root and byte total for that application only. The idle gate is independent per application: a running or selected Obsidian never authorizes or suppresses an editor or Trae, and vice versa.
- Protection suppresses protected roots before totals and downstream projection without authorizing siblings; each allowlisted root is an independent Opportunity or Opt-in candidate.
- Execute revalidates fresh and permanently deletes with per-run `--allow-permanent` (CLI) or TUI confirmation; without authorization, candidates are skipped with `permanent_deletion_not_authorized` and never fall back to the Recycle Bin.
- The `Applications` report category renders these opportunities separately from `Developer tools` in the preview, after Developer tools. The `app-caches` group token expands Applications report-category application cache categories only (currently `obsidian_cache`), in catalog order; it owns no resolver, candidates, or deletion action. `dev-caches` continues to expand developer-cache plus editor Application cache categories under Developer tools and must not include Applications categories. Exact-name `obsidian_cache`, `app-caches`, `all`, and Clean TUI selection all work; `cli-agents` excludes both Obsidian and editor caches.

This is explicitly not the broad UWP `Packages\*\LocalCache` scan and not a user-supplied or remote rule store, both of which ADR 0019 rejected. The seam already supported per-policy allowlists, so no seam change was needed: Obsidian reuses the existing discovery, gating, Protection, incomplete-sibling, cancellation, and fresh-revalidation behavior with its own allowlist and process identity.

Out of scope: broad UWP or Local AppData application scans, a user-supplied or remote rule store, multi-segment roots (Service Worker\CacheStorage deferred), process stopping, automatic elevation, and any application whose regenerating roots are not independently proven.

## Consequences

- Non-editor Electron applications can reclaim proven regenerating caches without being mislabeled as developer tools and without broadening into a CCleaner-style app catalog.
- Each non-editor application still needs its own evidence, exact allowlist, process identity, and idle gate before registration; the `Applications` report category and `app-caches` group do not relax the per-application proof bar of ADR 0018 and ADR 0019.
- The `Applications` report category and `app-caches` group are the extension point for future non-editor applications (for example other note-taking or messaging Electron apps) once their exact regenerating roots are proven; adding one does not imply adding any other.
- `dev-caches` and `app-caches` are disjoint by report category, so opting into non-editor application caches never silently opts into editor or developer-tool caches, and vice versa.
- This decision does not reopen ADR 0019's rejection of consumer/office app cache catalogs, a rule store, UWP scanning, or any application without independent regenerable proof.
