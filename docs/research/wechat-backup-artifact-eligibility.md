# WeChat backup artifact cleanup eligibility

Status: researched and executable category rejected 2026-07-20. This note evaluates only the proposed
`wechat_files` Clean category under the legacy current-user
`Documents\WeChat Files` tree. It does not authorize reading message databases,
register a category, or authorize deletion.

## Decision

**Do not register an executable `wechat_files` category.** This is the accepted
product decision for the current slice. The intended posture (`Opt-in`,
`move_to_recycle_bin`, Not proven, exact selection only) is not enough to close
the resolver's ownership gap.

| Proposed scope | Evidence result | Decision |
| --- | --- | --- |
| `BackupFiles\*.bakdb` | Legacy Tencent code identifies `BackupFiles`, `.bakdb`, and backup/recovery concepts, but does not publish their exact path relationship or deletion contract | reject for now |
| `Msg\Multi\bak\*` | No Tencent first-party layout or semantic evidence found; not observed on the research host | reject |
| ordinary `*.db`, `Msg`, attachments, account/configuration state | User data or unknown state | permanently exclude |
| whole `BackupFiles`, account root, or `WeChat Files` root | May contain user-created backup/control state; root ownership is too broad | permanently exclude |

Exact CLI/TUI selection can record the user's intent and their confirmation
that another copy exists. It cannot prove which local files are backup payloads
instead of restore indexes, partial jobs, message stores, or account state.
Filename extension and directory spelling are not substitutes for that proof.

## Evidence policy

Only these evidence classes were used:

- Tencent/WeChat public distribution material;
- version-identified, validly Tencent-signed installed program artifacts;
- bounded, read-only, de-identified directory metadata.

No chat database was opened. No account directory name, contact name, message
filename, database content, or other personal identifier was recorded. No
forum, cleaner list, reverse-engineered database schema, or third-party blog is
product authority.

The public [WeChat for Windows distribution surface](https://pc.weixin.qq.com/)
establishes the vendor and product surface, but no Tencent public cleanup API,
stable Windows backup-file layout, `.bakdb` deletion guarantee, or retention
contract was found there or in Tencent support material available during this
research.

## Tencent-signed installed-artifact evidence

Two materially different Windows generations were present on the research
host. Both inspected modules had a valid Authenticode signature from
`Tencent Technology (Shenzhen) Company Limited`.

| Generation | Module identity | SHA-256 | Relevant embedded terms |
| --- | --- | --- | --- |
| legacy WeChat | `WeChatWin.dll`, product `WECHAT`, file version `3.9.12.17` | `0989347CFBA54C8D63F226CDB6F440BF2C9D3981C506FABCC7B515372E4CAF48` | `WeChat Files`, `BackupFiles`, `.bakdb`, `backup_media`, `BackupRestoreWnd`, backup/restore/manage/delete resource identifiers |
| current Weixin | `Weixin.dll`, product `Weixin`, file version `4.1.11.54` | `260E7B50797FB52CE6D86AF215865B54628904FCE67CF629C86190F6878111D8` | `WeChat Files`, `xwechat_files`, `BackupFiles`, `Backup.db`; no `.bakdb` term found |

This is strong, version-specific evidence that the legacy client has a backup
and recovery feature and understands `.bakdb`, while the current client has a
different data-generation vocabulary. It does **not** establish:

- that every `.bakdb` below `BackupFiles` is a completed, inactive backup;
- that `.bakdb` is never a restore index or partial/intermediate file;
- that the client does not require companion metadata for restore or backup
  management;
- that `BackupFiles` has the same semantics in 3.x and 4.x;
- that deleting a payload outside the client preserves its backup catalog;
- that `Msg\Multi\bak` is a supported backup-payload location.

The presence of backup-management and delete resource identifiers shows that
the application itself owns a managed deletion workflow. It is not authority
for Foal to reproduce that workflow from filenames.

## Research-host metadata

Read-only, de-identified inspection found the legacy `Documents\WeChat Files`
root. One account-scoped tree contained an empty `BackupFiles` directory.
`Msg\Multi\bak` was not present. No candidate files and no reparse points were
observed. Relevant WeChat/Weixin processes were not running at the time of the
snapshot.

An empty directory is not a cleanup candidate. This host therefore supplies no
payload fixture, no completed/partial-state fixture, and no evidence connecting
a concrete `.bakdb` file to a safe deletion state.

## Required exclusions

A future implementation must never infer eligibility merely from `.db` or a
name containing `bak`. It must exclude at least:

- all ordinary `*.db`, including message, search, media-index, contact, and
  account databases;
- `Backup.db` unless Tencent documents it as disposable (current evidence
  instead suggests control/catalog state);
- the whole `Msg` tree except a separately proven exact payload layout;
- `FileStorage`, images, video, audio, received/sent files, and other
  attachments;
- configuration, authentication, device/account, migration, backup catalog,
  restore index, WAL/journal, lock, temporary, and partial-transfer state;
- every unknown child, reparse point, alternate stream, hard-linked file, and
  path outside the fixed current-user root;
- the account root, `BackupFiles` root, `WeChat Files` root, and current 4.x
  `xwechat_files` generation as whole-directory candidates.

Protection remains deny-only and must apply before totals and again immediately
before action. It cannot turn an unknown artifact into an eligible backup.

## Process and stability requirements for any future rule

If Tencent later supplies an exact payload/state contract, the rule would still
need all of these fail-closed gates:

1. WeChat/Weixin application identity known idle before discovery and again
   immediately before Recycle Bin mutation; unknown process state skips.
2. Cover both product generations and their backup/update/helper processes;
   never stop or terminate them.
3. Resolve fresh from the fixed current-user root; do not enumerate arbitrary
   drives, registry-selected roots, or sibling Tencent products.
4. Reject reparse points throughout the containment chain and validate ordinary
   file identity immediately before action.
5. Reject any open, locked, partial, journaled, or vendor-state-referenced
   artifact.
6. Take bounded metadata snapshots before and after inspection and require
   identical file identity, length, and last-write time.
7. Apply a conservative recent-write exclusion, but do not claim that age
   proves completion. Tencent provides no supported age threshold, so choosing
   one would remain product policy rather than evidence.
8. Never fall back from Recycle Bin failure to permanent deletion.

The installed executable names are useful fixtures, not an exhaustive process
contract. An allowlist that misses a writer is unsafe; an all-Tencent gate has
large false-skip cost. This unresolved writer-identity gap is independently
sufficient to fail closed.

## User confirmation does not close the resolver gap

The proposed exact selection wording may say that the user confirms a phone or
other independently verified copy exists. It must not claim a cloud copy: no
Tencent evidence reviewed here establishes that ordinary personal WeChat chat
history is a reconstructible cloud cache.

Even after that confirmation, Foal must know that each candidate is the backup
payload the user intended to discard. Current evidence cannot make that
classification. Consequently the safest product surface is a path-free notice
directing the user to WeChat's own migration/backup management UI, not a Clean
candidate.

## Evidence needed to reopen

Any one of these could justify fresh evaluation, but not automatic approval:

- Tencent documentation naming the exact Windows backup payload layout and
  stating when files can be removed;
- a supported Tencent API/command for listing and deleting completed backups;
- Tencent-signed code evidence that joins the full canonical path, completion
  state, companion metadata, and managed deletion behavior for each supported
  generation;
- de-identified fixtures covering completed, active, canceled, partial,
  restored, and corrupted backup states across supported 3.x and 4.x clients.

Until then, `wechat_files` remains research-only and absent from the canonical
catalog, `all`, group tokens, and TUI Select All.
