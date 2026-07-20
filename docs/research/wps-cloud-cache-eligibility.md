# WPS cloud cache eligibility

Status: researched and executable category rejected 2026-07-20. This note evaluates the proposed
`wps_cloud_cache` Clean category for local content under a user's WPS cloud
drive. It uses WPS/Kingsoft first-party material, Microsoft Cloud Files API
documentation, and bounded read-only inspection of WPS-signed installed
artifacts. It does not inspect document names or contents, register a category,
or authorize a mutation.

## Decision

| Proposed scope | Evidence result | Maximum defensible action |
| --- | --- | --- |
| A `WPSDrive` tree or “local mirror” selected by path | **Rejected** | none |
| Files believed to have a cloud copy from names, icons, age, or location | **Not proven** | none |
| Ordinary files inside a WPS synchronized directory | **Deletion risk proven** | none |
| A WPS-owned Cloud Files placeholder proven in-sync | Cloud copy state can be established by Windows | provider-supported unpin/dehydrate, not deletion |
| WPS's own **Release space** operation | Vendor-supported | recommend the WPS UI; no public automation contract found |

The requested `move_to_recycle_bin / Not proven` category is **not safe enough
to implement**. Moving a synchronized item to the Windows Recycle Bin is still
a filesystem deletion from its sync root. Windows explicitly exposes delete
notifications to the sync provider, and WPS describes synchronized folders as
bidirectionally linked. A provider may propagate that deletion to cloud state.
The local Recycle Bin is not a boundary around the cloud copy.

The proposed category therefore has neither permanent-delete nor Recycle Bin
eligibility. The maximum defensible shipped behavior today is a path-free
recommendation that tells the user to invoke WPS's own **Release space**
control. A future implementation may introduce a distinct
`release_cloud_file_space`/storage-provider action for verified Cloud Files
placeholders. It must not be represented as deletion and must not accept a
plain directory merely because its path contains `WPSDrive`.

Accepted product decision: **do not add `wps_cloud_cache` in this slice.** It
will not be registered in the catalog, policy matrix, group tokens, or Clean
TUI. The research remains the evidence for rejecting deletion-based cleanup.

## Evidence classes

- **Proven**: stated by WPS/Kingsoft or Microsoft documentation, or directly
  observed in a version-identified Kingsoft-signed binary.
- **Observed**: bounded inspection of one host; suitable for exclusions and
  test hypotheses, not a cross-version layout contract.
- **Unknown**: no first-party compatibility contract was found.

Community user claims are not used as product authority. An answer from a WPS
community administrator is used only as a vendor support instruction and is
consistent with WPS's separate official learning page.

## WPS first-party evidence

### WPS owns a supported “Release space” workflow

WPS states that editing cloud documents creates a local cloud-drive path used
to store cache data. For low disk space, its documented operation is:

```text
WPS cloud icon -> Settings -> Storage location -> Release space
```

WPS separately documents changing the storage location and clearing local
cache while signing out. These are semantically different operations; the
sign-out option also removes login records and is ineligible because Foal must
not delete account or login state.

