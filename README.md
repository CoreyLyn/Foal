# Foal

[![CI](https://github.com/CoreyLyn/Foal/actions/workflows/ci.yml/badge.svg)](https://github.com/CoreyLyn/Foal/actions/workflows/ci.yml)
[![License: GPL-3.0-only](https://img.shields.io/badge/license-GPL--3.0--only-blue.svg)](LICENSE)

Foal is a safe, preview-first cleanup CLI for Windows developers and power users.

It finds reclaimable space, explains the planned action, and makes destructive work explicit. Foal also provides project-artifact cleanup, disk analysis, system snapshots, operation history, and read-only uninstall review.

> [!IMPORTANT]
> Foal is pre-release software. Preview the result before execution and review permanent-deletion selections carefully.

## Why Foal?

- **Preview first** — inspect candidates and their planned actions before changing files.
- **Conservative by default** — broader categories require an explicit CLI opt-in or a disclosed TUI confirmation.
- **Windows-aware safety** — protected paths, reparse points, permissions, running applications, and Recycle Bin capacity are first-class checks.
- **No silent elevation** — inaccessible items are reported as skipped; Foal never elevates itself.
- **Automation-friendly** — JSON is the stable contract shared by the CLI and TUI.

Foal is inspired by tools such as Mole, but follows its own Windows-specific safety model rather than pursuing feature parity.

## Install

Download the Windows amd64 or arm64 ZIP from [GitHub Releases](https://github.com/CoreyLyn/Foal/releases), extract `foal.exe`, and place it on your `PATH`.

Release archives include SHA-256 checksums and GitHub provenance attestations. Current binaries are not Authenticode-signed; Windows may show an unrecognized-app warning. ARM64 builds remain preview builds until native ARM64 smoke testing is available.

To build from source, install Go 1.25 or later on Windows:

```powershell
git clone https://github.com/CoreyLyn/Foal.git
cd Foal
go build -o foal.exe ./cmd/foal
.\foal.exe --version
```

## Quick start

Preview conservative cleanup candidates without deleting anything:

```powershell
foal clean --dry-run
```

Open the interactive TUI:

```powershell
foal
```

Run the default cleanup after reviewing the preview:

```powershell
foal clean --execute
```

Permanent-action categories need explicit per-run authorization:

```powershell
foal clean --execute --opt-in browser_cache --allow-permanent
```

For scripts, request structured output:

```powershell
foal clean --dry-run --json
foal status --json
foal history --json
```

## Commands

| Command | Behavior |
| --- | --- |
| `foal` | Opens the interactive TUI in a terminal. |
| `foal clean` | Previews or executes catalog-based cleanup. Requires `--dry-run` or `--execute`. |
| `foal purge <root...>` | Previews or permanently removes allowlisted rebuildable project artifacts under explicit roots. |
| `foal analyze <path>` | Reports directory totals and top children without changing files. |
| `foal status` | Reports a read-only Windows and Foal state snapshot. |
| `foal history` | Reads prior Clean and Purge operation records. |
| `foal uninstall` | Reviews installed applications and possible residue; never runs uninstallers or deletes leftovers. |
| `foal version` | Reports version, commit, Go runtime, and target platform. |

Run `foal --help` for the complete shipped flag surface.

## Cleanup model

Every executable Clean category declares exactly one action:

- `move_to_recycle_bin` for recoverable cleanup.
- `delete_permanently` for narrowly proven regenerable or re-downloadable content.

Permanent deletion is never a fallback when a Recycle Bin operation fails. The CLI requires both `--execute` and `--allow-permanent` for permanent work; without authorization, those candidates are skipped while eligible Recycle Bin work can continue. The TUI presents the same actions in one strengthened confirmation.

Foal resolves candidates again, reloads protection rules, runs applicable safety gates, and validates every path immediately before mutation. Recycle Bin work runs before permanent work. Local failures do not silently broaden the cleanup scope.

Available opt-in groups:

| Group | Includes |
| --- | --- |
| `dev-caches` | Supported package-manager, build-tool, browser-runtime, IDE, and editor caches. |
| `app-caches` | Supported non-editor application caches, currently Obsidian. |
| `cli-agents` | Independently approved product-scoped CLI-agent residue, currently Grok Build updater backups. |
| `all` | Every executable opt-in category; still subject to safety gates and permanent authorization. |

Use an exact category name when you want the narrowest scope:

```powershell
foal clean --dry-run --opt-in vscode_cache
foal clean --execute --opt-in vscode_cache --allow-permanent
```

The authoritative category list, action matrix, impact notes, and exclusions live in the [Clean deletion policy](docs/plan/clean-deletion-policy.md).

## Project artifact purge

`purge` is separate from Clean. It only scans roots you provide and previews by default:

```powershell
foal purge .\my-project
foal purge --json .\project-a .\project-b
```

The v1 allowlist matches exact directory names: `node_modules`, `target`, `dist`, `build`, `.build`, `.next`, and `__pycache__`. Volume roots and Windows system paths are rejected.

Execution re-discovers matches and permanently removes them only with per-run authorization:

```powershell
foal purge --execute --allow-permanent .\my-project
```

Deleted dependencies or build output may need to be downloaded or rebuilt. Purge never performs secure erasure, elevation, process stopping, installer cleanup, or implicit root discovery.

## Protection rules

Create `%APPDATA%\Foal\protection.txt` to exclude paths from Clean and Purge. Use one absolute local path per line; `#` begins a comment. A rule protects that exact path and its full subtree.

```text
# Keep local development data
D:\Work\ImportantProject
C:\Users\me\AppData\Local\Example\Cache
```

Set `FOAL_PROTECTION_FILE` to use a different file. Rules are deny-only: they can remove candidates, but never add or authorize cleanup. Invalid lines are skipped and reported; an unreadable selected file or invalid UTF-8 fails closed. UNC paths, relative paths, and 8.3 short-name paths are rejected.

## Safety boundaries

Foal deliberately does not:

- empty the Recycle Bin;
- automatically elevate or stop applications;
- treat browser history, cookies, credentials, sessions, or user-authored data as cache;
- claim secure erasure or guaranteed physical-space recovery;
- execute uninstallers or remove application leftovers;
- perform system optimization actions.

Administrator-only cleanup remains a read-only permission-boundary notice. `optimize` is reserved for future read-only health checks and recommendations.

## Platform and releases

- Compatibility baseline: Windows 10 or Windows Server 2016 and later.
- Primary target: Windows 11 x64.
- Release artifacts: portable amd64 and arm64 ZIP archives containing `foal.exe`.
- Release flow: tag-driven draft releases, manually smoke-tested before publishing.

See the [release process](docs/plan/release-process.md) and [Windows support research](docs/research/windows-support.md) for verification details.

## Development

```powershell
go test ./...
go build -o foal.exe ./cmd/foal
```

Safety invariants and JSON contracts take priority over human-output snapshots. Product and architecture decisions are documented under [`docs/`](docs/).

## License

Foal is licensed under the [GNU General Public License v3.0 only](LICENSE) (`GPL-3.0-only`).
