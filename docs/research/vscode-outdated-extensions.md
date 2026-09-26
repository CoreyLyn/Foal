# VS Code outdated extension versions (Clean eligibility)

Status: researched 2026-09-26 against VS Code `1.139.1` (`04c0d99f4f`),
vscode-docs `f54680b`, and VSCodium `1.135.06055`. Evidence for the
`vscode-outdated-extensions` Clean category.

## Decision

| Scope | Decision | Planned action |
| --- | --- | --- |
| Folders VS Code marked for removal in `.obsolete` while another version of the same extension stays installed | **Ship permanent (opt-in, `dev-caches`)** | `delete_permanently` |
| Fully uninstalled extensions (no unmarked installed version remains) | **Excluded** | — |
| `%USERPROFILE%\.vscode-oss\extensions` | **Excluded** | — |
| Cursor, Windsurf, Trae extension folders | **Excluded** (closed-source forks, no first-party evidence) | — |

Roots: `%USERPROFILE%\.vscode\extensions` (VS Code) and
`%USERPROFILE%\.vscode-insiders\extensions` (VS Code Insiders).

## Upstream behavior

- `.obsolete` is VS Code's own "delete at next cleanup" list. Only
  `withRemovedExtensions` reads and writes it: a flat JSON object
  `{"<key>": true}`; unparsable content is treated as `{}`
  ([extensionManagementService.ts#L822-L858](https://github.com/microsoft/vscode/blob/1.139.1/src/vs/platform/extensionManagement/node/extensionManagementService.ts#L822-L858)).
- Keys are `ExtensionKey.toString()`: `${id}-${version}`, plus
  `-${targetPlatform}` unless the platform is `undefined`
  ([extensionManagementUtil.ts#L30-L63](https://github.com/microsoft/vscode/blob/1.139.1/src/vs/platform/extensionManagement/common/extensionManagementUtil.ts#L30-L63)).
  The id is lowercased
  ([#L78-L84](https://github.com/microsoft/vscode/blob/1.139.1/src/vs/platform/extensionManagement/common/extensionManagementUtil.ts#L78-L84));
  version and platform keep their case. A marketplace `universal` platform keeps
  its `-universal` suffix
  ([extensionGalleryService.ts#L403-L405](https://github.com/microsoft/vscode/blob/1.139.1/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L403-L405)).
  Lookups are exact
  ([extensionManagementService.ts#L978](https://github.com/microsoft/vscode/blob/1.139.1/src/vs/platform/extensionManagement/node/extensionManagementService.ts#L978)).
- Extension folders are extracted under the same key
  ([#L620-L623](https://github.com/microsoft/vscode/blob/1.139.1/src/vs/platform/extensionManagement/node/extensionManagementService.ts#L620-L623)).
- `ExtensionsWatcher` marks every on-disk version that no profile references
  ([extensionsWatcher.ts#L104-L134](https://github.com/microsoft/vscode/blob/1.139.1/src/vs/platform/extensionManagement/node/extensionsWatcher.ts#L104-L134),
  [#L183-L191](https://github.com/microsoft/vscode/blob/1.139.1/src/vs/platform/extensionManagement/node/extensionsWatcher.ts#L183-L191)).
  `.obsolete` therefore holds upgrade leftovers, fully uninstalled extensions,
  and unreferenced manual folders.
- `deleteExtensionsMarkedForRemoval`
  ([#L960-L998](https://github.com/microsoft/vscode/blob/1.139.1/src/vs/platform/extensionManagement/node/extensionManagementService.ts#L960-L998))
  treats an extension as still installed when an unmarked version remains on
  disk. Only when none remains does it run `postUninstall` on the highest
  version: the `vscode:uninstall` hook and global storage removal
  ([extensionLifecycle.ts#L29-L62](https://github.com/microsoft/vscode/blob/1.139.1/src/vs/platform/extensionManagement/node/extensionLifecycle.ts#L29-L62)).
  It deletes marked folders whose metadata has `installedTimestamp`.
- The cleanup runs when the desktop shared process starts, when a remote server
  starts, and on "Developer: Cleanup Extensions Folder" — not only at editor
  launch.
- `~\.vscode\extensions\extensions.json` is the default profile's manifest;
  entries carry `relativeLocation` when the folder is a direct child of the
  extensions root
  ([extensionsProfileScannerService.ts#L308-L312](https://github.com/microsoft/vscode/blob/1.139.1/src/vs/platform/extensionManagement/common/extensionsProfileScannerService.ts#L308-L312)).
  Other profiles keep their own manifests under `User\profiles\<id>`.
- VSCodium keeps the OSS default `.vscode-oss`, which source builds of
  Code - OSS share
  ([VSCodium migration.md#L12](https://github.com/VSCodium/vscodium/blob/1.135.06055/docs/migration.md?plain=1#L12)),
  so Foal cannot attribute that folder to one process.

## Foal rule

A folder is a candidate only when all hold:

1. `.obsolete` maps its exact ExtensionKey to `true`;
2. the default profile's `extensions.json` does not reference it;
3. its `package.json` yields that ExtensionKey (folder name compared
   case-insensitively), with `installedTimestamp` present and a non-empty
   platform when one is declared;
4. the same extension stays installed: a manifest entry references an existing,
   unmarked folder whose `package.json` declares the same id.

Rule 4 is stricter than upstream — the unmarked version must be the one the
default profile uses — so removing a candidate can never be the step that makes
VS Code treat the extension as uninstalled. Every metadata read is bounded and
strict; a missing or malformed file, or any legacy manifest entry without
`relativeLocation`, yields no candidate for that root. Foal never rewrites
`.obsolete`; a stale key is harmless because VS Code iterates existing folders
only and clears keys itself on reinstall.

Gates: VS Code and VS Code Insiders must both be idle before discovery and
after measurement (either editor can use the other's folder through
`--extensions-dir`). The `code` CLI runs `Code.exe`, so the same gate covers it.
Remote extension hosts use `~/.vscode-server` and are unaffected.

## Residual risk

Foal's permanent removal continues past locked files. If a stray process locks a
file inside a candidate after the idle gates, a partial removal can leave a
folder without `package.json` that VS Code no longer scans or deletes. It is
harmless and can be removed manually; the idle gates make it unlikely.
