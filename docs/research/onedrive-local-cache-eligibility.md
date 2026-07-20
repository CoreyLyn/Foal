# OneDrive local cache eligibility

Status: researched and executable category rejected 2026-07-20. This note evaluates
only the current user's `%LOCALAPPDATA%\Microsoft\OneDrive` tree. It uses
Microsoft first-party documentation and bounded, read-only observations from a
Microsoft-signed OneDrive 26.123 installation. It does not register a Clean
category or authorize deletion.

## Decision

The proposed broad `onedrive_cache` category is **not executable in this
slice**. The accepted product decision is to omit it from the catalog, policy
matrix, group tokens, and Clean TUI, retaining only a path-free recommendation
to use OneDrive Free up space or Storage Sense.
No exact path set under `%LOCALAPPDATA%\Microsoft\OneDrive` has both:

1. a stable Microsoft-owned layout contract; and
2. Microsoft evidence that an external cleaner may remove it without deleting
   account, sync, updater, or diagnostic state.

| Proposed scope | Evidence result | Maximum defensible action |
| --- | --- | --- |
| Entire OneDrive local root | **Rejected** | none |
| `settings\**` | **Permanently excluded by product decision** | none |
| `ListSync\**` | Observed state/configuration, not proven cache | none |
| `Update\**`, `StandaloneUpdater\**`, `setup\**` | Observed updater/installer state | none |
| `logs\**` | Mixed logs, databases, key stores, and metadata | none |
| Exact `aria-debug` OneDrive log files | Microsoft says they are safe to delete, but does not publish a fixed local layout or filename grammar | no resolver yet |
| `EBWebView\**` | Mixed WebView2 profile data; direct filesystem deletion is not the supported selective-clear API | none |
| Synced OneDrive folders / Cloud Files placeholders | User content whose deletion can propagate to the cloud | none; use Files On-Demand / Storage Sense |

The requested alternative `delete_permanently / Proven` is therefore not
supported. `move_to_recycle_bin` does not fix uncertain ownership or state: it
would still mutate live OneDrive data and can impair diagnostics, updates, or
account/sync operation.

## Evidence classes

- **Proven**: explicitly stated in Microsoft Support / Microsoft Learn, or
  directly identified by a versioned Microsoft-signed installed artifact.
- **Observed**: read-only evidence from one host; useful for exclusions and
  fixtures, but not a compatibility contract.
- **Unknown**: no first-party contract was found.

Microsoft Q&A community answers and third-party cleanup lists are not used as
product authority.

## Microsoft public documentation

### Reset is a supported operation, not deletion authority

Microsoft says OneDrive reset disconnects all sync connections, performs a
full sync, deletes a DAT file, persists reset information in the registry,
keeps settings on disk, and rebuilds the DAT file after restart. It can also
require folder selections to be made again. This proves that reset has
application-owned semantics and side effects; it does not authorize Foal to
approximate reset by deleting `settings`, databases, or the local root.

