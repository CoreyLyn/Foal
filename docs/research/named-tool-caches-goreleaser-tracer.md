# Named tool caches under temp roots — GoReleaser tracer (#259)

Status: researched 2026-07-17. This note is evidence for research+feat ticket
[#259](https://github.com/CoreyLyn/Foal/issues/259). It does **not** register
executable Clean categories, catalog entries, or deletion behavior.

Parent product constraints:

- Product boundaries / exact roots: `AGENTS.md`,
  [`docs/plan/clean-deletion-policy.md`](../plan/clean-deletion-policy.md)
- Regenerable permanent standard: all surviving content under an exact candidate
  root must be regenerable or re-downloadable; age, a cache-like name, or a Temp
  location is **not** enough.
- Existing Go coverage: shipped `go-cache` and `go-modcache` resolve
  `GOCACHE` / `GOMODCACHE` (or Windows defaults) from the current process
  environment only.

Research style peer: [`amd-intel-gpu-shader-caches.md`](./amd-intel-gpu-shader-caches.md).

## Dated conclusions

| Question | Decision | Confidence |
| --- | --- | --- |
| Ship a **GoReleaser-owned** Clean category (`goreleaser-cache` / similar) | **No-go** | High |
| Observed `C:\tmp\goreleaser-{cache,mod}` are regenerable | **Yes** (as Go toolchain caches, not as GoReleaser product state) | High (layout markers on research host) |
| Exact official Windows path table for “GoReleaser cache dirs” | **Does not exist** in primary sources | High |
| Permanent vs Recycle Bin if ever shipping *redirected Go caches* | Same class as `go-cache` / `go-modcache` → `delete_permanently` | High for content class; N/A for a goreleaser-named category |
| Scan `C:\tmp` / whole Temp for `*cache*` / tool-name prefixes | **No** without a first-party path contract + layout validation | High (product non-goal) |

**Ship recommendation for Phase B of #259 as written: do not implement.**

ROI on this host (~3.3 GB under `C:\tmp\goreleaser-*`) is real but does **not**
establish an official path. Eligibility requires official path proof—not the
local observation alone (issue acceptance criteria).

---

## Phase A checklist (issue questions)

### 1. GoReleaser official docs/env for cache directories on Windows

**Finding: none that define `goreleaser-cache` or `goreleaser-mod`.**

Primary sources checked:

| Source | What it says about caches / temp |
| --- | --- |
| [goreleaser.com Install](https://goreleaser.com/install/) | Install surfaces only; no user cache directory contract |
| [goreleaser.com Build](https://goreleaser.com/customization/builds/) | Artifacts go into project `dist`; no persistent global cache path |
| [Verifiable builds / gomod.proxy](https://goreleaser.com/customization/builds/verifiable_builds/) + [Building Go modules cookbook](https://goreleaser.com/resources/cookbooks/build-go-modules/) | When `gomod.proxy: true`, creates **project-local** `dist/proxy/{{ build.id }}`, writes a synthetic `go.mod`, copies `go.sum`, runs `go get module@version` |
| Source: `internal/pipe/gomod/gomod_proxy.go` (goreleaser/goreleaser `main`) | `dir := filepath.Join(ctx.Config.Dist, "proxy", build.ID)` — not under Temp, not named `goreleaser-mod` |
| Source: `internal/builders/golang/build.go` | Passes through env; only special-cases passthrough of host `GOCACHEPROG`; does **not** set `GOCACHE`/`GOMODCACHE` to tool-named temp dirs |
| Source: `internal/nodedist/download.go` | Node dist downloads: **no on-disk cache**; each call uses `os.MkdirTemp("", "goreleaser-nodedist-*")` under `os.TempDir()`, reaped by the OS |
| Source tree search for path helpers (`configdir`, `cachedir`, XDG, AppData, …) | No dedicated user cache-dir package/paths in `main` tree listing |
| GitHub code search for literal `goreleaser-cache` / `goreleaser-mod` as Go string paths in `goreleaser/goreleaser` | No product directory naming of that form |

**Override env vars that exist are Go’s, not GoReleaser’s:**

| Variable | Owner | Official role |
| --- | --- | --- |
| `GOCACHE` | Go command | Absolute path for build/test cache |
| `GOMODCACHE` | Go command | Directory for downloaded modules |
| `GOTMPDIR` | Go command | Temporary work dir override (not the long-lived build/module caches) |
| `TMP` / `TEMP` / `TMPDIR` | OS / process | Passthrough in some GoReleaser pipes (`internal/exec`, `internal/pipe/sbom`) for child processes — not a product cache root |

Windows defaults on the research host (`go env`, empty overrides):

| Var | Default observed |
| --- | --- |
| `GOCACHE` | `%LOCALAPPDATA%\go-build` → `C:\Users\corey\AppData\Local\go-build` |
| `GOMODCACHE` | `%USERPROFILE%\go\pkg\mod` → `C:\Users\corey\go\pkg\mod` |

Source: `go help environment` (`GOCACHE`, `GOMODCACHE`, `GOTMPDIR`); Go Modules
Reference and `cmd/go` package docs for cache semantics.

### 2. Regenerability of candidate content

**If the path is a real Go build or module cache: yes, regenerable** (same class
already permanent-eligible in Foal as `go-cache` / `go-modcache`).

Official clean surfaces:

- `go clean -cache` removes the entire build cache (`GOCACHE`)
- `go clean -modcache` removes the entire module download cache (`GOMODCACHE`),
  including unpacked versioned dependency source

Impact (must disclose if ever deleted by Foal):

- Rebuild cost for packages previously cached under that `GOCACHE`
- Re-download of modules (public proxy and private modules); offline/private
  restore risk for module cache — already required for `go-modcache`

**Not regenerable / not candidates if mis-identified:**

| Content | Why excluded |
| --- | --- |
| Project `dist/` / release artifacts | User release outputs; not a tool cache |
| `.goreleaser.yaml` / project config | User-authored |
| Secrets / tokens | Live in env, CI secrets, credential helpers — not these cache trees when they are pure Go caches |
| Arbitrary siblings under `C:\tmp` | Mixed user/installer/lock files; whole-root cleanup is a product non-goal |

GoReleaser’s own ephemeral `goreleaser-nodedist-*` temps are regenerable downloads
but are **not** a stable named root and are designed to be OS-reaped — not worth a
category without a durable layout contract.

### 3. Scan roots

| Root | Allowed for a *goreleaser* category? | Notes |
| --- | --- | --- |
| `%LOCALAPPDATA%\Temp` / `%TEMP%` | Only if first-party exact names land there | GoReleaser nodedist uses `os.TempDir()` with random `goreleaser-nodedist-*` prefixes — not stable |
| `C:\tmp`, `C:\temp` | **Not as official GoReleaser roots** | Observed host convention only; not documented by GoReleaser; issue forbids arbitrary drive roots |
| `%WINDIR%\Temp` | **No** | Shared system temp; out of scope |
| Project `dist/` | **No** for Clean default/opt-in | Project artifact territory (`purge` / out of Clean) |

**Approved parents for a name-based scanner would require a first-party path
contract that does not exist.** Local observation of `C:\tmp\…` is ROI-only.

### 4. Matching rule

Issue asks: exact directory name allowlist or env-resolved absolute roots — not
substring match.

| Rule | Status |
| --- | --- |
| Exact names `goreleaser-cache`, `goreleaser-mod` under approved parents | **Names are not first-party**; they appear to be a local/ad-hoc isolation convention for `GOCACHE`/`GOMODCACHE` when installing/running tools via `go` |
| Env-resolved absolute roots for “GoReleaser cache” | **No product env** such as `GORELEASER_CACHE` found |
| Substring `*cache*` / whole Temp | Explicit non-goal |

**Layout validation (research host) of the observed paths:**

| Path | Layout class | Markers |
| --- | --- | --- |
| `C:\tmp\goreleaser-cache` | Go **build** cache (`GOCACHE`) | `README`: “This directory holds cached build artifacts from the Go build system…”; hex-named child dirs (`00`…`1f` style) |
| `C:\tmp\goreleaser-mod` | Go **module** cache (`GOMODCACHE`) | `cache\download` present; module host paths (`github.com\…`, `golang.org\…`); `!`-escaped uppercase path elements typical of the module cache |
| `C:\tmp\actionlint-cache` / `actionlint-mod` | Same dual pattern for another Go tool | Corroborates **tool-install isolation convention**, not a GoReleaser-only product feature |

Default process env on research host does **not** point `GOCACHE`/`GOMODCACHE` at
`C:\tmp\…`; those trees are orphaned relative to the default Go env Foal already
cleans via `go-cache` / `go-modcache`.

### 5. Idle / process gate

| Gate | Assessment |
| --- | --- |
| Distinctive process `goreleaser.exe` | Useful only while a release is running; does **not** own the cache trees if they are Go’s |
| Shared Go toolchain gate (`go.exe` / existing go-cache identity) | Correct class for true `GOCACHE`/`GOMODCACHE` trees — matches shipped `go-cache` / `go-modcache` |
| Shared runtime without attribution | Same caveats as other package/build caches |

Fail-closed: unknown process state must not authorize mutation. Never stop
processes.

### 6. Permanent vs Recycle Bin

| Content class | Eligibility | Planned action if shipped |
| --- | --- | --- |
| Validated Go build cache root | Already proven (matrix `go-cache`) | `delete_permanently` |
| Validated Go module cache root | Already proven (matrix `go-modcache`) | `delete_permanently` |
| Unvalidated temp name match | **Not proven** | Do not ship; name alone insufficient |
| Whole `C:\tmp` / user Temp | Explicitly unsafe | Never |

Permanent deletion remains ordinary filesystem removal; per-run
`--allow-permanent` / TUI authorization unchanged.

### 7. Include / exclude tables (Windows)

#### Include — **none for a GoReleaser category**

| Candidate path | Why not shipped as `goreleaser-*` |
| --- | --- |
| `C:\tmp\goreleaser-cache` | Not first-party; is a redirected `GOCACHE` |
| `C:\tmp\goreleaser-mod` | Not first-party; is a redirected `GOMODCACHE` |
| `%TEMP%\goreleaser-nodedist-*` | Ephemeral MkdirTemp; no durable root |
| `<project>\dist\proxy\<build.id>` | Project-local verifiable-build workspace |

#### Exclude (must never become candidates under any “named tool cache” design)

| Path / pattern | Why excluded |
| --- | --- |
| Whole `C:\tmp`, `C:\temp`, `%TEMP%`, `%WINDIR%\Temp` | Mixed content; product non-goal |
| Substring match `*goreleaser*`, `*cache*` | Collision with unrelated folders |
| Project `dist/` archives, checksums, drafts | Release artifacts |
| Default `%LOCALAPPDATA%\go-build` | Already `go-cache` |
| Default `%USERPROFILE%\go\pkg\mod` | Already `go-modcache` |
| `GOPATH\pkg` siblings, `GOROOT`, installed tool binaries under `GOBIN` | Existing go-modcache exclusions |
| Config, tokens, git credentials, CI secret stores | Not caches |

#### What already covers regenerable Go caches

| Category | Resolution today | Gap vs observation |
| --- | --- | --- |
| `go-cache` | Non-empty `GOCACHE` env → else `%LOCALAPPDATA%\go-build` | Does **not** discover orphaned alternate cache dirs unless Foal’s own process env points at them |
| `go-modcache` | Non-empty `GOMODCACHE` → else `%USERPROFILE%\go\pkg\mod` | Same gap for redirected/orphaned module caches |

That gap is a **general redirected-Go-cache discovery** problem, not a
GoReleaser feature. Solving it by hard-coding tool names under `C:\tmp` invents
a path standard Foal cannot prove.

---

## Regenerable proof standard applied

Accepted proof classes for permanent eligibility (from clean-deletion-policy +
peer research notes):

1. First-party owner documentation of path + regenerability
2. Official env/default contract with rebuild/re-download semantics
3. Controlled layout evidence that the root holds only that cache class
4. Cross-tool corroboration only as inventory, not authority

For #259:

| Class | Result |
| --- | --- |
| GoReleaser first-party cache path | **Missing** |
| Go first-party `GOCACHE`/`GOMODCACHE` | **Present** — but already mapped to `go-cache` / `go-modcache` |
| Layout evidence on host for `C:\tmp\goreleaser-*` | **Matches Go caches**, not a separate GoReleaser layout |
| Name-based allowlist under `C:\tmp` | **Insufficient** as sole proof |

---

## Machine evidence (research host, 2026-07-17)

- OS: Windows; user Temp = `%LOCALAPPDATA%\Temp`
- `go env GOCACHE` = `C:\Users\corey\AppData\Local\go-build`
- `go env GOMODCACHE` = `C:\Users\corey\go\pkg\mod`
- Process env: no `GOCACHE`/`GOMODCACHE` overrides; no `goreleaser` on PATH
- Present under `C:\tmp\`:
  - `goreleaser-cache` (GOCACHE markers)
  - `goreleaser-mod` (GOMODCACHE markers; includes `github.com\goreleaser\…` among modules)
  - `actionlint-cache`, `actionlint-mod` (same dual pattern)
- Issue ROI figures (non-authoritative): ~1.96 GB + ~1.33 GB for the two
  goreleaser-named trees
- Foal `.goreleaser.yaml` sets only build/archive/release fields; no cache path
  env overrides

Creator of the `C:\tmp\{tool}-{cache,mod}` convention was **not** found in
GoReleaser primary sources or Foal repo scripts. Treat as host/tooling
convention outside product contracts.

---

## Go / no-go

### No-go (Phase B as issue-framed)

Do **not** add:

- Category id `goreleaser-cache` (or family) that treats
  `goreleaser-cache` / `goreleaser-mod` as official GoReleaser roots
- Scanning whole `C:\tmp` / Temp for tool-named folders without first-party docs
- Implicit permanent deletion based on name or Temp location alone

Reasons: missing primary-source path contract; observed content is Go’s; product
forbids whole-root temp cleanup; inventing allowlisted names under `C:\tmp`
fails the “exact roots only” bar used by sibling categories.

### Out of scope / future (not #259 Phase B)

If product later wants orphaned redirected Go caches:

1. Frame as **Go toolchain cache instances**, not tool marketing names
2. Require **layout validation** (GOCACHE README / module-cache structure), not
   name alone
3. Decide approved parents explicitly (likely still **not** arbitrary `C:\tmp`
   unless documented by a packaging tool Foal chooses to support)
4. Reuse permanent action + Go idle gate from `go-cache` / `go-modcache`
5. Prefer env/default resolution expansion over filesystem folklore

That is a separate research+design ticket.

### Ephemeral `goreleaser-nodedist-*` under Temp

Optional micro-follow-up only: exact prefix + temp parent + directory age/idle —
still low value (OS-reaped, small, random suffix). **Not** recommended in the
same PR as a “named tool cache family.”

---

## Acceptance criteria mapping

| Research AC | Result |
| --- | --- |
| Research doc with primary sources and go/no-go | This file; **no-go** for GoReleaser-named category |
| Exact candidate path table for Windows | Empty include table for product category; observed paths classified as redirected Go caches |
| Explicit permanent-delete decision | N/A for new category; redirected Go caches would be permanent **if** ever covered via Go categories with layout proof |

Implementation AC for Phase B: **deferred / cancelled** unless the issue is
re-scoped away from GoReleaser product caches.

---

## Primary sources (index)

1. GoReleaser docs: [Install](https://goreleaser.com/install/),
   [Build](https://goreleaser.com/customization/builds/),
   [Verifiable builds](https://goreleaser.com/customization/builds/verifiable_builds/),
   [Building Go modules](https://goreleaser.com/resources/cookbooks/build-go-modules/)
2. GoReleaser source (github.com/goreleaser/goreleaser `main`):
   `internal/pipe/gomod/gomod_proxy.go`,
   `internal/builders/golang/build.go`,
   `internal/nodedist/download.go`
3. Go command: `go help environment` (`GOCACHE`, `GOMODCACHE`, `GOTMPDIR`);
   `go clean -cache` / `go clean -modcache` semantics via `cmd/go` docs
4. [Go Modules Reference](https://go.dev/ref/mod)
5. Foal: `docs/plan/clean-deletion-policy.md` (`go-cache`, `go-modcache` matrix
   rows); `internal/clean/dev_cache.go` resolvers
6. Host layout evidence: `C:\tmp\goreleaser-{cache,mod}`, `C:\tmp\actionlint-{cache,mod}`
   (2026-07-17; non-authoritative for product paths)

## Non-goals restated

- Generic clean of all temp drives
- Package Cache / Windows Installer / WSL VHDX / hibernation
- Implementing additional tool names in the same PR without primary-source path
  proof (actionlint shows the same *convention*, not a shippable catalog)