Source:
[How to delete WPS cloud drive files?](https://www.wps.cn/learning/question/detail/id/333345).

In response to a user asking how to remove local files without removing cloud
files, a WPS community administrator likewise directs users to **Release
space**, not to delete the `WPSDrive` directory or its children.

Source:
[How to delete local WPS cloud drive files without deleting cloud files](https://bbs.wps.cn/topic/18076).

No public WPS command-line option, IPC contract, SDK, manifest field, or stable
on-disk marker was found that lets a third-party cleaner invoke this operation
or reproduce its eligibility decision. The UI instruction proves the feature
exists; it does not authorize Foal to emulate it by deleting files.

### WPS synchronization propagates local changes

WPS's official learning material states that after a local folder is made a
WPS synchronized folder, **all changes inside it** are automatically updated
to the cloud. It also documents the reverse direction: edits on another device
are synchronized back to the computer. This is incompatible with treating
every locally present file as an independently deletable cache. A WPS community
administrator separately describes synchronized folders as bidirectionally
linked over the network with real-time data updates.

Sources:

- [Local folder changes synchronize to other devices](https://www.wps.cn/learning/article/detail/id/330472)
- [How to upload to WPS cloud only, without synchronizing locally](https://bbs.wps.cn/topic/34665)

The support material also distinguishes cloud-document cache, synchronized
folders, uploaded storage, and account-local cache. Consequently, a path such
as `Documents\WPSDrive` is not itself a state proof. Foal cannot infer that a
file has a remote copy, is fully uploaded, or is safe to remove from its path,
extension, timestamp, or WPS icon.

## Windows Cloud Files evidence

### Deleting a placeholder is a provider-visible delete

The Cloud Files platform defines `CF_CALLBACK_TYPE_NOTIFY_DELETE` to inform a
sync provider that a placeholder under its sync root is about to be deleted,
and `CF_CALLBACK_TYPE_NOTIFY_DELETE_COMPLETION` after successful deletion.
This proves that ordinary filesystem deletion is deliberately observable by
the provider. Moving an item to the Recycle Bin does not turn it into a
provider-neutral cache eviction.

Source:
[CF_CALLBACK_TYPE](https://learn.microsoft.com/en-us/windows/win32/api/cfapi/ne-cfapi-cf_callback_type).

Stopping WPS before a move would not establish safety. It is not a supported
conversion from delete to dehydrate, and a provider can reconcile filesystem
changes when it reconnects. Foal must not stop WPS processes for Clean in any
case.

### Windows can identify an in-sync placeholder, but only a placeholder

`CF_PLACEHOLDER_STATE_PLACEHOLDER` identifies a Cloud Files placeholder, and
`CF_PLACEHOLDER_STATE_IN_SYNC` means its content is in sync with the cloud.
`CF_PLACEHOLDER_STANDARD_INFO` separately reports on-disk, validated, locally
modified, pin, and in-sync information. These APIs can support a fail-closed
decision for an actual Cloud Files item; they do not classify ordinary files
or arbitrary WPS directories.

Sources:

- [CF_PLACEHOLDER_STATE](https://learn.microsoft.com/en-us/windows/win32/api/cfapi/ne-cfapi-cf_placeholder_state)
- [CF_PLACEHOLDER_STANDARD_INFO](https://learn.microsoft.com/en-us/windows/win32/api/cfapi/ns-cfapi-cf_placeholder_standard_info)
- [CfGetPlaceholderStateFromFileInfo](https://learn.microsoft.com/en-us/windows/win32/api/cfapi/nf-cfapi-cfgetplaceholderstatefromfileinfo)

For a future release-space action, a candidate would need at minimum:

1. membership in a registered sync root whose WPS ownership is established by
   a stable first-party identity, not display-name or directory-name matching;
2. placeholder and in-sync state read from the opened file handle;
3. `ModifiedDataSize == 0` and no partial/unvalidated state;
4. an ordinary, non-directory file with no unexpected reparse or hard-link
   condition beyond the Cloud Files placeholder itself;
5. Protection, fresh handle-based revalidation, provider health, and
   idle/stability checks;
6. a second state check immediately before requesting release.

Unknown or changing state must produce no operation.

### Supported local-space release is unpin/dehydrate, not delete

Microsoft defines `CF_PIN_STATE_UNPINNED` as a request that causes the sync
provider to be notified to asynchronously dehydrate/invalidate the
placeholder's on-disk content. It explicitly warns that a successful call does
not guarantee full dehydration. Microsoft also states that any application,
not only the provider, can call `CfSetPinState` when it has the required access.

Sources:

- [CF_PIN_STATE](https://learn.microsoft.com/en-us/windows/win32/api/cfapi/ne-cfapi-cf_pin_state)
- [CfSetPinState](https://learn.microsoft.com/en-us/windows/win32/api/cfapi/nf-cfapi-cfsetpinstate)

The sync root's hydration policy still controls the mechanism. Without
`AUTO_DEHYDRATION_ALLOWED`, Microsoft says the supported request is to clear
the pinned attribute and set the unpinned attribute, after which the sync
engine performs dehydration asynchronously. With that modifier, the platform
may call `CfDehydratePlaceholder` directly for an in-sync placeholder.

Source:
[CF_SYNC_POLICIES](https://learn.microsoft.com/en-us/windows/win32/api/cfapi/ns-cfapi-cf_sync_policies).

Direct `CfDehydratePlaceholder` is a lower-level operation requiring an
exclusive handle; Microsoft warns that improper exclusivity can corrupt data.
Foal should not call it directly unless a future dedicated design proves the
provider policy and uses the required oplock protocol.

Source:
[CfDehydratePlaceholder](https://learn.microsoft.com/en-us/previous-versions/mt827480%28v%3Dvs.85%29).

These APIs support a new storage-provider operation, not either existing Clean
deletion action. Results would also need asynchronous semantics: “request
accepted” is not “bytes reclaimed,” and a preview cannot count logical file
size as reclaimable on-disk bytes.

## Kingsoft-signed installed-artifact observations

Observed read-only on 2026-07-20. No user document names or contents were read.

- WPS Office 12.1.0.26895 was installed.
- `wpscloudsvr.exe` and `addons\ksyncengine\ksyncengine.dll` had valid
  Authenticode signatures from `Zhuhai Kingsoft Office Software Co., Ltd.`.
- The signed `ksyncengine.dll` contains imports/references for
  `CfDehydratePlaceholder`, `CfSetPinState`, and `cldapi.dll`. Another signed
  WPS cloud module also references the Cloud Files calls.

This is version-specific evidence that WPS implements Cloud Files behavior. It
does not publish WPS's sync-root identity, prove that every WPS storage mode
uses Cloud Files, or authorize third-party deletion.

On the same host, the observed `WPSDrive` root was an ordinary directory, not a
reparse point. Its bounded attribute inspection found no Cloud Files
placeholder indicators. WPS was running. This observation must not be
generalized into a universal WPS layout; instead, it demonstrates why a
directory-name resolver cannot assume CFAPI semantics.

The observed WPS application-data tree also contained account-scoped database
and configuration state. It is outside the proposed `WPSDrive` scope and must
remain excluded; Foal must never delete WPS login, identity, account,
collaboration, or synchronization metadata.

WPS's Help Center also explicitly warns users to configure cleaning software
so it does not delete WPS backup files. That is direct vendor evidence against
classifying adjacent WPS backup/recovery state as generic cache.

Source:
[Recover deleted, unsaved, and backup files](https://help.wps.com/articles/how-to-recover-files-deleted-by-mistake-and-unsaved-files-and-view-backup-files/).

## Why Recycle Bin is not an adequate safety control

`move_to_recycle_bin` only makes the local filesystem operation recoverable.
It does not guarantee any of the following:

- that WPS will interpret the operation as local cache eviction;
- that the cloud item will remain present;
- that a cloud/shared/enterprise recycle bin retains the item;
- that collaborators or other devices will be unaffected;
- that a later WPS reconciliation will not propagate the deletion;
- that moving a directory preserves all descendant cloud identities.

Accordingly, user confirmation such as “the cloud has a copy” cannot authorize
Foal to use the wrong operation. This is an operation-semantics problem, not a
warning-text or opt-in problem.

## Strict exclusions

A future resolver or recommendation must permanently exclude:

- the `WPSDrive` root and every directory selected only from its path;
- ordinary files lacking Cloud Files placeholder state;
- placeholders that are not in-sync, are partial/unvalidated, have locally
  modified bytes, or return invalid/unknown state;
- files pinned for offline availability unless the user's explicit action is
  specifically to change that pin intent;
- account, credential, login, sync database, collaboration, team-space,
  shared-file, history/version, conflict, upload-queue, and pending-operation
  state;
- WPS application data under `%LOCALAPPDATA%`/`%APPDATA%` merely because a
  directory or database contains `cache` in its name;
- symlinks, junctions, arbitrary reparse points, hard links, alternate streams,
  and paths outside an exact opened sync-root identity;
- every fallback from release-space failure to Recycle Bin or permanent
  deletion.

## Implementation recommendation

### Current slice

Do **not** add `wps_cloud_cache` as an executable Clean category. Add at most a
path-free review recommendation:

> WPS manages local cloud-document space. Use WPS cloud settings -> Storage
> location -> Release space. Foal will not delete synchronized files because a
> local deletion may affect the cloud copy.

Do not scan the user's WPS document tree merely to display that advice. Do not
claim measurable potential space.

### Possible future slice

If product scope expands, introduce a distinct planned action such as
`release_cloud_file_space`; do not overload `move_to_recycle_bin` or
`delete_permanently`. It should:

1. operate only on exact, WPS-owned registered Cloud Files sync roots;
2. resolve individual in-sync, locally hydrated placeholder files through
   handle-based Windows APIs;
3. request `CF_PIN_STATE_UNPINNED` through `CfSetPinState` rather than delete;
4. preserve namespace placeholders and cloud identity;
5. never unpin a user-pinned item without exact selection and explicit impact
   confirmation;
6. report accepted/pending/completed/failed release requests separately from
   deleted items;
7. compute observed on-disk allocation rather than logical file length, and
   promise no exact reclaimed bytes until post-operation verification;
8. fail closed when WPS provider identity, state, health, or API behavior is
   unknown;
9. retain WPS's own Release-space UI as the preferred route until a stable WPS
   automation contract or cross-version compatibility suite exists.

This future action remains **Not proven for WPS compatibility** despite the
Windows API being documented. Promotion requires a stable way to identify a
WPS sync root, versioned tests against supported WPS releases, and evidence
that WPS honors third-party unpin requests without cloud deletion or account
side effects.

## Evidence gaps

The following were not established from WPS first-party public contracts:

- a stable WPS sync-root provider ID or registration identity;
- a public WPS API/CLI for Release space;
- a stable mapping between WPS icons/UI labels and Cloud Files state;
- which WPS storage modes and versions use CFAPI placeholders;
- a vendor guarantee that third-party `CfSetPinState(UNPINNED)` is supported;
- provider-specific idle, upload-complete, conflict-free, or shared-item APIs;
- a supported way to estimate bytes WPS will actually release;
- a guarantee that local Recycle Bin deletion never propagates to WPS cloud.

These are state and operation-ownership gaps. Age thresholds, filenames,
directory naming, process-idle checks, user warnings, and `move_to_recycle_bin`
cannot close them.
