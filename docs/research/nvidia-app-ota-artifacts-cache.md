# NVIDIA app OTA artifacts cache eligibility

Status: researched 2026-07-21. This note evaluates only
`C:\ProgramData\NVIDIA Corporation\NVIDIA app\UpdateFramework\ota-artifacts`,
the OTA update-framework artifact tree used by the current NVIDIA app (the
successor to GeForce Experience). It uses NVIDIA first-party support material
and read-only evidence from NVIDIA-signed software installed on the research
host. It does not register a Clean category or authorize a delete action.

The legacy `C:\ProgramData\NVIDIA Corporation\Downloader` root is evaluated in
a separate note (`nvidia-installer-downloader-cache.md`) and is permanently
out of scope here. The two trees are different generations of the NVIDIA
update framework with different roots, persisted-state schemas, and lifecycle
rules; evidence about one never authorizes action on the other.

## Decision

| Proposed scope | Evidence result | Maximum defensible action |
| --- | --- | --- |
| Entire `ota-artifacts` root | **Rejected** | none |
| Named-component subdirs (`nvapp`, `grd`, `crd`, `default`) | **Rejected** | none |
| Hash-named leaf dirs (`<component>\<hash>\<exe>`) | **Not proven** | none |
| Hash-named extracted post-processing trees (`<component>\post-processing\<hash>\`) | **Rejected - active self-update payload** | none |
| Orphan UUID subdirs at the `ota-artifacts` root | **Not proven** | none |
| `status\`, `profile-catalog\`, `registry\`, `plugins\` siblings | **Excluded** | none |

The requested `delete_permanently / Proven` and `move_to_recycle_bin / Not
proven` classifications are **both not supported** for any sub-scope of
`ota-artifacts`. NVIDIA's own NVIDIA-signed `Downloader.dll` 11.0.7.247 and
`UpdateFrameworkPlugin.dll` 11.0.7.247 prove that NVIDIA has an internal
`CleanupOldArtifcacts` (sic) capability that deletes both downloaded packages
and extracted post-processing trees under this root. That capability is
NVIDIA's own lifecycle management: it is triggered by NVIDIA's scheduler,
guarded by cleanup-pending markers, resumable across shutdown, and not
exposed as a public removal contract. NVIDIA publishes no statement that
external deletion of `ota-artifacts` is supported, no stable cross-version
schema for `status\<component>\download.json`, no numeric mapping for the
`state` field, and no documented writer set.

The accepted product policy is therefore a **path-free Review clue** that
points the user at NVIDIA app's own update UI. Foal does not execute against
this tree, does not measure it as Potential space or Opt-in reclaimable
bytes, and never stops NVIDIA processes or services to access it. This
mirrors the WeChat backup, WPS cloud, and OneDrive local-diagnostic pattern:
observable cleanup opportunity, no vendor removal contract, path-free
direction only.

A future Clean category would require a NVIDIA-published contract (official
cleanup API, stable state schema, or a support article authorizing external
deletion of completed packages). Present evidence does not provide one.

## Evidence classes

- **Proven**: stated by NVIDIA public documentation, or directly declared by a
  version-identified NVIDIA-signed installed artifact.
- **Observed**: read-only inspection of one research host; useful for fixtures
  and exclusions, but not a cross-version product contract.
- **Unknown**: no NVIDIA first-party contract was found.

No forum, blog, cleaner database, or community answer is used as product
authority.

## NVIDIA public documentation

### Re-downloadability is proven, directory safety is not

NVIDIA states that drivers are available through NVIDIA App and GeForce.com,
and recommends NVIDIA App for driver downloads. NVIDIA also provides manual
driver search and an archive of older drivers. These facts prove that a driver
package is generally re-downloadable; they do not identify `ota-artifacts` as
a safe local cache directory or define a completed-download marker for the
OTA framework.

Sources:

- [NVIDIA App FAQ](https://nvidia.custhelp.com/app/answers/detail/a_id/5521/~/nvidia-app-faq)
- [Downloading Drivers](https://www.nvidia.com/en-us/drivers/downloading-drivers/)
- [NVIDIA Drivers](https://www.nvidia.com/en-us/drivers/)

NVIDIA App also supports rollback to previously installed drivers. NVIDIA does
not state whether that feature depends on packages retained under
`ota-artifacts`, so Foal must not claim that removing this directory preserves
all offline rollback behavior. Source:
[NVIDIA App FAQ](https://nvidia.custhelp.com/app/answers/detail/a_id/5521/~/nvidia-app-faq).

### NVIDIA does not publish an OTA-artifacts removal contract

No NVIDIA first-party documentation was found that:

- names the `ota-artifacts` directory or the `UpdateFramework` tree;
- authorizes external deletion of completed OTA packages;
- publishes the numeric mapping of the `state` field in
  `status\<component>\download.json`;
- identifies the exhaustive set of processes or services that may write the
  tree;
- exposes a supported cleanup API comparable to the internal
  `CleanupOldArtifcacts` implementation.

NVIDIA's installer troubleshooting guide instructs users to stop NVIDIA-named
services and end processes beginning with `nv` or `NVIDIA` before retrying a
failed install. That supports treating active NVIDIA software as a conflict
but does not authorize external cleanup of `ota-artifacts`. Source:
[Solving NVIDIA Installer Issues](https://nvidia.custhelp.com/app/answers/detail/a_id/4223/~/solving-nvidia-installer-issues).

### `Installer2` is a different lifecycle and must remain excluded

NVIDIA identifies
`C:\Program Files\NVIDIA Corporation\Installer2` as an installer cache used
for complete reinstall, uninstall, OS/device-initiated driver rollback, and
add-on version matching. That evidence does **not** apply to the ProgramData
`ota-artifacts` root. A resolver must never broaden into `Installer2` or use
its documentation to justify deleting `ota-artifacts`.

Source:
[Disk Space Used When Installing NVIDIA Drivers](https://nvidia.custhelp.com/app/answers/detail/a_id/3333/~/disk-space-used-when-installing-nvidia-drivers).

## NVIDIA-signed installed-artifact evidence

### The `ota-artifacts` root is hardcoded by NVIDIA-signed code

The research host runs NVIDIA App 11.0.7.247. Its NVIDIA-signed
`UpdateFrameworkPlugin.dll` (FileVersion 11.0.7.247, CompanyName NVIDIA
Corporation, located at
`C:\ProgramData\NVIDIA Corporation\NVIDIA app\UpdateFramework\plugins\UpdateFrameworkPlugin.dll`)
contains the literal string `ota-artifacts`, proving that the directory name
is a vendor-fixed location, not a coincidence.

### NVIDIA has an internal `CleanupOldArtifcacts` capability for this tree

The NVIDIA-signed `Downloader.dll` 11.0.7.247 at
`C:\Program Files\NVIDIA Corporation\NVIDIA app\Plugins\localuser\NvApp\Downloader.dll`
contains the following first-party strings (verbatim, including the typo
`Artifcacts`):

```text
CleanupOldArtifcacts
CleanupOldArtifcacts for
Actually starting cleanup for component
Cleanup started
Cleanup finished successfully for component
Cleanup failed for component
Cleanup failed error
Cleanup cancelled before starting for component
Cleanup was interrupted by shutdown for component
Cleanup task already active. RESULT_INVALID_OPERATION for
Cleanup timeout exceeded after DownloadManager
Cleanup timeout exceeded after PostProcessingManager
Cleanup timeout exceeded after Scheduler
Cleanup timeout exceeded after UpdateCheckManager
Cleanup timeout exceeded after TelemetryObject
```

The same DLL contains strings proving cleanup deletes both downloaded and
extracted packages:

```text
Cleanup interrupted during downloaded package deletion for version
Cleanup interrupted during extracted package deletion for version
Cleanup interrupted by shutdown request after extracted package deletion.
Cleanup interrupted during pending directory deletion
Cleanup interrupted during pending directory processing
Failed to remove directory
Failed to remove root directory
Error searching for pending cleanup directories
Found pending cleanup directory
Created cleanup pending marker
Removed cleanup pending marker
Failed to create cleanup marker for
Failed to remove cleanup marker for
Remaining items will be cleaned on next opportunity.
Removing stale task
Removing stale UpdateChecker
Failed to delete the folder for download task
```

This is strong first-party evidence that NVIDIA itself owns cleanup of this
directory: it deletes downloaded `.exe` packages, deletes extracted
post-processing trees, uses on-disk cleanup-pending markers, resumes across
shutdown, removes stale tasks and stale update checkers, and can fail in the
process. It is **not** a public contract. The capability is invoked by
NVIDIA's own scheduler under internal triggers, not by an external API, and
NVIDIA does not document the trigger conditions, retention policy, or a safe
external deletion procedure.

This evidence was anticipated by the legacy Downloader research note, which
listed "a vendor-supported cleanup capability comparable to its internal
`CleanupOldArtifacts` implementation" as a permanent-delete eligibility gap.
The current OTA framework has the same shape: internal cleanup exists, public
contract does not.

### Download-state persistence exists, but the numeric mapping is not published

`Downloader.dll` declares a `DownloadState` enum and emits log strings that
reveal the following state names (verbatim):

```text
DS_DOWNLOADING
DS_FINISHED
DS_PAUSED
DS_NETWORK_ERROR_RETRYING
DS_NETWORK_ERROR_RETRY_PAUSED
```

The same DLL also contains the strings `DownloadTriggered`, `Downloading`,
`VerifyingChecksum`, `VerifyingSignature`, `Paused`, `Cancelled`, `Retrying`,
`Finished`, `Failed`, `Completed`, `Unknown`, and `Pending`. The full enum
membership is not exhaustively enumerated by string extraction alone, and
**NVIDIA does not publish the numeric mapping**. The persisted
`status\<component>\download.json` records use numeric `state` values (the
research host observed `state=8` and `state=14`). Foal cannot safely equate
any observed numeric value with `DS_FINISHED` or any other terminal state
without a published mapping, and cannot infer that a "Finished" download is
no longer part of a pending install or self-update workflow.

The same DLL also contains:

```text
Download persistence has gone in bad state. This may result in download progress going beyond 100
```

This is a first-party admission that the persisted `download.json` state can
become inconsistent with the actual download. Any external resolver that
treats the JSON as authoritative can act on stale or corrupted state.

### Post-processing trees are the active self-update payload

`status\<component>\postprocessing.json` records an `extract` action whose
`output` is `<ota-artifacts>\<component>\post-processing\<hash>\setup.exe`.
`status\nvapp\nvAppUpdate.json` then names that `setup.exe` with installer
arguments:

```text
"url":"C:\\ProgramData\\NVIDIA Corporation\\NVIDIA App\\UpdateFramework\\ota-artifacts\\nvapp\\post-processing\\<hash>\\setup.exe"
"installer_args":"-custominvokerid:8 -selfupdate -loglevel:6 -log:\"C:\\ProgramData\\NVIDIA Corporation\\NVIDIA App\\Installer\\Logs\""
```

The `-selfupdate` argument proves that the extracted post-processing tree is
the active NVIDIA App self-update payload, not an orphan cache. Deleting it
during a pending self-update can break the user's app upgrade. The same
pattern applies to `grd` driver packages: the extracted `setup.exe` is the
installer NVIDIA App will invoke to install the downloaded driver.

### Path spelling is not a stable contract

The persisted JSON uses `NVIDIA app` (lowercase `app`) and `NVIDIA App`
(capitalized `App`) interchangeably when naming the same
`C:\ProgramData\NVIDIA Corporation\NVIDIA app\UpdateFramework\ota-artifacts`
path. Windows path semantics make this resolve to the same directory, but the
inconsistency is direct first-party evidence that NVIDIA's own code does not
treat the path string as a normalized contract. An external resolver that
matches paths case-sensitively, or that expects a single canonical spelling,
can mismatch.

## Research-host observations

Observed read-only on 2026-07-21. These facts are fixtures, not universal
layout guarantees.

### Root shape and space

```text
C:\ProgramData\NVIDIA Corporation\NVIDIA app\UpdateFramework\ota-artifacts
```

Total observed size: 4.5 GiB.

| Child | Shape / observed size | Interpretation |
| --- | ---: | --- |
| `grd\cf30e8fa…\610.47-…exe` | one signed EXE, 791,230,752 B | completed display-driver download (state=8) |
| `grd\post-processing\cf30e8fa…\` | extracted tree, ~3.0 GiB, 466 files | active driver installer payload (`setup.exe` present) |
| `grd\6c19a90f…\` | empty directory | state=14, `failureCount=3`; metadata references a `fileLocation` whose file is gone |
| `nvapp\1504eac4…\NvidiaAppSetupInt_x86_11.0.8.299_OFFICIAL_2007D0.exe` | one signed EXE, 176,843,352 B | completed NVIDIA App self-update download (state=8) |
| `nvapp\post-processing\1504eac4…\` | extracted tree, ~676 MiB, 814 files | active self-update payload (`setup.exe`, referenced by `nvAppUpdate.json`) |
| `crd\`, `default\` | each only contains an empty `post-processing\` | component scaffolding with no current artifact |
| 8 UUID-named root children (e.g. `0531d9f7-…`) | each only contains an empty `post-processing\` | orphaned schema from an earlier UpdateFramework generation |

The 8 UUID-named root subdirs (dated Nov 2024 through Feb 2025) and the
named-component schema (`nvapp`, `grd`, `crd`, `default`) coexist on the same
host. That coexistence is direct evidence that the layout schema has already
changed once within the installed base, so the current named-component shape
must not be treated as a timeless NVIDIA contract.

### Status records

`status\grd\download.json` contained two records:

- `state=14`, `percentComplete=100`, `failureCount=3`, `taskId=6c19a90f…`,
  `fileLocation` pointing to a file that no longer exists on disk.
- `state=8`, `percentComplete=100`, `failureCount=0`, `taskId=cf30e8fa…`,
  `fileLocation` pointing to the present 791 MiB `610.47` EXE.

`status\nvapp\download.json` contained one record:

- `state=8`, `percentComplete=100`, `failureCount=0`, `taskId=1504eac4…`,
  `fileLocation` pointing to the present 176 MiB `NvidiaAppSetupInt` EXE.

`status\nvapp\autoupdate.json` recorded `updateState=2, currentPhase=5` for
the same task. `status\nvapp\updatecheck.json` was modified during the
research session at 2026-07-21 18:21:50 local time and references version
`11.0.8.299`, the same version as the downloaded package. `status\nvapp\postprocessing.json`
recorded `status=1` with an `extract` action whose action status is `2`.

This proves three things. First, `state=8` co-occurs with a present file and
`state=14` co-occurs with a missing file on this host, but the numeric
mapping is not published, so the same numbers cannot be trusted to mean the
same thing across NVIDIA App versions. Second, NVIDIA's own state persistence
can reference paths whose files have already been removed, so an external
resolver cannot trust that "file present + status record present" means
"NVIDIA considers this package live," nor that "file absent" means "NVIDIA
considers this package abandoned." Third, the tree was being actively written
during the research session, which rules out treating `ota-artifacts` as an
idle cache.

### Component catalog and registry

`UpdateFramework\profile-catalog\component_profiles.json` declares per-component
downloader profiles, trusted signers, checksums, retry policy, and
`maxDaysBetweenReleases` (a release-check cadence, not a retention bound).
`UpdateFramework\registry\nvapp.json` records the OTA channel (`official`) and
`firstBootTime`. None of these files publishes a removal contract, a retention
TTL, or a safe external deletion procedure.

### Ownership and elevation

On this host the `ota-artifacts` root owner was the current user; SYSTEM,
Administrators, and the current user had full control. Deleting a leaf
directory therefore did not inherently require UAC on this host. ACLs can
vary. Foal must use its normal non-elevated execution and report permission
failures; even if a future category were proven, this tree would not be
evidence for automatic elevation.

## Process and service gate

The research host ran the following NVIDIA processes during inspection with
no download in progress:

- `nvcontainer.exe` (multiple instances, services and console sessions)
- `NVDisplay.Container.exe`
- `NVIDIA Overlay.exe` (multiple instances)
- `nvsphelper64.exe`

The following NVIDIA services were registered:

- `NvContainerLocalSystem` (NVIDIA LocalSystem Container) - auto-start,
  runs as LocalSystem, binary
  `C:\Program Files\NVIDIA Corporation\NvContainer\nvcontainer.exe`.
- `NVDisplay.ContainerLocalSystem` (NVIDIA Display Container LS) - auto-start,
  runs as LocalSystem, binary under
  `C:\WINDOWS\System32\DriverStore\FileRepository\nv_dispi.inf_…\Display.NvContainer\NVDisplay.Container.exe`.

NVIDIA security bulletins establish that `NVIDIA Web Helper.exe` belongs to
GeForce Experience and that `nvcontainer.exe` is used by GeForce Experience
services:

- [NVIDIA Web Helper security bulletin](https://nvidia.custhelp.com/app/answers/detail/a_id/4279/)
- [GeForce Experience / nvcontainer security bulletin](https://nvidia.custhelp.com/app/answers/detail/a_id/5076/)

The mere existence of any NVIDIA process is too broad to mean "download
active," but the available first-party evidence does not identify an
exhaustive writer set for the OTA tree. The `ota-artifacts` root and the
`status\` tree can be written by `NvContainerLocalSystem` (running as
LocalSystem) and by NVIDIA App processes in the user session; an external
non-elevated resolver cannot reliably attribute idle state. Foal must never
stop these processes or services for Clean.

## Maximum safe resolver supported by present evidence

None. Present evidence does not support any executable resolver against any
sub-scope of `ota-artifacts`. Specifically, all of the following proposed
scopes are rejected:

1. **Whole `ota-artifacts` root** - rejected: contains active self-update
   payload, status, catalog, and registry state owned by NVIDIA App.
2. **Named-component subdirs (`nvapp`, `grd`, `crd`, `default`)** - rejected:
   each contains or can contain active download and post-processing state.
3. **Hash-named leaf dirs with a completed-download EXE** - rejected: NVIDIA
   does not publish the numeric `state` mapping, and `state=8` cannot be
   safely equated to `DS_FINISHED` across versions; the same record can
   describe a package that is part of a pending install or self-update.
4. **Hash-named extracted post-processing trees** - rejected: `nvAppUpdate.json`
   directly names `<post-processing>\<hash>\setup.exe` with `-selfupdate`
   arguments, proving these trees are the active installer payload.
5. **Orphan UUID subdirs at the `ota-artifacts` root** - not proven: their
   emptiness is observed, not contracted; NVIDIA's own stale-task removal may
   repopulate or remove them at any time, and an external resolver cannot
   distinguish "NVIDIA has abandoned this" from "NVIDIA will clean this on
   next opportunity."
6. **`status\`, `profile-catalog\`, `registry\`, `plugins\` siblings** -
   excluded: metadata, configuration, and vendor code; never candidates.

Because NVIDIA's own `CleanupOldArtifcacts` implementation is the only
evidenced cleanup capability and is not exposed as a supported external API,
no gate sequence, age threshold, filename pattern, status-field check, or
process-name test can promote any of these scopes to a Foal deletion
candidate. A user confirmation likewise cannot substitute for the missing
vendor contract.

## Permanent-delete and Recycle-Bin eligibility gaps

The following remain **unknown** from first-party sources:

- a NVIDIA-published statement that any part of `ota-artifacts` may be
  removed externally;
- a stable cross-version schema contract for the named-component layout
  (`nvapp`, `grd`, `crd`, `default`) and the legacy UUID layout that
  co-exists with it;
- the numeric mapping of the `state` field in `download.json`, including
  whether `state=8` corresponds to `DS_FINISHED` across all NVIDIA App
  releases;
- the meaning of `state=14` and other observed non-8 values;
- the trigger conditions, retention policy, and scheduling of NVIDIA's
  internal `CleanupOldArtifcacts` capability;
- whether completed packages participate in NVIDIA App's offline rollback or
  pending-install UX;
- whether the `updateState` / `currentPhase` values in `autoupdate.json`
  identify a pending self-update that would break if the extracted
  `setup.exe` were removed;
- an exhaustive list of processes and services that can write the tree;
- a supported "download/post-processing idle" API;
- a vendor-supported cleanup capability comparable to the internal
  `CleanupOldArtifcacts` implementation.

Because these gaps concern state, ownership, and lifecycle - not merely path
spelling - the tree cannot be promoted to `move_to_recycle_bin` or
`delete_permanently` by adding age thresholds, filename patterns, process-name
guesses, status-field checks, or a user confirmation.

## User impact statement (Review clue)

Recommended path-free wording, aligned with the WeChat backup, WPS cloud, and
OneDrive local-diagnostic Review clues:

> NVIDIA App stores downloaded driver and application update packages under
> its own UpdateFramework cache. NVIDIA App manages this cache internally and
> may re-download packages on demand. Foal does not delete this cache because
> NVIDIA does not publish a supported removal contract. Use NVIDIA App's own
> Settings > Updates interface to manage updates, or download drivers
> directly from nvidia.com/drivers.

Do not promise that rollback is unaffected, do not name the on-disk path in
user-facing copy, do not call the deletion secure erasure, and do not
conflate this tree with `Installer2` or with the legacy `Downloader` root.
