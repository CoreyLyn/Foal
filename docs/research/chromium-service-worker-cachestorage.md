# Chromium Service Worker `CacheStorage` eligibility for `browser_cache`

Status: researched 2026-07-17; evidence for
[#257](https://github.com/CoreyLyn/Foal/issues/257). This note does **not**
implement catalog changes, path matchers, or deletion behavior.

Parent product constraints: [ADR 0007](../adr/0007-clean-browser-cache-discovery-requires-running-application-detection.md)
(browser-idle-before-and-after, regenerating dirs only, complete inspection or
discard); [`docs/plan/clean-deletion-policy.md`](../plan/clean-deletion-policy.md)
(`browser_cache` already `delete_permanently` for allowlisted roots only).

## Dated conclusions

| Scope | Decision | Planned action if shipped | Confidence |
| --- | --- | --- | --- |
| **Chrome / Edge** profile-relative `Service Worker\CacheStorage` only | **Ship allowlist extension** under existing `browser_cache` | `delete_permanently` (same category action; CLI still needs per-run `--allow-permanent`) | **High** for path layout + content class (Chromium first-party READMEs + W3C Cache objects); **Medium** for residual site-chosen Response body risk (same class as HTTP `Cache`, disclosed) |
| Whole `Service Worker` parent | **Do not add** | n/a | High (mixed: Database + ScriptCache + CacheStorage) |
| `Service Worker\ScriptCache` | **Do not add** (this ticket) | n/a | High as non–Cache-API install artifact; separate research if ever revisited |
| `Service Worker\Database` | **Do not add** | n/a | High (registration metadata / state) |
| Firefox Cache API path | **Out of scope** for this Chrome/Edge ship decision | n/a | Medium (mixed origin storage; see § Firefox) |

**Go for follow-up `feat(clean)` ticket:** yes — extend Chromium
`cacheDirectoryKinds` only; keep idle policy; strengthen impact notice for PWA
offline.

Not recommended: Recycle Bin only (regenerability matches existing permanent
`browser_cache` roots) or do-not-add (primary layout + regenerability proof is
stronger than several already-shipped permanent roots).

---

## Prior art in Foal (current `browser_cache` allowlist)

Source: [`internal/clean/browser_cache.go`](../../internal/clean/browser_cache.go).

| Browser | Profile catalog | Profile-relative allowlisted kinds | Planned action |
| --- | --- | --- | --- |
| Google Chrome | `Local State` under `%LOCALAPPDATA%\Google\Chrome\User Data` | `Cache`, `Code Cache`, `GPUCache` | `delete_permanently` |
| Microsoft Edge | `Local State` under `%LOCALAPPDATA%\Microsoft\Edge\User Data` | `Cache`, `Code Cache`, `GPUCache` | `delete_permanently` |
| Mozilla Firefox | `profiles.ini` (Roaming) → Local profile | `cache2` only | `delete_permanently` |

Discovery rules already required by ADR 0007 / production code:

- Running-application detection: whole browser running or unknown ⇒ discard.
- Idle before **and** after complete inspection of every recognized cache dir.
- Incomplete inspection of any existing allowlisted dir ⇒ discard (fail-closed).
- Protection on User Data root or any cache path ⇒ suppress.
- Per-profile candidates; never whole `User Data` or whole profile root.
- Tests already treat bare `Service Worker` content as **non-candidate**
  ([`internal/clean/preview_test.go`](../../internal/clean/preview_test.go)).

Policy matrix row today (`clean-deletion-policy.md`):

> Only allowlisted Chrome/Edge (`Cache`/`Code Cache`/`GPUCache`) and Firefox
> (`cache2`) profile cache roots; each browser must be idle before and after
> complete inspection.

Local ROI observation (issue #257, non-authoritative for eligibility): one host
reported Edge `...\Default\Service Worker\CacheStorage` ≈ 2.6 GB vs allowlisted
caches ≈ 0.5 GB.

---

## Regenerable proof standard used here

Per `docs/plan/clean-deletion-policy.md`: permanent eligibility requires that
**all surviving content under the exact candidate root** is regenerable or
re-downloadable, and excludes user-authored, diagnostic, configuration, history,
and login state.

Proof classes accepted (match other Foal research notes):

1. **First-party product/spec docs** — W3C Service Workers (Cache objects as
   request/response store); Chromium `content/browser` READMEs; Chrome DevTools
   / web.dev Cache Storage guidance.
2. **Chromium source / architecture docs** defining on-disk layout and what
   each sibling under `Service Worker` holds.
3. **Controlled layout evidence** — documented directory tree
   (`CacheStorage/<origin-hash>/<cache-guid>/` with `disk_cache` backends).
4. **Cross-tool path inventory** (Winapp2, SuperUser paths) only as corroboration,
   **not** product authority for permanent deletion.

Age thresholds and folder-name substrings alone are **insufficient**.

---

## Layout evidence (exact relative paths)

### Chromium first-party storage map

Chromium’s service worker README documents three distinct stores under the
profile-relative `Service Worker` directory
([`content/browser/service_worker/README.md`](https://chromium.googlesource.com/chromium/src/+/lkgr/content/browser/service_worker/README.md)):

| Relative path under profile (`$PROFILE` / `DIR_USER_DATA` in Chromium docs) | Role | Pure regenerable cache for Foal? |
| --- | --- | --- |
| `Service Worker\Database` | LevelDB **registration metadata** | **No** — install/registration state |
| `Service Worker\ScriptCache` | `disk_cache` (simple) of **service worker scripts**; registration metadata points at these | **No** for this ship — install-related script store, not Cache API |
| `Service Worker\CacheStorage` | `disk_cache` (simple) for the **Cache Storage API** | **Yes** — Request/Response pairs |

Quoted distinction (Chromium):

> Service worker scripts are stored in a disk_cache instance using the “simple”
> implementation, located at `${DIR_USER_DATA}/Service Worker/ScriptCache`.
>
> The related Cache Storage API uses a disk_cache instance using the “simple”
> implementation, located at `${DIR_USER_DATA}/Service Worker/CacheStorage`.
> This location was chosen because the Cache Storage API is currently defined
> in the Service Worker specification, but it can be used independently of
> service workers.

`DIR_USER_DATA` / `$PROFILE` in these docs is the **browser profile directory**
(e.g. `...\User Data\Default`, `...\User Data\Profile 1`), not the parent
`User Data` root. Windows community and support paths consistently show the
same layout under Edge/Chrome profiles (path inventory only).

### CacheStorage on-disk shape (Chromium)

From
[`content/browser/cache_storage/README.md`](https://chromium.googlesource.com/chromium/src/+/lkgr/content/browser/cache_storage/README.md):

```text
$PROFILE/Service Worker/CacheStorage/<origin-hash>/<cache-guid>/
```

- `origin` directory name is a **hash of the origin**.
- `cache` directory is a **GUID** generated at cache creation time (dooms old
  cache while old handles drain).
- Content is a `disk_cache::Backend`: each entry is a **Request/Response** pair
  (metadata protobuf stream + response body stream + optional side stream).
- `CacheStorage` maintains an index mapping cache names to paths; directories not
  in the index are deleted on init.

**Ship candidate root:** the whole `CacheStorage` directory under the profile
(not individual origin/guid children as separate product categories). Same
pattern as whole `Cache` / `Code Cache` / `GPUCache` roots today.

### Windows absolute path templates (Chrome / Edge)

| Browser | Candidate root pattern |
| --- | --- |
| Chrome | `%LOCALAPPDATA%\Google\Chrome\User Data\<Profile>\Service Worker\CacheStorage` |
| Edge | `%LOCALAPPDATA%\Microsoft\Edge\User Data\<Profile>\Service Worker\CacheStorage` |

`<Profile>` is the Local State catalog directory name (`Default`, `Profile 1`,
…). Same relative allowlist for every ordinary user profile Foal already
enumerates. Guest / System profiles remain excluded by existing catalog rules.

Windows path casing is case-insensitive; canonical spelling from Chromium docs
and live trees: **`Service Worker`** (space) and **`CacheStorage`** (no space).

---

## Include table

| Relative path under profile | Browsers | Role | Evidence class |
| --- | --- | --- | --- |
| `Service Worker\CacheStorage` | Chrome, Edge | Cache Storage API backend (named caches of Request/Response) | Chromium service_worker README + cache_storage README; W3C “request and response store”; Chrome DevTools Application → Cache Storage |

Implement match: one additional Chromium `cacheDirectoryKinds` entry that
resolves as `filepath.Join(profilePath, "Service Worker", "CacheStorage")`
(or equivalent multi-segment kind). Kind label may be
`Service Worker\CacheStorage` / `Service Worker/CacheStorage` for JSON
display — product choice, but path join must produce the nested directory.

---

## Exclude table (must never become candidates via this work)

| Path / pattern | Why excluded |
| --- | --- |
| `Service Worker` (parent as whole) | Mixed tree: Database + ScriptCache + CacheStorage |
| `Service Worker\ScriptCache` | SW **scripts** for installed registrations, not Cache API entries |
| `Service Worker\Database` | LevelDB **registration metadata** / state |
| `Cache`, `Code Cache`, `GPUCache` | Already allowlisted; do not change semantics |
| Profile root / `User Data` root | Over-broad; existing fail-closed |
| `Extensions`, `Local Extension Settings`, … | Extension install/state |
| `Cookies`, `Network\Cookies`, login DB | Credentials / session |
| `History`, `Visited Links`, `Top Sites` | History |
| `Local Storage`, `Session Storage`, `WebStorage` | Origin web storage state |
| `IndexedDB`, `File System`, `blob_storage` | Persistent site data / blobs |
| `Preferences`, `Secure Preferences`, `Bookmarks` | Configuration |
| `Service Worker` under VS Code / Cursor / Teams / other apps | Different product roots; not browser `User Data` profiles |
| Chromium forks (Brave, Opera, …) | Deferred (ADR 0019); paths analogous but not in scope |
| Firefox any path | Out of scope for this ship (see § Firefox) |

---

## Regenerability analysis

### What CacheStorage holds

1. **W3C Service Workers** describes a
   [“request and response store”](https://w3c.github.io/ServiceWorker/#cache-objects)
   “similar in design to the HTTP cache” to build offline-enabled apps
   ([Service Workers Nightly](https://w3c.github.io/ServiceWorker/), abstract +
   Cache objects).

2. **Chromium cache_storage README** states the browser-process implementation
   of the Cache Storage specification stores **Request/Response key-value
   pairs** in `disk_cache` backends under
   `$PROFILE/Service Worker/CacheStorage/...`.

3. **web.dev** (Chrome product surface): Cache Storage API uses `Request` keys
   and `Response` values; no automatic HTTP freshness; entries persist until
   code deletes them; used heavily for SW offline strategies
   ([Service workers and the Cache Storage API](https://web.dev/articles/service-workers-cache-storage)).

4. **Chrome DevTools** documents viewing/deleting **Cache Storage** entries
   separately from the HTTP cache under Application → Storage → Cache Storage,
   and clearing site data with the Cache Storage checkbox
   ([View cache data](https://developer.chrome.com/docs/devtools/storage/cache)).

### What deleting CacheStorage alone does **not** do

Because Database and ScriptCache are siblings and must remain non-candidates:

| User concern | Effect of deleting only `CacheStorage` |
| --- | --- |
| Log out / cookies | **No** — cookies untouched |
| History / autofill / bookmarks | **No** |
| IndexedDB / Local Storage user data | **No** |
| Unregister all service workers | **No** — registrations + SW scripts remain |
| Push subscription records | **Not** stored in CacheStorage as the primary store; push uses other SW/platform storage (not proven as CacheStorage content) |
| Offline PWA asset shells | **Yes impact** — precached shells/assets gone until SW re-populates |

Regenerability: sites and service workers re-create Cache API entries via
`caches.open` / `cache.add` / `cache.put` on install/activate/fetch (standard
PWA / Workbox patterns). Content is **re-downloadable network Response bodies**
plus metadata, not browser login DBs or user-authored documents under the
profile root.

### Residual risk (disclose, same class as existing `Cache`)

A site **may** put any `Response` body into the Cache API, including API JSON
that reflects user-specific data. That is site-controlled cache of network-class
payloads, not Foal-owned user documents. The existing HTTP `Cache` allowlist
already accepts temporary copies of page/asset bytes. Foal permanent bar still
holds for **login state / history / configuration / credentials as primary
store** — CacheStorage is not those stores.

**Confidence:** High that surviving content under the exact root is the Cache
API backend; Medium residual that individual Response bodies can be sensitive
until re-fetched (mitigate with impact notice, not by blocking permanent).

---

## User impact / notice text recommendation

Deleting `Service Worker\CacheStorage` is **higher UX impact** than deleting
HTTP `Cache` alone because PWAs and offline shells commonly depend on it.

Recommended preview / confirmation notice language (English product surface;
tune to existing notice style):

> Browser Cache Storage (Service Worker CacheStorage) holds site-controlled
> offline and performance caches of web Request/Response data. Deleting it does
> not remove cookies, passwords, history, or IndexedDB. Progressive Web Apps and
> sites that work offline may need a network connection on next visit so the
> site can rebuild its cache. First loads after cleanup may be slower.

Do **not** claim: secure erasure; that service workers are unregistered; that
push is fully wiped; that extensions are cleared.

---

## Idle / locking analysis

| Question | Answer | Confidence |
| --- | --- | --- |
| Extra fail-closed rules beyond ADR 0007? | **No** for ship | High |
| Browser idle-before-and-after complete inspection sufficient? | **Yes** — CacheStorage is profile-local disk_cache opened by browser processes; same lock/in-use class as `Cache` / `Code Cache` | High |
| Per-origin or per-cache locking? | Not required — Foal already refuses partial inspection of an existing allowlisted root | High |
| Process stopping / elevation? | Still forbidden | High |

Rationale: Chromium CacheStorage classes run in the browser process IO path and
hold backends open while caches are in use. Existing whole-browser running
detection + idle-after-inspection already prevents measuring/deleting while the
browser is active. Incomplete walk/read of `CacheStorage` must still discard
that browser’s opportunity (existing rule).

---

## Multi-profile

Same relative path under every ordinary profile directory from `Local State`
(`Default`, `Profile N`, named profiles). No special-case path per profile.

| Profile example | Relative allowlist path |
| --- | --- |
| `...\User Data\Default` | `Service Worker\CacheStorage` |
| `...\User Data\Profile 1` | `Service Worker\CacheStorage` |

Existing multi-profile enumeration, protection checks, and complete-inspection
aggregation apply unchanged. Missing `CacheStorage` under a profile is silent
absence for that kind (same as missing `GPUCache`).

---

## Firefox note (explicitly out of scope for this research ship decision)

Firefox implements the Cache API, but storage is **not** a clean Chromium-style
profile-relative `Service Worker\CacheStorage` sibling of HTTP `cache2`.

- Firefox local profile layout places many origin site-data types under
  `storage/default/<origin>/` (e.g. `ls`, `idb`, and related stores) —
  forensic/layout literature treats these as mixed origin storage, not a single
  pure Cache API root.
- Foal already permanent-deletes only Firefox `cache2` (HTTP cache). Expanding
  into `storage/default` without an exact pure-cache child proof would violate
  the permanent bar.

**Decision:** do not add a Firefox Cache API path in the follow-up Chrome/Edge
ticket. Optional future research issue if a first-party Firefox layout doc
identifies an exact regenerable Cache-only root.

---

## Permanent eligibility decision + rationale

| Decision | **`delete_permanently` justified** for exact `Service Worker\CacheStorage` under Chrome/Edge profiles as an extension of `browser_cache` |
| --- | --- |
| Rationale | (1) Chromium defines the directory as the Cache Storage API backend of Request/Response pairs via `disk_cache`. (2) W3C defines Cache objects as a request/response store for offline/performance, not credentials/history. (3) Same regenerability class as already-permanent `Cache` / `Code Cache` / `GPUCache`. (4) Exact root excludes mixed siblings. (5) Existing browser idle + complete-inspection gates apply. |
| Not Recycle Bin only | Regenerability is proven to the same standard used for current permanent browser_cache roots; Recycle Bin would be an unnecessary downgrade. |
| Not “do not add” | Primary layout + content class evidence is strong; ROI can be large; exclusions are clear. |

Update policy matrix text when implementing (illustrative):

> Only allowlisted Chrome/Edge (`Cache` / `Code Cache` / `GPUCache` /
> `Service Worker\CacheStorage`) and Firefox (`cache2`) profile cache roots;
> each browser must be idle before and after complete inspection. Never whole
> `Service Worker`, `ScriptCache`, or `Database`.

---

## Risks

| Risk | Severity | Mitigation |
| --- | --- | --- |
| **PWA offline broken until re-cache** | High UX, expected | Explicit impact notice; opt-in category; permanent authorization already required |
| **Site-chosen Response bodies may include user-specific API data** | Medium residual | Same class as HTTP cache; notice; never market as “privacy wipe” |
| **Multi-profile large totals** | Medium | Existing per-profile measure; user may clear TUI selection |
| **Locking if browser races inspection** | Medium | Keep idle-before-and-after + incomplete discard; no process kill |
| **Accidental parent `Service Worker` deletion** | High if mis-implemented | Exact child only; tests must forbid parent/ScriptCache/Database |
| **Confusion with ScriptCache** | Medium | Document exclude; never add ScriptCache without separate research |
| **Chrome “Cached images and files” UI does not always free CacheStorage** | Low (product context) | Confirms CacheStorage is a distinct store from HTTP cache; Foal is intentionally broader for this root |
| **Firefox accidental scope creep** | Medium process risk | Explicit out-of-scope note; no Firefox kind change |

---

## If ship: proposed path matchers, idle policy, notice, tests

### Path matchers

Chromium configs only:

```text
cacheDirectoryKinds: Cache, Code Cache, GPUCache, Service Worker\CacheStorage
```

Implementation sketch (not applied here): ensure join is nested:

```go
// Conceptual — implement ticket owns the real change.
filepath.Join(resolvedProfilePath, "Service Worker", "CacheStorage")
```

If `cacheDirectoryKinds` remains flat strings joined with a single
`filepath.Join(profile, kind)`, use a kind whose separators work with
`filepath.Join` on Windows (e.g. store as `filepath.Join("Service Worker", "CacheStorage")`
at config init, or switch kinds to `[]string` segments).

Firefox config: **unchanged** (`cache2` only).

### Idle policy reuse

- Same `browser_cache` category.
- Same running-application detection per browser.
- Same idle-before-and-after complete inspection.
- No new process list; no elevation; no stop-browser.

### Impact notice

Use the § User impact text (or equivalent) on dry-run and TUI confirmation when
`browser_cache` is selected / opted in. May apply to the whole category notice
(simplest) since CacheStorage is additive within the same category.

### Test fixtures outline

1. **Chrome/Edge fixture profile** with:
   - `Cache/`, `Code Cache/`, `GPUCache/` (existing)
   - `Service Worker/CacheStorage/<fake-origin>/<fake-guid>/` with opaque bytes
   - `Service Worker/ScriptCache/` file (must **not** count)
   - `Service Worker/Database/` file (must **not** count)
   - `Service Worker/worker.bin` or other parent-level file (must **not** count)
2. Assert measured bytes include only allowlisted roots including CacheStorage.
3. Assert JSON kind list / paths contain CacheStorage when present.
4. Assert forbidden strings remain: whole `Service Worker` as candidate path,
   ScriptCache, Database, Cookies, History, Extensions, IndexedDB.
5. Multi-profile: `Default` + `Profile 1` both contribute CacheStorage when
   present.
6. Idle gate: browser running ⇒ no browser_cache opportunity (existing).
7. Incomplete inspection of CacheStorage ⇒ discard (existing incomplete path).
8. Permanent action still `delete_permanently` for category; execute still
   requires `--allow-permanent`.
9. Protection on CacheStorage path suppresses as today.

---

## Follow-up implementation issue skeleton (do not implement in #257)

**Title:** `feat(clean): allowlist Chromium Service Worker\CacheStorage in browser_cache`

**Body outline:**

```markdown
## Summary
Extend Chrome/Edge `browser_cache` allowlist with profile-relative
`Service Worker\CacheStorage` only. Evidence:
`docs/research/chromium-service-worker-cachestorage.md` (#257).

## Scope
- Chromium kinds: add nested `Service Worker\CacheStorage`
- Planned action: keep `delete_permanently` for `browser_cache`
- Idle: reuse existing browser idle-before-and-after complete inspection
- Impact notice: PWA offline / re-download (see research note)
- Policy matrix + CONTEXT/AGENTS allowlist wording if they list kinds

## Non-goals
- Whole `Service Worker` parent
- `ScriptCache`, `Database`
- Firefox Cache API / storage/default
- Chromium forks
- Process stopping, elevation, secure erase

## Acceptance
- [ ] Path join resolves nested directory under each Local State profile
- [ ] Dry-run JSON measures CacheStorage when present
- [ ] Execute permanently deletes only after idle + `--allow-permanent`
- [ ] Tests: include CacheStorage; exclude ScriptCache/Database/parent/SW siblings
- [ ] Impact notice documents offline PWA effect
- [ ] Policy docs list the fourth Chromium kind
```

**Labels (suggested):** `enhancement`, triage per project vocabulary.

---

## Research questions answered (checklist)

1. **Layout:** Yes — stable profile-relative `Service Worker\CacheStorage` for
   Chrome and Edge; siblings `ScriptCache` and `Database` are not pure Cache API
   cache.
2. **Regenerability:** Yes for all surviving content under that exact root
   (Request/Response via disk_cache); no login/history/config primary stores.
3. **User impact:** Offline PWA/shells and first-load performance; not logout;
   notice text recommended above.
4. **Locking / idle:** Existing browser idle-before-and-after complete
   inspection remains sufficient.
5. **Multi-profile:** Same relative allowlist under each catalog profile.
6. **Exclusions:** Whole parent, ScriptCache, Database, Extensions, Cookies,
   History, Local Storage, IndexedDB, etc. remain non-candidates.
7. **Firefox:** Out of scope for this ship (mixed origin storage).
8. **Permanent eligibility:** **Yes** — `delete_permanently` justified as
   same class as existing allowlisted browser cache roots.

---

## Sources

### Primary (layout + content class)

- [Chromium — Service workers README (Storage)](https://chromium.googlesource.com/chromium/src/+/lkgr/content/browser/service_worker/README.md) — `Database`, `ScriptCache`, `CacheStorage` paths and roles.
- [Chromium — Cache Storage README](https://chromium.googlesource.com/chromium/src/+/lkgr/content/browser/cache_storage/README.md) — `$PROFILE/Service Worker/CacheStorage/<origin>/<cache>/`, Request/Response in disk_cache.
- [W3C Service Workers Nightly](https://w3c.github.io/ServiceWorker/) — Cache objects as request/response store; offline motivation.
- [W3C Cache objects section](https://w3c.github.io/ServiceWorker/#cache-objects).
- [web.dev — Service workers and the Cache Storage API](https://web.dev/articles/service-workers-cache-storage) — Request/Response API model; no automatic expiry.
- [Chrome DevTools — View cache data](https://developer.chrome.com/docs/devtools/storage/cache) — Cache Storage vs HTTP cache; clear site data.

### Foal product / prior art

- [Issue #257](https://github.com/CoreyLyn/Foal/issues/257)
- [`internal/clean/browser_cache.go`](../../internal/clean/browser_cache.go)
- [ADR 0007](../adr/0007-clean-browser-cache-discovery-requires-running-application-detection.md)
- [`docs/plan/clean-deletion-policy.md`](../plan/clean-deletion-policy.md)
- Research style peers: [`explorer-thumbnail-and-inet-cache-allowlists.md`](./explorer-thumbnail-and-inet-cache-allowlists.md), [`amd-intel-gpu-shader-caches.md`](./amd-intel-gpu-shader-caches.md)

### Path inventory corroboration only (not permanent authority)

- SuperUser / Edge community paths for `...\User Data\Default\Service Worker\CacheStorage`
- Winapp2 / cleaner inventories that list `Service Worker` under Chrome profiles
  (often over-broad whole-parent deletes — **do not copy** that breadth)
- MS Q&A / forums noting multi-GB `CacheStorage` growth (ROI context)

---

## Go / no-go

| Follow-up | Verdict |
| --- | --- |
| `feat(clean)` allowlist extension for Chrome/Edge `Service Worker\CacheStorage` under `browser_cache` | **GO** |
| Planned action | **`delete_permanently`** (existing category) |
| Whole `Service Worker` / ScriptCache / Database | **NO-GO** |
| Firefox parallel ship | **NO-GO** (separate research if desired) |
