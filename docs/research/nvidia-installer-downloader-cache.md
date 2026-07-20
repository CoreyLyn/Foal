# NVIDIA installer downloader cache eligibility

Status: researched and product decision accepted 2026-07-20. This note evaluates only
`C:\ProgramData\NVIDIA Corporation\Downloader`. It uses NVIDIA first-party
support material and read-only evidence from NVIDIA-signed software installed
on the research host. It does not register a Clean category or authorize a
delete action.

## Decision

| Proposed scope | Evidence result | Maximum defensible action |
| --- | --- | --- |
| Entire `Downloader` root | **Rejected** | none |
| Every 32-hex child directory | **Not proven** | none without state validation |
| Strictly validated completed download-task directories | **Observed, not a stable vendor contract** | `move_to_recycle_bin`, opt-in |
| `latest`, `PostProcessing`, metadata, or `Installer2` | **Excluded** | none |

The requested `delete_permanently / Proven` classification is **not supported**.
NVIDIA proves that drivers can be downloaded again, and a signed legacy
GeForce Experience module defines `status = 2` as `COMPLETED`. NVIDIA does not
publish a stable contract for this directory, however. The installed host also
contains two materially different generations of the update framework, with
different roots and persisted state schemas. A cleanup implementation cannot
safely assume that one legacy numeric status enum applies to every supported
NVIDIA App / GeForce Experience version.

The accepted product policy is an opt-in **Not proven** category using
`move_to_recycle_bin`, with the narrow resolver and fail-closed gates in this
note. Permanent deletion remains ineligible unless future evidence establishes
a stable vendor layout/status contract or a supported NVIDIA cleanup API.

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
package is generally re-downloadable; they do not identify a safe local cache
directory or a completed-download marker.

Sources:

