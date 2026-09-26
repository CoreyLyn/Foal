# Unity Package Manager global cache (Clean eligibility)

Status: researched 2026-09-26. Evidence for the `unity-package-cache` Clean
category.

## Decision

| Scope | Decision | Planned action |
| --- | --- | --- |
| Exact documented children of `%LOCALAPPDATA%\Unity\cache` | **Ship permanent (opt-in, `dev-caches`)** | `delete_permanently` |
| `git-lfs` caches, the root, the `upm` parent, unknown siblings | **Excluded** | — |
| Custom cache roots (`UPM_CACHE_ROOT` and similar) | **Not followed** | — |

## Evidence

Unity Manual, "Global cache" (Windows, ordinary user):

| Unity version | Root | Children |
| --- | --- | --- |
| 2022.3, 2023.1 | `%LOCALAPPDATA%\Unity\cache` | `npm` (registry data), `packages` (unpacked packages), `git-lfs` (only when enabled) |
| 2023.2 | `%LOCALAPPDATA%\Unity\cache\upm` | `db`, `packages`, `git-lfs` |
| Unity 6 | `%LOCALAPPDATA%\Unity\cache\upm` | `db`, optional `git-lfs`; `packages` is no longer used |

Sources:
[2022.3](https://docs.unity3d.com/2022.3/Documentation/Manual/upm-cache.html),
[2023.2](https://docs.unity3d.com/2023.2/Documentation/Manual/upm-cache.html),
[Unity 6](https://docs.unity3d.com/6000.0/Documentation/Manual/upm-cache.html),
[Unity 6 upgrade guide](https://docs.unity3d.com/6000.0/Documentation/Manual/UpgradeGuideUnity6.html),
[Unity 6 cache configuration](https://docs.unity3d.com/6000.0/Documentation/Manual/upm-config-cache.html).
Unity 6 states the old `upm\packages` folder can be deleted once no Unity 2023.2
project is maintained.

## Foal rule

Candidates are the existing real directories `npm`, `packages`, `upm\db`, and
`upm\packages`, each measured independently. `upm\packages` stays in the
allowlist: every child here is a cache another installed Unity version may
still use, and all of them are downloaded or unpacked again on demand. The
impact notice says so rather than claiming projects are unaffected.

`UPM_CACHE_ROOT` and the legacy per-cache variables are not followed. A custom
root is never touched; when one is set, versions honoring it stop using the
default root, so removing the default root only drops unused content. The cost
is missed candidates, never extra ones.

Gate: one logical Unity identity must be idle before discovery and after
measurement:

- `Unity.exe` — the Editor, per the Windows path in Unity's
  [command-line arguments](https://docs.unity3d.com/Manual/EditorCommandLineArguments.html) page;
- `Unity Hub.exe` — observed at `C:\Program Files\Unity Hub` on the research host;
- `UnityPackageManager.exe` — best-effort, unverified name for the Package
  Manager server; an extra name can only make the gate stricter.

Unity running or unknown skips the whole root.