Source: [Reset OneDrive](https://support.microsoft.com/en-us/onedrive/reset-onedrive).

Microsoft's unlink/re-link instructions also identify
`%LOCALAPPDATA%\Microsoft\OneDrive\settings` and one narrowly named
`PreSignInSettingsConfig.json` as sign-in state. This reinforces the accepted
rule that the entire `settings` subtree is excluded. A troubleshooting step
for one credential problem is not a general cleanup contract.

Source: [Unlink and re-link OneDrive](https://support.microsoft.com/en-us/onedrive/unlink-and-re-link-onedrive).

### Only `aria-debug` has an explicit safe-delete statement

Microsoft Support states that OneDrive log files titled `aria-debug` can be
safely deleted. The same page does not specify a Windows root, extension,
anchored pattern, account subtree, age rule, or whether every similarly named
file belongs to OneDrive. That is enough to reject a claim that *all* OneDrive
logs are required, but not enough for Foal's exact, fail-closed resolver.

Source: [Troubleshoot OneDrive sync stuck on Processing Changes](https://support.microsoft.com/en-us/onedrive/troubleshoot-onedrive-sync-issues-stuck-on-processing-changes).

No Microsoft first-party source found during this research says that `.odl`,
`.odlgz`, `.odlsent`, `.aodl`, `.otc`, their WAL/SHM files, or the entire
`logs` directory may be deleted. Filename extensions, age, compression, and
successful upload do not establish safe deletion.

### Local logs have diagnostic value

Microsoft documents OneDrive diagnostic data as data used to find and fix
problems, identify and mitigate threats, and improve the experience. OneDrive
sync health reporting likewise exists to diagnose sync errors and device
health. These sources do not define local file retention, but they establish a
real diagnostic impact: removing undocumented logs can reduce evidence for a
support investigation. Foal must disclose that impact if a future exact log
rule is added.

Sources:

- [Diagnostic data in Microsoft 365](https://support.microsoft.com/en-us/privacy/diagnostic-data-in-microsoft-365)
- [OneDrive sync reports in the Apps Admin Center](https://learn.microsoft.com/en-us/sharepoint/sync-health)

### WebView2 data is mixed and has a supported API boundary

Microsoft defines a WebView2 user data folder as a mix of cookies,
permissions, cached resources, local storage, IndexedDB, settings, and other
profile data. Microsoft recommends the Clear Browsing Data API to selectively
clear `DiskCache` and requires the WebView2 session and browser processes to
end before deleting a whole user data folder. It also advises persisting a UDF
when an app reuses the user's data across sessions.

The observed OneDrive `EBWebView` tree contains both cache-looking children and
state such as `Default`, `Local State`, security component data, and metrics.
Foal cannot infer that direct deletion of Chromium-looking subdirectories is
equivalent to OneDrive invoking WebView2's profile API. The whole tree and all
direct-path subsets remain excluded. A future supported action would need an
application/API contract, not an allowlist copied from a browser cache rule.

Sources:

- [Manage WebView2 user data folders](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/user-data-folder)
- [Clear browsing data from a WebView2 user data folder](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/clear-browsing-data)

### Cloud Files content is outside this category

Microsoft says deleting an online-only file locally deletes it from OneDrive
on all devices and online. The supported way to reclaim hydrated content while
retaining the cloud file is OneDrive's **Free up space** operation or Windows
Storage Sense, which makes eligible locally available files online-only and
preserves files marked always available.

Therefore `onedrive_cache` must never discover a configured sync root, a Known
Folder Move root, or a Cloud Files placeholder. Recycle Bin is not a safety
boundary for a deletion that OneDrive may synchronize to the service.

Sources:

- [Save disk space with OneDrive Files On-Demand](https://support.microsoft.com/en-us/onedrive/save-disk-space-with-onedrive-files-on-demand-for-windows)
- [Use OneDrive and Storage Sense to manage disk space](https://support.microsoft.com/en-us/onedrive/use-onedrive-and-storage-sense-in-windows-10-to-manage-disk-space)
- [Sync files and folders with OneDrive](https://support.microsoft.com/en-us/onedrive/sync-your-computer-s-files-and-folders-with-onedrive)

## Research-host observations

Observed read-only on 2026-07-20. These are fixtures, not universal layout
guarantees. The installed OneDrive version was `26.123.0628.0001`; the inspected
executables had valid Authenticode signatures issued to Microsoft Corporation.

### Root shape and space

| Immediate child | Observed bytes | Safety interpretation |
| --- | ---: | --- |
| `logs` | 170,854,807 | mixed diagnostic/state tree; not a whole-root candidate |
| `settings` | 51,741,109 | account/sync state; permanently excluded |
| `ListSync` | 36,368,377 | contains account-named and `settings` children; excluded |
| `EBWebView` | 28,753,301 | mixed WebView2 UDF; excluded |
| `setup` | 979,866 | installer logs; excluded |
| `StandaloneUpdater` | 75,889 | update/config state; excluded |
| `Update` | 25,899 | live `update.xml`; excluded |

Running processes included `OneDrive.exe`, `OneDrive.Sync.Service.exe`, and
`FileCoAuth.exe`. Microsoft-signed installed binaries also included
`OneDriveStandaloneUpdater.exe` and `OneDriveUpdaterService.exe`.

### `logs` is not extension-homogeneous

The observed `logs` tree had immediate children `Common`, `ListSync`, `OD4`,
`Personal`, and `setup`. It contained 2,515 files, including:

| Extension | Count | Bytes |
| --- | ---: | ---: |
| `.odl` | 2,221 | 141,431,819 |
| `.odlsent` | 212 | 24,807,710 |
| `.otc-wal` / `.otc` / `.otc-shm` | 21 | 3,553,376 |
| `.odlgz` | 34 | 584,165 |
| `.aodl` | 8 | 472,718 |
| `.keystore`, `.json`, `.log`, `.txt`, `.dfd` | 19 | small but semantically mixed |

No `aria-debug`-named file was present. The oldest observed log dated from
December 2024 and the newest was being updated during research. This proves
that an age threshold could reclaim substantial space on this host; it does
not prove ownership is complete, state is terminal, or deletion is supported.

The `.otc` databases, WAL/SHM sidecars, `.keystore`, JSON, and unknown metadata
make an entire `logs` subtree candidate categorically unsafe. Even an
extension-only `.odl` resolver lacks Microsoft retention and safe-delete
evidence.

### Installer and browser-profile state are live

`Update\update.xml` was modified on the research date. `StandaloneUpdater`
contained `ECSConfig.json`, `PreSignInSettingsConfig.json`, and `Update.xml`.
The observed `EBWebView` root mixed cache-like children with `Default`,
`Local State`, certificate/security data, and component data. Directory names
alone cannot distinguish safely rebuildable bytes from live configuration.

## Fail-closed resolver boundary

### Current release

Do **not** register an executable `onedrive_cache` category. A path-free review
notice may recommend OneDrive **Free up space** or Storage Sense for hydrated
user content, but it must not report those bytes as Clean candidates and must
not call filesystem deletion.

The resolver must reject all of the following if the category is accidentally
registered or broadened:

- the OneDrive local root itself;
- `settings`, `ListSync`, `Update`, `StandaloneUpdater`, `setup`, and
  `EBWebView`, recursively;
- every synced user-content root and every Cloud Files placeholder;
- `.otc`, `.otc-wal`, `.otc-shm`, `.keystore`, JSON, and unknown files;
- `.odl`, `.odlgz`, `.odlsent`, `.aodl`, and generic `.log` based only on name,
  extension, age, sent-looking suffix, or compression;
- any path with a reparse point in the chain or a file with more than one hard
  link.

### Evidence needed to reopen execution

The narrowest plausible future resolver is exact `aria-debug` files under a
Microsoft-documented anchored local path. It requires first-party evidence for
the complete filename grammar and location, plus fixtures from supported
OneDrive versions. If those facts become available, the resolver should:

1. resolve only the current user's standard Local AppData path;
2. candidate individual regular files, never a directory or account subtree;
3. require an exact, anchored vendor-published filename grammar;
4. reject every reparse point in the path and hard-link count greater than one;
5. apply Protection before totals and again immediately before action;
6. require OneDrive, sync service, FileCoAuth, updater, and attributable
   WebView2 processes to be idle before inspection and immediately before
   mutation; unknown process state skips;
7. compare stable identity, size, and last-write metadata across fresh
   resolution and immediate validation;
8. never stop a process, elevate, or fall back from Recycle Bin failure to
   permanent deletion.

Microsoft's current wording supports safe deletion of `aria-debug` logs in
principle, but the unresolved path/name contract means neither
`move_to_recycle_bin` nor `delete_permanently` is presently eligible. The
action decision must be made only after the resolver is provable; a broad user
confirmation cannot supply missing vendor state.

## User impact wording for a future exact log rule

Recommended wording:

> Removes only Microsoft-documented disposable OneDrive diagnostic logs while
> OneDrive and its related processes are idle. Account settings, sync state,
> updater state, WebView profile data, and synced files are excluded. Removed
> logs may no longer be available to Microsoft Support for diagnosing past
> problems.

Do not call `.odl` files cache, promise that troubleshooting evidence is
unaffected, claim secure erasure, or imply that deleting Local AppData frees
hydrated OneDrive user content.