- [NVIDIA App FAQ](https://nvidia.custhelp.com/app/answers/detail/a_id/5521/~/nvidia-app-faq)
- [Downloading Drivers](https://www.nvidia.com/en-us/drivers/downloading-drivers/)
- [NVIDIA Drivers](https://www.nvidia.com/en-us/drivers/)

NVIDIA App also supports rollback to previously installed drivers. NVIDIA does
not state whether that feature depends on a package retained under
`Downloader`, so Foal must not claim that removing this directory preserves all
offline rollback behavior. Source:
[NVIDIA App FAQ](https://nvidia.custhelp.com/app/answers/detail/a_id/5521/~/nvidia-app-faq).

### Downloads and installs have non-terminal states

NVIDIA documents corrupted downloads, installation failures, temporary driver
files, UAC, and interference from background applications or Windows Update.
Therefore a version-looking directory, an old timestamp, or a signed `.exe`
alone does not prove a completed and inactive task.

Sources:

- [7-Zip Error Message When Installing NVIDIA Display Driver/NVIDIA App](https://nvidia.custhelp.com/app/answers/detail/a_id/21/~/7-zip-error-message-when-installing-nvidia-display-driver/nvidia-app)
- [NVIDIA App driver installation failed: manual clean install](https://nvidia.custhelp.com/app/answers/detail/a_id/10/~/nvidia-app-driver-installation-failed.-how-do-i-manually-clean-install-the)

### `Installer2` is a different lifecycle and must remain excluded

NVIDIA identifies
`C:\Program Files\NVIDIA Corporation\Installer2` as an installer cache used for
complete reinstall, uninstall, OS/device-initiated driver rollback, and add-on
version matching. That evidence does **not** apply to the ProgramData
`Downloader` root. A resolver must never broaden into `Installer2` or use its
documentation to justify deleting `Downloader`.

Source:
[Disk Space Used When Installing NVIDIA Drivers](https://nvidia.custhelp.com/app/answers/detail/a_id/3333/~/disk-space-used-when-installing-nvidia-drivers).

## NVIDIA-signed installed-artifact evidence

### Legacy GeForce Experience status enum

The research host retains an NVIDIA-distributed GeForce Experience 3.19.0.107
payload at:

```text
C:\ProgramData\NVIDIA Corporation\Downloader\PostProcessing\GFE\
  920c23fb7471819f37d81760e38997e7\nodejs\
```

Its vendor JavaScript `downloader.js` declares:

```text
UNDEFINED=-1, DOWNLOADING=0, PAUSED=1, COMPLETED=2, RETRYING=3,
PAUSED_FOR_FAILED=4, STOPPED_FOR_FAILED=5,
CHECKSUM_VERIFY_FAILS=6, SIGNATURE_VERIFICATION_FAILS=7,
DISK_WRITE_FAIL=8, DOWNLOAD_ERROR=9
```

`NvAutoDriverDownload.js` repeats the core mapping. `NvAutoDownload.js`
triggers extraction/post-processing only after `status == COMPLETED`, proving
for this specific legacy generation that `2` means download completion, not
download in progress.

This is strong version-specific first-party evidence. It is **not** a public or
stable compatibility contract. In particular, completion of the download is
separate from completion of extraction, post-processing, installation, or a
user's pending install choice.

### Current NVIDIA App uses a different persisted model

The installed NVIDIA App 11.0.7.247 has an NVIDIA-signed
`Downloader.dll` under:

```text
C:\Program Files\NVIDIA Corporation\NVIDIA app\
  Plugins\localuser\NvApp\Downloader.dll
```

The DLL embeds a versioned API schema with download states including
`DownloadTriggered`, `Downloading`, `VerifyingChecksum`,
`VerifyingSignature`, `Paused`, `Cancelled`, retry states, `Finished`, and
verification/write failures. It also exposes separate post-processing states.
Its current persisted data is stored under a different root:

```text
C:\ProgramData\NVIDIA Corporation\NVIDIA app\UpdateFramework\
  ota-artifacts\...
  status\<component>\download.json
  status\<component>\postprocessing.json
```

Observed current records use `state`, byte counters, HTTP result, verification
duration, component identity, and separate post-processing actions. The profile
catalog specifies trusted signers, checksums, retries, and component-specific
download configuration.

This proves that current NVIDIA software has richer active state and that the
legacy `Downloader\status.json` layout must not be treated as a timeless NVIDIA
contract. It also means the newer `NVIDIA app\UpdateFramework` tree is outside
the proposed category and must not be discovered by sibling or vendor-parent
scanning.

## Research-host observations

Observed read-only on 2026-07-20. These facts are fixtures, not universal
layout guarantees.

### Root shape and space

```text
C:\ProgramData\NVIDIA Corporation\Downloader
```

| Child | Shape / observed size | Interpretation |
| --- | ---: | --- |
| `62fec250047846ce8e4aa1d21192b479` | one signed EXE, 153,886,600 B | NVIDIA App update task |
| `c1341b0ededc2dcc770c70a1bfda183e` | one signed EXE, 584,133,640 B | display-driver task |
| `e7ec208bf0c7aa531556e7aa49b84dc7` | one signed EXE, 131,930,520 B | GeForce Experience update task |
| `latest` | extracted tree, 601,774,748 B | active self-update handoff; excluded |
| `PostProcessing` | extracted tree, 515,776,162 B | post-processing/back-up state; excluded |
| `config` | metadata | excluded |
| `gfeupdate.json` | metadata | excluded |
| `status.json` | task registry | excluded |

`gfeupdate.json` directly names `latest\setup.exe`, so `latest` cannot be
treated as an orphan cache. `PostProcessing\postprocessing_status.json` only
contains `[{"packageType":2}]` and does not give
a usable completed-state contract.

### Legacy task registry

`status.json` contained 86 task records:

- 84 with `status = 2`;
- one with `status = -1`;
- one with `status = 11`.

All three task directories still present matched an exact `taskId` record with
`status = 2`. The two non-2 records had no corresponding directory. Each
present directory contained exactly the `fileLocation` named by its record.

All three EXEs had a valid Authenticode signature whose signer subject was
NVIDIA Corporation. The display-driver record supplied an MD5 checksum and its
file matched it exactly. The two application-update records supplied no
checksum, although their Authenticode signatures were valid.

This is consistent with completed downloads and with the legacy enum. It does
not prove that future or other installed NVIDIA versions use the same schema,
nor that a completed package is no longer part of a pending install or offline
rollback workflow.

### Ownership and elevation

On this host the root owner was the current user; SYSTEM, Administrators, and
the current user had full control, while Builtin Users had write permission.
Deleting a validated task directory therefore did not inherently require UAC
on this host. ACLs can vary. Foal must use its normal non-elevated execution and
report permission failures; this category is not evidence for automatic
elevation.

## Process and service gate

NVIDIA's installer troubleshooting guide instructs users to stop NVIDIA-named
services and end processes beginning with `nv` or `NVIDIA` before retrying a
failed install. That supports treating active NVIDIA software as a conflict,
but does not prove that idle status alone makes a folder deletable. Foal must
not stop those processes or services for Clean.

Source:
[Solving NVIDIA Installer Issues](https://nvidia.custhelp.com/app/answers/detail/a_id/4223/~/solving-nvidia-installer-issues).

NVIDIA security bulletins establish that `NVIDIA Web Helper.exe` belongs to
GeForce Experience and that `nvcontainer.exe` is used by GeForce Experience
services:

- [NVIDIA Web Helper security bulletin](https://nvidia.custhelp.com/app/answers/detail/a_id/4279/)
- [GeForce Experience / nvcontainer security bulletin](https://nvidia.custhelp.com/app/answers/detail/a_id/5076/)

The research host had `nvcontainer`, `NVDisplay.Container`, `NVIDIA Overlay`,
and `nvsphelper64` processes while no download was observed. Therefore the mere
existence of any NVIDIA process is too broad to mean “download active,” but the
available first-party evidence does not identify an exhaustive writer set or a
safe narrower gate.

For a Not-proven implementation, the conservative gate is:

1. detect NVIDIA App / GeForce Experience update-related processes and NVIDIA
   services before inspection;
2. never stop them;
3. if state attribution is incomplete or unknown, skip;
4. inspect and validate the exact task;
5. repeat the process/service and task-state checks immediately before action;
6. require no recent write and identical metadata before/after inspection.

An all-NVIDIA-process gate will produce frequent false skips. A shorter list
such as only `nvcontainer.exe` and `NVIDIA Web Helper.exe` risks missing a
writer. This remains an unresolved product trade-off, not a proven safety gate.

## Maximum safe resolver supported by present evidence

If the product chooses `move_to_recycle_bin / Not proven`, use all of these
conditions. Failure or uncertainty at any step emits no candidate.

1. Resolve only the fixed machine path
   `C:\ProgramData\NVIDIA Corporation\Downloader`; do not search other drives,
   environment overrides, registry paths, or NVIDIA parent directories.
2. Never candidate the root itself.
3. Allow only immediate child names matching exactly 32 lowercase or
   case-insensitive hexadecimal characters.
4. Parse a bounded, valid root `status.json`; require exactly one matching
   record whose `taskId` equals the child name.
5. Require the legacy, version-specific completed state `status == 2`.
   Unknown fields, duplicate IDs, unknown schema, and every other status skip.
6. Initially restrict to `downloadType == 1` (display-driver download). Exclude
   self-update and differential/update types because their extraction and
   backup lifecycle is separate and less constrained.
7. Require a non-empty NVIDIA HTTPS URL from a fixed NVIDIA-owned download
   host, a non-empty version, a non-empty checksum, and one `fileLocation`.
8. Canonicalize `fileLocation`; require it to be a direct regular-file child of
   the task directory. Reject traversal, alternate streams, hard-link count
   greater than one, and every reparse point in the chain.
9. Require the task directory to contain exactly that one regular file; no
   subdirectories or extra state.
10. Verify the declared checksum and a valid Authenticode signature chaining to
    NVIDIA Corporation. Signature/checksum failure skips; neither turns an
    unknown status into a completed task.
11. Exclude a task when `extractedPath` exists, when it is referenced by
    `gfeupdate.json`, or when any post-processing/current-update record names
    the task.
12. Apply Protection, idle-before-and-after, recent-write, stable metadata,
    fresh resolution, containment, immediate revalidation, and normal Recycle
    Bin capacity checks.
13. Never fall back from Recycle Bin failure to permanent deletion.

These guards deliberately yield no candidate for the two observed
application-update directories because their checksum field is empty. They can
yield the observed 566.03 driver package only if all live gates pass.

## Permanent-delete eligibility gaps

The following remain **unknown** from first-party sources:

- a stable cross-version contract for `Downloader\status.json`;
- whether `status = 2` is supported across all relevant GeForce Experience and
  NVIDIA App releases rather than only the inspected legacy generation;
- an official statement that completed task directories may be removed;
- an exhaustive list of processes/services that can write the legacy tree;
- a supported “download/post-processing idle” API;
- whether retained completed packages participate in NVIDIA App's offline
  rollback or pending-install UX;
- retention policy and cleanup ownership for `latest` and `PostProcessing`;
- a vendor-supported cleanup capability comparable to its internal
  `CleanupOldArtifacts` implementation.

Because these gaps concern state and ownership—not merely path spelling—the
category cannot be promoted to `delete_permanently` by adding age thresholds,
filename patterns, process-name guesses, or a user confirmation.

## User impact statement for a future Not-proven category

Recommended wording:

> Removes a verified completed NVIDIA driver download from the Recycle Bin
> candidate set. The driver can normally be downloaded again, but offline
> install or rollback may require a new download. NVIDIA update activity is
> never stopped; uncertain or active tasks are skipped.

Do not promise that rollback is unaffected, do not call the deletion secure
erasure, and do not conflate this directory with `Installer2`.
