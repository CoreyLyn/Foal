# AMD and Intel GPU / shader cache paths (Clean eligibility)

Status: researched 2026-07-17. This note is evidence for implement ticket
[#238](https://github.com/CoreyLyn/Foal/issues/238). It does **not** register
executable Clean categories, catalog entries, or deletion behavior.

Parent product decision: [ADR 0019](../adr/0019-cleanup-category-gap-decisions.md)
item 3 — AMD and Intel GPU/shader caches, symmetric to `d3d_shader_cache` /
`nvidia_dx_cache`, after path and layout evidence; no vendor-merged mega-category.

## Dated conclusions

| Vendor | Decision | Planned action if shipped | Confidence |
| --- | --- | --- | --- |
| **AMD** | **Ship permanent** | `delete_permanently` | High for path names and regenerability; medium for LocalLow sibling (no AMD GPU on research host) |
| **Intel** | **Ship permanent** | `delete_permanently` | High (primary path observed on research host + community/cleaner corroboration) |

Neither vendor is “ship Recycle Bin” or “do not ship.” Age and “cache-like name”
alone are not the proof; regenerability and exact current-user roots are.

## Prior art in Foal (do not re-derive)

| Category | Relative root under current-user Local AppData | Action | Notes |
| --- | --- | --- | --- |
| `d3d_shader_cache` | `D3DSCache` | `delete_permanently` | OS DirectX shader cache; existence discovery; no idle gate |
| `nvidia_dx_cache` | `NVIDIA\DXCache` | `delete_permanently` | Vendor DX shader cache only; not whole `NVIDIA` tree; not GLCache in production |

Catalog registration pattern (reference only):

```text
existenceOpportunityEntry(..., "D3DSCache")
existenceOpportunityEntry(..., "NVIDIA", "DXCache")
```

Shared policy both new vendors should mirror:

- Current-user only; no elevation; missing root = silent absence.
- Existence observation (not idle-age). Age is not a safety signal for regenerating GPU caches.
- `RunningApplicationPolicyNotApplicable` (same as D3D / NVIDIA). Locked/in-use files fail or skip locally; do not stop processes.
- Opt-in opportunity; permanent needs per-run authorization.
- Separate category IDs per vendor (ADR 0019). Suggested IDs for #238:
  - `amd_gpu_shader_caches`
  - `intel_gpu_shader_cache`

## Regenerable proof standard used here

Per `docs/plan/clean-deletion-policy.md`: permanent eligibility requires that
**all surviving content under the exact candidate root** is regenerable or
re-downloadable, and excludes user-authored, diagnostic, configuration, history,
and login state.

Proof classes accepted in this note:

1. **Vendor / OS first-party statement** that shader cache is stored compiled
   output and can be reset/deleted (AMD Adrenalin FAQ “Reset Shader Cache”).
2. **OS integration class** for DirectX shader caches (Microsoft DirectX-Specs
   D3DSCache + Disk Cleanup “DirectX Shader Cache”) establishing the same
   product class Foal already permanent-deletes for `d3d_shader_cache`.
3. **Controlled layout evidence**: fixed known roots whose children are only
   opaque compiler cache blobs (`.parc` / hash-named binaries), not settings or
   profiles.
4. **Cross-tool corroboration** (Winapp2.ini structured cleaner entries) only as
   path inventory, not as product authority.

Age thresholds and folder-name substrings alone are **insufficient**.

---

## AMD — ship permanent

### Candidate roots (current-user only)

Parent directory (never a candidate):

```text
%LOCALAPPDATA%\AMD
```

**Ship as exact child roots only** (each existing directory is one candidate
root, whole-root measure/delete of that child only). Windows paths are
case-insensitive; implement fixtures should use these canonical spellings:

| Relative path (under `%LOCALAPPDATA%`) | Role | Content evidence |
| --- | --- | --- |
| `AMD\DxCache` | DirectX 10/11 driver shader cache | Multi-user investigation: `*.parc` cache files; commonly multi-GB |
| `AMD\DxcCache` | DirectX 12 driver shader cache | Same `.parc` naming family |
| `AMD\Dx9Cache` | DirectX 9 driver shader cache | `*.bin` blobs in controlled listings |
| `AMD\OglCache` | OpenGL driver shader cache | `.parc` files containing AMDGPU / proprietary shader compiler strings |
| `AMD\VkCache` | Vulkan driver shader cache | `.parc` files in controlled listings |

Optional secondary root (ship only if implementer keeps exact name match; lower
host evidence here):

| Relative path | Role | Notes |
| --- | --- | --- |
| `%USERPROFILE%\AppData\LocalLow\AMD\DxCache` | Alternate / low-integrity DX cache | Listed by Winapp2; **not observed** on the research host (no AMD adapter). Resolve via Known Folder `FOLDERID_LocalAppDataLow` + `\AMD\DxCache`, not by string-hacking `%LOCALAPPDATA%`. |

### Excluded siblings and non-targets (must not become candidates)

Under `%LOCALAPPDATA%\AMD` (and related current-user AMD trees), **do not** treat
as GPU shader-cache candidates:

| Path / pattern | Why excluded |
| --- | --- |
| `%LOCALAPPDATA%\AMD` (parent as whole) | Mixed tree: caches + software UI + logs |
| `AMD\RadeonSoftware\` (entire tree, including `Cache`, `QtWebEngine`, `vkcache`) | Adrenalin UI / Electron-class app data, not driver shader cache |
| `AMD\cn\` | Control / logging surface |
| `AMD\Fuel\` | Client logs |
| `AMD\Ryzen Master\`, `AMD\StoreMI\`, `AMD Ryzen Master\` | CPU/storage tools, not GPU shader cache |
| `%LOCALAPPDATA%\RadeonSettings\`, `%LOCALAPPDATA%\Radeon*\` | Legacy UI paths |
| `%ProgramData%\AMD\...`, `%ProgramFiles%\AMD\...`, `C:\AMD\` | Machine / install / admin scope |
| `%LOCALAPPDATA%\Packages\*\AC\*\AMD\DxCache` | UWP package container copies; out of fixed-root current-user model |
| System profile / LocalService copies under `%WINDIR%\...` | Not current interactive user |

### Regenerable rationale (AMD)

1. **AMD first-party (primary):** AMD Software: Adrenalin Edition FAQ DH3-012
   documents **Reset Shader Cache**:

   > Shader cache allows for faster loading times in games and reduced CPU usage
   > by compiling and storing frequently used game shaders, rather than
   > regenerating them each time they are needed. **Reset Shader Cache can be
   > used to delete all stored shader cache files.**

   Source:
   [Customize Graphics Settings with AMD Software: Adrenalin Edition (DH3-012)](https://www.amd.com/en/resources/support-articles/faqs/DH3-012.html)
   (last updated 2025-03-03 on AMD.com).

2. **Layout class:** Controlled multi-user investigation of
   `%LOCALAPPDATA%\AMD\{DxCache,DxcCache,Dx9Cache,OglCache,VkCache}` shows only
   opaque cache files (`.parc` / `.bin`), not user documents or settings. OpenGL
   cache payloads include strings such as “Proprietary GPU Shader Compiler” /
   AMDGPU pipeline metadata — consistent with compiled shader storage, not
   configuration.
   Source: [pbhj investigation gist](https://gist.github.com/pbhj/ccae7ef1d1446f4450005de139c601c4)
   (layout inventory; not product authority).

3. **Product symmetry:** Same regenerating-driver-cache class as shipped
   `nvidia_dx_cache` (`%LOCALAPPDATA%\NVIDIA\DXCache`).

### Machine evidence (research host)

Research host 2026-07-17:

- Adapters: `Intel(R) Arc(TM) Graphics` only (no AMD GPU).
- `%LOCALAPPDATA%\AMD` and all listed AMD child roots: **missing**.
- Path recommendations therefore rest on AMD FAQ + multi-source layout inventory,
  not live AMD regeneration on this machine.

### Idle / locking notes (AMD)

- No process stop. Match D3D/NVIDIA: no running-application gate.
- Drivers may memory-map some `.parc` files while games or the desktop compositor
  run; implement #238 must treat in-use failures as local skips/failures, never
  elevation or process kill.
- User impact notice: temporary longer load / shader recompilation stutter after
  permanent delete (same class as NVIDIA/D3D).

### Fixture sketch for #238 (AMD)

```text
{tempLocalAppData}\AMD\DxCache\example.0.parc          # candidate content
{tempLocalAppData}\AMD\DxcCache\example.0.parc
{tempLocalAppData}\AMD\Dx9Cache\example.bin
{tempLocalAppData}\AMD\OglCache\example.parc
{tempLocalAppData}\AMD\VkCache\example.parc
{tempLocalAppData}\AMD\RadeonSoftware\UserSettings.json # EXCLUDED sibling
{tempLocalAppData}\AMD\cn\note.log                      # EXCLUDED sibling
{tempLocalAppDataLow}\AMD\DxCache\example.parc          # optional secondary root
```

Resolver must:

1. Require parent segment `AMD` under Local AppData (or LocalLow for the optional root).
2. Allowlist only the exact child directory names above.
3. Never promote parent `AMD` or non-allowlisted siblings to candidates.
4. Emit one candidate per existing allowlisted child root (or one multi-root
   category that lists those roots only — product choice, but paths are fixed).

---

## Intel — ship permanent

### Candidate root (current-user only)

Exact single root:

```text
%USERPROFILE%\AppData\LocalLow\Intel\ShaderCache
```

Resolution for implement/tests:

1. Prefer Windows Known Folder `FOLDERID_LocalAppDataLow`
   (`{A520A1A4-1780-4FF6-BD18-167343C5AF16}`), then join `Intel\ShaderCache`.
2. Equivalent env expansion:
   `filepath.Join(os.UserHomeDir(), "AppData", "LocalLow", "Intel", "ShaderCache")`
   when the Known Folder is unavailable in fixtures.
3. Do **not** derive LocalLow by stripping `Local` from `%LOCALAPPDATA%` as the
   only strategy in production if a Known Folder API is already available
   elsewhere; fixtures may hard-code the relative form
   `AppData\LocalLow\Intel\ShaderCache` under a fake profile.

### Layout (observed 2026-07-17 on research host)

| Property | Observation |
| --- | --- |
| Adapter | `Intel(R) Arc(TM) Graphics` driver `32.0.101.8801` |
| Path | `C:\Users\corey\AppData\LocalLow\Intel\ShaderCache` |
| Shape | **Flat directory** — 250 files, **0 subdirectories** |
| Names | Extensionless 64-hex-char filenames (SHA-256-like) |
| Size sample | ~34.7 MB total on this host |
| Siblings under `LocalLow\Intel` | Only `ShaderCache` |
| Exclusive open sample | 18/20 sample files opened exclusive; **2/20 locked** while desktop GPU in use |

Whole-root candidate model is appropriate: every observed child is an opaque
cache blob. No settings files, no “Local History,” no profile databases under
this root.

### Excluded siblings and non-targets

| Path | Why excluded |
| --- | --- |
| `%LOCALAPPDATA%\Intel` (parent) | Mixed: AGS, IntelGraphicsSoftware UserSettings, SUR telemetry — **not** shader cache |
| `%LOCALAPPDATA%\Intel\IntelGraphicsSoftware\` | App settings / WebView data |
| `%LOCALAPPDATA%\Intel\AGS\`, `%LOCALAPPDATA%\Intel\SUR\` | Non-cache app/service data (observed on research host) |
| `%LOCALAPPDATA%\Intel\ShaderCache` or `...\DXCache` | **Not observed** on Arc research host; forum/third-party lists are inconsistent. Do **not** ship without a new evidence note |
| `%ProgramData%\Intel\ShaderCache` | Machine-wide / admin-adjacent; Winapp2 lists it; Foal: permission-boundary only, never executable Clean |
| `%WINDIR%\ServiceProfiles\LocalService\AppData\LocalLow\Intel\ShaderCache` | Service profile, not current user |
| Intel Graphics Command Center UWP package trees under `%LOCALAPPDATA%\Packages\AppUp.IntelGraphicsExperience*` | App cache, different product surface |

### Regenerable rationale (Intel)

1. **Layout + class match:** Flat, content-addressed opaque binaries under a
   fixed driver-owned `ShaderCache` directory for the current user, parallel to
   vendor DX caches Foal already treats as permanent regenerable caches.

2. **Intel developer community path acknowledgment:** Intel forum threads for
   Iris Xe / Arc discuss
   `C:\Users\<userid>\AppData\LocalLow\Intel\ShaderCache` as the on-disk driver
   shader cache and recommend deleting unused files when empty/corrupt cache
   files cause issues (regeneration path implied by continued app use).

3. **Structured cleaner inventory:** Winapp2 `[Intel Shader Cache *]` targets
   exactly:
   - `%UserProfile%\AppData\LocalLow\Intel\ShaderCache\*`
   - (plus non-current-user ProgramData / LocalService copies Foal must ignore)

4. **Not the same as Intel Precompiled Shaders cloud feature:** Intel support
   article [000102440](https://www.intel.com/content/www/us/en/support/articles/000102440/graphics.html)
   describes optional Steam-oriented **precompiled shader downloads** stored “on
   your system’s disk” without publishing this LocalLow path. That feature is
   re-downloadable when enabled; the LocalLow `ShaderCache` root observed here is
   the general driver cache and regenerates through normal rendering even when
   precompiled distribution is off. Do not conflate the two in UI copy beyond a
   generic “shaders recompile / may re-download” impact notice.

### Machine evidence (research host)

```text
LOCALAPPDATA = C:\Users\corey\AppData\Local
LocalAppDataLow (Known Folder) = C:\Users\corey\AppData\LocalLow

EXISTS  LocalLow\Intel\ShaderCache   children=250  bytes≈34751305  subdirs=0
MISSING Local\Intel\ShaderCache
MISSING Local\Intel\DXCache
MISSING ProgramData\Intel\ShaderCache
EXISTS  Local\Intel\{AGS, IntelGraphicsSoftware, SUR}   # excluded siblings
```

Sample names (not secrets; content-addressed blobs):

```text
2c7c61dbe648ab71ea78715033fc61175af9aa9abef684f315907d7cd6dabe95  (~7.5 MB)
0770a1fa0d630e54cd15211582e664aba0b8f7fc2df8db357e76df0bd7f0c292  (~7.2 MB)
```

No production files were deleted during research; regenerability is established
by cache class + vendor/community delete-and-reuse practice + Foal’s existing
permanent treatment of the same class for D3D/NVIDIA.

### Idle / locking notes (Intel)

- No process stop; no “Intel Graphics Software must be idle” gate for v1
  (symmetric to NVIDIA/D3D).
- Expect occasional sharing violations on active blobs; skip/fail locally.
- Impact notice: temporary shader recompilation cost; optional note that
  Precompiled Shaders (if user enabled) may re-download for supported titles.

### Fixture sketch for #238 (Intel)

```text
{tempProfile}\AppData\LocalLow\Intel\ShaderCache\aaaaaaaa...64hex   # candidate
{tempProfile}\AppData\LocalLow\Intel\ShaderCache\bbbbbbbb...64hex   # candidate
{tempProfile}\AppData\Local\Intel\IntelGraphicsSoftware\UserSettings\x.json  # EXCLUDED
{tempProfile}\AppData\Local\Intel\AGS\data.ags                      # EXCLUDED
{tempProfile}\AppData\Local\Intel\SUR\QUEENCREEK\...                # EXCLUDED
# ProgramData\Intel\ShaderCache must not be wired into executable discovery
```

Resolver must join **LocalLow + Intel + ShaderCache** only. The Local `Intel`
tree is never a scan root for this category.

---

## Shared do-not-ship list

| Target | Decision | Why |
| --- | --- | --- |
| Merged “GPU caches” mega-category | Do not ship | ADR 0019: separate vendors |
| `%LOCALAPPDATA%\D3DSCache` | Already shipped as `d3d_shader_cache` | OS root, not AMD/Intel |
| `%LOCALAPPDATA%\NVIDIA\DXCache` | Already shipped as `nvidia_dx_cache` | Not this ticket |
| Steam `steamapps\shadercache` | Do not ship here | Game/platform owned; not current-user driver root |
| Any `%ProgramData%\...` GPU cache | Do not ship as executable | Elevation / machine-shared |
| Whole `%LOCALAPPDATA%\AMD` or `%LOCALAPPDATA%\Intel` | Do not ship | Mixed non-cache siblings |
| Guessed `Local\Intel\DXCache` without observation | Do not ship | Not present on Arc host; weak evidence |
| Secure erase / admin Disk Cleanup parity claims | Never | Existing permanent-delete policy |

## Recommended product shape for #238 (non-binding to registration code)

| Suggested category ID | Roots | Eligibility | Action | TUI initially selected when measurable |
| --- | --- | --- | --- | --- |
| `amd_gpu_shader_caches` | Exact allowlisted children under Local `AMD` (+ optional LocalLow `AMD\DxCache`) | Opt-in | `delete_permanently` | Yes |
| `intel_gpu_shader_cache` | Exact LocalLow `Intel\ShaderCache` | Opt-in | `delete_permanently` | Yes |

Discovery: existence of each exact root; measure whole root; missing roots omit
silently. Report category: System (same family as D3D/NVIDIA).

## Evidence gathering method (verification)

Performed on worktree host 2026-07-17 without elevation and without deleting
production cache contents:

1. Enumerated `%LOCALAPPDATA%` and Known Folder LocalLow for
   `D3DSCache`, `NVIDIA\*`, `AMD\*`, `Intel\*`, and `ProgramData` Intel/AMD
   shader paths.
2. Listed Intel `ShaderCache` children (names, sizes, extension histogram,
   subdir count) and exclusive-open locking sample.
3. Listed Intel Local AppData siblings (`AGS`, `IntelGraphicsSoftware`, `SUR`).
4. Queried `Win32_VideoController` for adapter identity.
5. Read AMD first-party Adrenalin FAQ DH3-012 for Reset Shader Cache wording.
6. Read Microsoft DirectX-Specs Shader Cache page for D3DSCache / Disk Cleanup class.
7. Cross-checked Winapp2.ini structured path inventory for AMD `*Cache` and
   Intel Shader Cache entries (path names only).
8. Cross-checked controlled community layout investigation for AMD `.parc` trees.

## Out of scope for this ticket

- Catalog registration, opportunity IDs in Go code, TUI rows, deletion-policy
  matrix edits, or execute paths.
- Live AMD GPU regeneration test (no AMD adapter on research host).
- Elevating to inspect service-profile or ProgramData roots as Clean targets.

## Sources

Primary / high trust:

- [AMD DH3-012 — Reset Shader Cache](https://www.amd.com/en/resources/support-articles/faqs/DH3-012.html)
- [DirectX-Specs: D3D12 Shader Cache / D3DSCache](https://microsoft.github.io/DirectX-Specs/d3d/ShaderCache.html)
- [Intel Precompiled Shaders (000102440)](https://www.intel.com/content/www/us/en/support/articles/000102440/graphics.html)
- Foal prior art: `internal/clean/category_catalog.go` (`d3d_shader_cache`, `nvidia_dx_cache`);
  ADR 0006, ADR 0018, ADR 0019; `docs/plan/clean-deletion-policy.md`

Secondary (layout inventory only):

- [Winapp2.ini](https://github.com/MoscaDotTo/Winapp2) entries `[AMD *]`,
  `[Intel Shader Cache *]`, DirectX / D3DSCache cleaners
- [pbhj AMD DxCache investigation gist](https://gist.github.com/pbhj/ccae7ef1d1446f4450005de139c601c4)
- Intel community threads referencing `AppData\LocalLow\Intel\ShaderCache`

## Implement checklist handoff (#238)

- [x] Register **separate** AMD and Intel categories only (both ship permanent).
- [x] AMD: allowlist exact child directory names; never parent `AMD`.
- [x] Intel: exact LocalLow `Intel\ShaderCache` only; never Local `Intel` parent.
- [x] Fixtures: use relative layouts in this note; no live GPU required.
- [x] Update deletion-policy matrix + docs only when categories register.
- [x] Impact notice: shader recompilation / possible temporary stutter; no secure-erase language.
- [x] In-use file failures isolated; no elevation; no process stopping.
