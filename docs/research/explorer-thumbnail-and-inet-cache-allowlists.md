# Exact allowlists: `explorer_thumbnail_cache` and `inet_cache`

Status: researched 2026-07-17; allowlist discovery implemented in [#239](https://github.com/CoreyLyn/Foal/issues/239).
This note remains the evidence source for paths and Planned-action rationale
from [#235](https://github.com/CoreyLyn/Foal/issues/235) / program
[#233](https://github.com/CoreyLyn/Foal/issues/233). Planned action stays
`move_to_recycle_bin` until a separate permanent-eligibility decision.

Parent product decision: [ADR 0019](../adr/0019-cleanup-category-gap-decisions.md)
item 5 — refine discovery first; reassess permanent-delete eligibility only
after Regenerable proof; keep Recycle Bin until then.

Policy matrix already flags both categories as over-broad whole roots:
[`docs/plan/clean-deletion-policy.md`](../plan/clean-deletion-policy.md).

## Dated conclusions

| Category | Discovery shape after refine | Planned action recommendation | Permanent-delete eligibility | Confidence |
| --- | --- | --- | --- | --- |
| **`explorer_thumbnail_cache`** | Exact files under Explorer root matching `thumbcache_*.db` and `iconcache_*.db` only | **`move_to_recycle_bin`** (keep) | Not proven for Foal permanent matrix yet | High for paths and regenerability class; permanent deferred (Explorer always-on locking) |
| **`inet_cache`** | Exact real directories `INetCache\IE` and `INetCache\Low\IE` only | **`move_to_recycle_bin`** (keep) | Not proven for Foal permanent matrix yet | High for WinINET TIF layout; permanent deferred (mixed parent tree + active WinINET use) |

Neither category should flip to `delete_permanently` in the implement ticket
that only applies this allowlist. A later eligibility decision may revisit
permanent after file/dir-level candidates ship and locking behavior is tested.

## Fail-closed rule (mandatory for implement)

1. Resolve only the exact allowlisted entries documented below.
2. If **no** allowlisted file or directory exists for a category, the category
   is **empty** (silent absence / zero candidates).
3. **Never** fall back to measuring or deleting the whole
   `%LOCALAPPDATA%\Microsoft\Windows\Explorer` or
   `%LOCALAPPDATA%\Microsoft\Windows\INetCache` root when allowlisted children
   are missing, empty, unreadable, or filtered out by Protection / reparse
   rules.
4. Parent roots of allowlisted entries are **never** candidates.
5. Non-allowlisted siblings under those parents are **never** candidates.

This matches ADR 0019 user story D.34 and the issue acceptance criteria.

## Prior art in Foal (current behavior — over-broad)

Catalog registration today (`internal/clean/category_catalog.go`):

```text
existenceOpportunityEntry(..., "Microsoft", "Windows", "Explorer")   // explorer_thumbnail_cache
existenceOpportunityEntry(..., "Microsoft", "Windows", "INetCache") // inet_cache
```

Both are:

- Opt-in existence opportunities (not idle-age).
- Whole configured LocalAppData relative root as **one** candidate path.
- Planned action: `move_to_recycle_bin`.
- TUI: Recycle Bin opt-in rows, initially unselected.

ADR 0006 introduced them as fixed known roots observed whole. This research
replaces that whole-root model **in design only**.

---

## Regenerable proof standard used here

Per `docs/plan/clean-deletion-policy.md`: permanent eligibility requires that
**all surviving content under the exact candidate root** is regenerable or
re-downloadable, and excludes user-authored, diagnostic, configuration,
history, and login state.

Proof classes accepted:

1. **Microsoft first-party product surface** that lists the artifact as a Disk
   Cleanup / Storage temporary category (VolumeCaches handlers) and describes
   recreation after delete.
2. **Microsoft API / shell documentation** for the cache system (e.g.
   `IThumbnailCache`).
3. **Controlled layout evidence** on a research host: fixed names, opaque cache
   blobs, and explicit non-cache siblings under the same parent.
4. **Cross-tool / support-procedure corroboration** (MS Q&A rebuild recipes,
   SuperUser/Disk Cleanup “View Files” path) as path inventory, not product
   authority for Foal permanent deletion.

Age thresholds and folder-name substrings alone are **insufficient**.

---

## `explorer_thumbnail_cache`

### Current whole-root over-includes (must stop being a candidate)

Whole candidate today:

```text
%LOCALAPPDATA%\Microsoft\Windows\Explorer
```

On research host 2026-07-17 this directory mixed:

| Name pattern / file | Role | Allowlist? |
| --- | --- | --- |
| `thumbcache_*.db` (15 files) | Centralized Explorer thumbnail databases | **Yes** |
| `iconcache_*.db` (15 files) | Explorer icon databases (Win8+ layout) | **Yes** |
| `ExplorerStartupLog.etl` | Explorer startup ETW diagnostic log | **No** |
| `ExplorerStartupLog_RunOnce.etl` | RunOnce startup ETW log | **No** |
| `RecommendationsFilterList.json` | Explorer recommendations filter/state JSON | **No** |

Host measurement (approx.):

- Allowlisted `thumbcache_*.db` + `iconcache_*.db`: **~156 MB**
- Non-allowlisted siblings: **~664 KB** (logs + JSON)

Whole-root therefore over-includes **diagnostic logs and Explorer recommendation
state**, which are not proven thumbnail/icon cache content and must not be
candidates once discovery is refined.

### Exact allowlist entries

Parent directory (never a candidate):

```text
%LOCALAPPDATA%\Microsoft\Windows\Explorer
```

**Ship as exact file candidates** (each existing matching regular file is an
independent candidate, or implement may aggregate bytes under a synthetic
category measure — product choice — but only these names may contribute):

| Relative path pattern | Role | Evidence |
| --- | --- | --- |
| `Microsoft\Windows\Explorer\thumbcache_*.db` | Centralized thumbnail cache DBs (size buckets + index) | Vista+ centralized cache location; Wikipedia; Microsoft shell `IThumbnailCache`; Disk Cleanup **Thumbnail Cache** VolumeCaches entry; MS Q&A rebuild recipes |
| `Microsoft\Windows\Explorer\iconcache_*.db` | Icon cache DBs co-located with thumbnails | MS Learn Q&A standard rebuild pairs `iconcache_*.db` with `thumbcache_*.db` in this same directory |

Canonical observed basenames on research host (illustrative, not closed set —
match by **prefix + `.db`**, not a frozen size list):

```text
thumbcache_16.db, thumbcache_32.db, thumbcache_48.db, thumbcache_96.db,
thumbcache_256.db, thumbcache_768.db, thumbcache_1280.db, thumbcache_1920.db,
thumbcache_2560.db, thumbcache_wide.db, thumbcache_wide_alternate.db,
thumbcache_custom_stream.db, thumbcache_exif.db, thumbcache_idx.db,
thumbcache_sr.db

iconcache_16.db, iconcache_32.db, iconcache_48.db, iconcache_96.db,
iconcache_256.db, iconcache_768.db, iconcache_1280.db, iconcache_1920.db,
iconcache_2560.db, iconcache_wide.db, iconcache_wide_alternate.db,
iconcache_custom_stream.db, iconcache_exif.db, iconcache_idx.db,
iconcache_sr.db
```

Implement rules:

1. Parent segments must be exactly `Microsoft\Windows\Explorer` under current
   user Local AppData.
2. Basename must match `thumbcache_*.db` or `iconcache_*.db` case-insensitively
   (Windows FS is case-insensitive).
3. Only regular files; reject reparse points / directories with those names if
   ever present.
4. Do **not** recurse into subdirectories under Explorer for this category
   (none observed; future nested junk must not expand scope).
5. Missing all matches ⇒ empty category (fail-closed).

### Explicit non-targets (must never become candidates)

| Path / pattern | Why excluded |
| --- | --- |
| `%LOCALAPPDATA%\Microsoft\Windows\Explorer` (whole root) | Mixed cache + logs + recommendation state |
| `ExplorerStartupLog*.etl` | Diagnostic ETW, not thumbnail cache |
| `RecommendationsFilterList.json` | Filter/state config, not regenerable preview blobs |
| `%LOCALAPPDATA%\IconCache.db` | Legacy single-file icon cache **outside** Explorer; not under this category root. Optional future research only; do not invent into this category without a new decision |
| Per-folder `Thumbs.db` / `ehthumbs.db` anywhere on disk | Legacy/local-folder caches; not the centralized Explorer root; disk-wide search is out of scope |
| Network-share `Thumbs.db` | Same; Group Policy territory, not Clean fixed-root |
| `%LOCALAPPDATA%\Microsoft\Windows\INetCache\thumbnails` | Separate secondary thumbnail location (not Explorer DBs); not this category |

### Regenerable rationale (thumbnail / icon)

1. **Microsoft Disk Cleanup category:** Registry
   `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\VolumeCaches\Thumbnail Cache`
   (CLSID `{889900c3-59f3-4c2f-ae21-a409ea01e605}`) is a first-party Disk
   Cleanup handler. Microsoft’s free-up-space guidance lists **Thumbnails**
   among default Disk Cleanup selections and describes them as previews Windows
   can rebuild.
2. **Shell API:** Microsoft documents
   [`IThumbnailCache`](https://learn.microsoft.com/en-us/windows/win32/api/thumbcache/nn-thumbcache-ithumbnailcache)
   as a system thumbnail cache shared across applications (Vista+), replacing
   per-folder `Thumbs.db` locality.
3. **Centralized path documentation:** Public technical summaries (Wikipedia
   *Windows thumbnail cache*) place `thumbcache_*.db` under
   `%userprofile%\AppData\Local\Microsoft\Windows\Explorer`.
4. **Rebuild procedure (path inventory):** Microsoft Q&A and support recipes
   delete `thumbcache_*.db` and `iconcache_*.db` under that Explorer directory
   (often after restarting Explorer) so Windows recreates them.

### Why Planned action stays Recycle Bin (not permanent yet)

Regenerability of **allowlisted** `.db` files is strong in the OS sense.
Permanent is still **not** recommended for the first implement slice because:

- `explorer.exe` is almost always running and commonly holds locks on these
  DBs; Foal must not stop processes. Permanent delete of locked files yields
  noisy failures without reclaim; Recycle Bin move has the same lock risk but
  matches the current matrix and recovery story.
- ADR 0019 explicitly keeps Recycle Bin until a separate permanent-eligibility
  reassessment after refine.
- Non-allowlisted siblings under the same parent prove the **current** whole
  root is not permanent-safe; permanent may be reconsidered **only after**
  file-level allowlist is live and tests prove non-allowlisted siblings never
  delete.

### Idle / locking notes

- No process stop. Prefer `RunningApplicationPolicyNotApplicable` (status quo)
  with local skip/fail on sharing violations, same spirit as GPU caches.
- Do not add “kill explorer.exe” guidance or automation.

### Fixture sketch (implement later)

```text
{tempLocalAppData}\Microsoft\Windows\Explorer\thumbcache_256.db     # candidate
{tempLocalAppData}\Microsoft\Windows\Explorer\iconcache_32.db       # candidate
{tempLocalAppData}\Microsoft\Windows\Explorer\ExplorerStartupLog.etl # EXCLUDED
{tempLocalAppData}\Microsoft\Windows\Explorer\RecommendationsFilterList.json # EXCLUDED
{tempLocalAppData}\IconCache.db                                     # OUT OF CATEGORY
```

---

## `inet_cache`

### Current whole-root over-includes (must stop being a candidate)

Whole candidate today:

```text
%LOCALAPPDATA%\Microsoft\Windows\INetCache
```

Controlled listing on research host 2026-07-17:

| Child | Kind | Role | Allowlist? |
| --- | --- | --- | --- |
| `IE\` | Directory (Hidden+System) | WinINET / Temporary Internet Files cache body (hash subdirs + blobs) | **Yes** (exact dir) |
| `Content.IE5` | **Junction** → `IE\` | Legacy name for Temporary Internet Files | **No** as separate candidate (reparse / double-count) |
| `Low\` | Directory | Low-integrity subtree | Parent only — not whole |
| `Low\IE\` | Directory | Low-integrity WinINET cache | **Yes** (exact dir) |
| `Low\Content.IE5` | **Junction** → `Low\IE\` | Legacy low-integrity TIF name | **No** as separate candidate |
| `Low\SuggestedSites.dat` | File (~5 MB) | IE Suggested Sites data | **No** |
| `Virtualized\` | Directory (empty on host) | Virtualized / package-related surface | **No** |
| `Content.MSO` | Directory when Office used | Office attachment / document temp cache | **No** (not observed on host; still excluded) |
| `Content.Word` / `Content.Outlook` / similar | Directories when present | Office family temp caches | **No** |
| `thumbnails\` | Directory when present | App/web thumbnail files distinct from Explorer DBs | **No** for this category |

Also out of category entirely (not under INetCache root, but often confused):

| Path | Why |
| --- | --- |
| `%LOCALAPPDATA%\Microsoft\Windows\WebCache\` | ESE WebCache DB (`WebCacheV01.dat` etc.); cookies/history-adjacent WinINET store — **not** Temporary Internet Files tree |
| `%LOCALAPPDATA%\Microsoft\Windows\INetCookies\` | Cookies, not cache |
| System profile copies under `%WINDIR%\System32\config\systemprofile\...\INetCache` | Not current interactive user; elevation / service profile |

Whole-root therefore over-includes **junction aliases**, **Suggested Sites data**,
**Virtualized**, and (when present) **Office Content.\* trees** that can hold
temporary copies of attachments/documents. Those are not proven pure disposable
internet cache and must stop being implicit candidates.

### Exact allowlist entries

Parent directory (never a candidate):

```text
%LOCALAPPDATA%\Microsoft\Windows\INetCache
```

**Ship as exact child directory candidates** (each existing **real directory**
is one candidate root; measure/delete only that directory tree):

| Relative path under `%LOCALAPPDATA%` | Role | Evidence |
| --- | --- | --- |
| `Microsoft\Windows\INetCache\IE` | Primary Temporary Internet Files / WinINET cache | Disk Cleanup **Internet Cache Files** (“Temporary Internet Files”) VolumeCaches CLSID `{9B0EFD60-F7B0-11D0-BAEF-00C04FC308C9}`; Disk Cleanup “View Files” navigates to `...\INetCache\IE`; MS Support Temporary Internet Files cleanup guidance |
| `Microsoft\Windows\INetCache\Low\IE` | Low-integrity WinINET cache | Same layout class under `Low\`; parallel to `IE` on research host |

Implement rules:

1. Candidates are **only** the two real directory paths above when they exist
   as directories (not when only a junction name exists without the target).
2. Do **not** register `Content.IE5` or `Low\Content.IE5` as candidates. They
   are junctions to `IE` / `Low\IE` on modern Windows (verified with
   `fsutil reparsepoint query` / PowerShell `LinkType=Junction` on research
   host). Following them double-counts and trips reparse-sensitive inspection.
3. Do **not** walk or measure the parent `INetCache` root.
4. Optional content policy inside an allowlisted root (implement choice, fail
   closed on uncertainty):
   - Prefer whole allowlisted directory as one existence candidate (simplest;
     matches current existence opportunity style once root is narrowed).
   - If implement later excludes metadata files, `container.dat` under `IE` is
     empty metadata on the research host; SuperUser system-profile cleanup notes
     sometimes skip it. Whole-dir delete of `IE` remains acceptable for Recycle
     Bin because the directory is the TIF cache container Disk Cleanup targets.
5. Missing both `IE` and `Low\IE` ⇒ empty category (fail-closed). Presence of
   only `SuggestedSites.dat` or only junctions must **not** create a candidate.

### Explicit non-targets (must never become candidates)

| Path / pattern | Why excluded |
| --- | --- |
| `%LOCALAPPDATA%\Microsoft\Windows\INetCache` (whole root) | Mixed WinINET cache + Office temps + Suggested Sites + Virtualized + junctions |
| `Content.IE5`, `Low\Content.IE5` | Junction aliases to allowlisted dirs; reparse |
| `Low\` (whole) | Contains `SuggestedSites.dat` and junctions; only `Low\IE` is allowlisted |
| `Low\SuggestedSites.dat` | Suggested Sites feature data, not TIF blob cache |
| `Virtualized\` | Not proven disposable WinINET content |
| `Content.MSO`, `Content.Word`, `Content.Outlook`, `Content.WordCache`, … | Office temporary document/attachment caches; Microsoft Q&A notes unsaved-attachment risk if user had not saved elsewhere |
| `thumbnails\` under INetCache | Not the Explorer thumbcache DB set; not Disk Cleanup TIF primary path |
| `WebCache`, `INetCookies` | Sibling Windows trees; cookies/history class excluded by ADR 0019 |
| System / service profile INetCache trees | Not current user; no elevation |

### Regenerable rationale (allowlisted IE trees only)

1. **Microsoft Disk Cleanup:** VolumeCaches entry **Internet Cache Files**
   displays as **Temporary Internet Files** with description that the folder
   contains web pages stored for quick viewing and that personalized web
   settings remain intact — classic disposable cache framing. Priority 100
   first-party handler.
2. **Path mapping:** Disk Cleanup “View Files” and multiple Microsoft Q&A
   threads resolve Temporary Internet Files to
   `%LOCALAPPDATA%\Microsoft\Windows\INetCache\IE`.
3. **MS Support:** [How to delete the contents of the Temporary Internet Files
   folder](https://support.microsoft.com/en-us/topic/how-to-delete-the-contents-of-the-temporary-internet-files-folder-8eb83a8d-43e2-300d-d355-2ee71602ab44)
   documents intentional user deletion of TIF content.
4. **Layout on research host:** Under `IE\`, content is hash-named subfolders
   of opaque cached resources (xml/json/png/cache blobs) plus `container.dat` —
   consistent with a download cache container, not user libraries.

### Why Planned action stays Recycle Bin (not permanent yet)

- Exact `IE` / `Low\IE` content is **closer** to Regenerable than the whole
  INetCache root, but the parent tree’s Office `Content.*` history shows why
  whole-root permanent was correctly rejected.
- WinINET is used by system and app components while the user is logged on;
  locked files and partial trees are expected. No process stop.
- ADR 0019: keep Recycle Bin until a separate permanent-eligibility decision
  with tests. This research **does not** claim permanent proof for the Foal
  matrix.
- `Content.MSO`-class paths remain excluded; permanent promotion must not
  expand to them under the `inet_cache` id.

### Idle / locking notes

- No process stop; no Internet Options UI automation.
- In-use files under `IE` should skip/fail locally.
- Do not clear cookies, credentials, or `WebCache` as part of this category.

### Fixture sketch (implement later)

```text
{tempLocalAppData}\Microsoft\Windows\INetCache\IE\ABCD1234\file[1].dat  # under allowlisted root
{tempLocalAppData}\Microsoft\Windows\INetCache\IE\container.dat         # under allowlisted root
{tempLocalAppData}\Microsoft\Windows\INetCache\Low\IE\...               # second allowlisted root
{tempLocalAppData}\Microsoft\Windows\INetCache\Content.IE5 -> IE        # EXCLUDED junction
{tempLocalAppData}\Microsoft\Windows\INetCache\Low\SuggestedSites.dat   # EXCLUDED
{tempLocalAppData}\Microsoft\Windows\INetCache\Content.MSO\doc.xlsx     # EXCLUDED
{tempLocalAppData}\Microsoft\Windows\INetCache\Virtualized\...          # EXCLUDED
```

Resolver must:

1. Join Local AppData + `Microsoft\Windows\INetCache`.
2. Stat only `IE` and `Low\IE` as candidate roots when they are real directories.
3. Never promote parent `INetCache` or non-allowlisted siblings.
4. Never fall back to whole-root if both children are missing.

---

## Planned action recommendation (summary)

| Category | Recommend now | Permanent later only if |
| --- | --- | --- |
| `explorer_thumbnail_cache` | `move_to_recycle_bin` | File-level allowlist shipped; exclusive-lock behavior tested; deletion-policy matrix + tests updated; still no process kill |
| `inet_cache` | `move_to_recycle_bin` | Only `IE` + `Low\IE` candidates shipped; Office/SuggestedSites/WebCache still excluded; matrix + tests updated |

This ticket / research note does **not** flip either action.

## Implement ticket boundary (out of this research)

Do **not** ship in a research-only change:

- Discovery code changes
- Catalog `fixedLocalAppDataPath` changes
- Planned action changes
- TUI selection policy changes

A future implement PR should:

1. Replace whole-root existence entries with allowlisted file/dir discovery.
2. Add tests that non-allowlisted siblings never become candidates.
3. Add tests that missing allowlist ⇒ empty (no whole-root fallback).
4. Keep `move_to_recycle_bin` unless a separate permanent-eligibility decision
   updates `docs/plan/clean-deletion-policy.md`.

## Machine evidence appendix (research host 2026-07-17)

- OS: Windows 11 family (user profile `C:\Users\corey`).
- Explorer root present with 15 `thumbcache_*.db` + 15 `iconcache_*.db` plus
  ETL logs and `RecommendationsFilterList.json`.
- `%LOCALAPPDATA%\IconCache.db` present (~216 KB) **outside** Explorer.
- INetCache: real `IE` with hash subdirs; `Content.IE5` junction → `IE`;
  `Low\IE` present (empty); `Low\SuggestedSites.dat` ~5 MB;
  `Content.MSO` / `thumbnails` absent; `WebCache` is a sibling under
  `Microsoft\Windows`, not under INetCache.
- VolumeCaches handlers present: **Thumbnail Cache**, **Internet Cache Files**.

## Primary sources

- Microsoft Learn: [IThumbnailCache](https://learn.microsoft.com/en-us/windows/win32/api/thumbcache/nn-thumbcache-ithumbnailcache)
- Microsoft Support: [Free up drive space in Windows](https://support.microsoft.com/en-us/windows/free-up-drive-space-in-windows-85529ccb-c365-490d-b548-831022bc9b32) (Disk Cleanup Thumbnails / Temporary Internet Files)
- Microsoft Support: [Delete Temporary Internet Files folder contents](https://support.microsoft.com/en-us/topic/how-to-delete-the-contents-of-the-temporary-internet-files-folder-8eb83a8d-43e2-300d-d355-2ee71602ab44)
- Host registry: `HKLM\...\Explorer\VolumeCaches\Thumbnail Cache`, `...\Internet Cache Files`
- Host filesystem layout under `%LOCALAPPDATA%\Microsoft\Windows\Explorer` and `...\INetCache`
- Wikipedia: [Windows thumbnail cache](https://en.wikipedia.org/wiki/Windows_thumbnail_cache) (centralized path inventory)
- Foal: ADR 0006, ADR 0018, ADR 0019; `docs/plan/clean-deletion-policy.md`
)
