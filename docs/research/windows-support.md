# Windows support baseline

Status: researched 2026-07-17. This document records compatibility evidence;
it does not by itself create a product support promise.

## Conclusion

- **Build/runtime floor:** `Windows 10` or `Windows Server 2016`, because Foal
  declares Go 1.25 and Go 1.21+ supports only those Windows versions or newer.
- **Foal API floor:** the Windows APIs called directly by Foal are older than
  that floor. None of the inspected calls raises the requirement above Windows
  10 / Server 2016.
- **Architectures:** releases target `windows/amd64` and `windows/arm64`, with
  `CGO_ENABLED=0`. Go documents both architectures on Windows 10 / Server 2016
  or newer. The release config leaves `GOAMD64` and `GOARM64` at Go's defaults;
  the documented defaults are AMD64 v1 and ARM64 v8.0.
- **Verified today:** CI uses GitHub Actions `windows-latest`, currently a
  Windows Server 2025 x64 image. It executes the amd64 binary, but only
  cross-builds arm64. Therefore Windows 10, Server 2016, and native arm64 are
  **not yet tested by this repository**.

Recommended first release statement:

> Compatibility baseline: Windows 10 or later, or Windows Server 2016 or later.
> Windows 11 x64 is the primary desktop target. ARM64 builds are provided as
> preview until native ARM64 CI or smoke testing is added. Current hosted CI
> verification covers Windows Server 2025 x64 only.

If `supported` is intended to mean “covered by CI and eligible for bug-fix
commitments,” the current evidence only justifies Windows Server 2025 x64.
Before promising the broader Go compatibility floor, add at least a Windows 10
22H2 x64 smoke test (self-hosted or VM) and a native Windows 11 ARM64 smoke test.

## Toolchain evidence

`go.mod` declares `go 1.25.0`. The Go project's current minimum-requirements
page says Go 1.21 and later requires Windows 10+ or Windows Server 2016+. The
Go-on-Windows matrix also lists Windows 8 / Server 2012 and Windows 7 / Server
2008 R2 as ending with Go 1.20. Consequently, rebuilding the current source for
Windows 7, 8, 8.1, Server 2008 R2, or Server 2012 is outside Go 1.25's supported
platform contract even if individual Win32 calls happen to exist there.

Primary sources:

- [Go minimum requirements](https://go.dev/wiki/MinimumRequirements)
- [Go on Microsoft Windows support matrix](https://go.dev/wiki/Windows)
- [Go 1.25 release notes](https://go.dev/doc/go1.25)
- [Go architecture environment variables](https://go.dev/doc/install/source#environment)

Foal also pins `golang.org/x/sys v0.45.0`. Its `windows` package is a low-level
binding to operating-system primitives, not a broader OS-support guarantee; the
Go toolchain's Windows floor and each called API's Microsoft requirements remain
the relevant constraints.

- [`golang.org/x/sys/windows` v0.45.0 documentation](https://pkg.go.dev/golang.org/x/sys@v0.45.0/windows)

## Direct Windows API audit

| Foal use | API or facility | Microsoft minimum client | Effect on Foal floor |
| --- | --- | --- | --- |
| `internal/status/disk_windows.go` | `GetDiskFreeSpaceExW` | Windows XP | None |
| `internal/clean/running_application_windows.go` | `CreateToolhelp32Snapshot`, `Process32First`, `Process32Next` | Windows XP | None |
| `internal/cli/terminal_windows.go` | `GetConsoleMode` | Windows 2000 Professional | None |
| `internal/core/pathsafe/pathsafe.go` | `GetFileAttributesW`, `CreateFileW`, `GetFileInformationByHandle`, `CloseHandle` | Windows XP or earlier | None |
| `internal/clean/recycle_bin_capacity_windows.go` | `GetVolumePathNameW` | Windows XP | None |
| same | `GetVolumeNameForVolumeMountPointW` | Windows XP | None |
| same | `SHQueryRecycleBinW` | Windows 2000 Professional / Windows XP | None |
| `internal/core/delete/recyclebin_windows.go` | `SHFileOperationW` | Windows XP | None; Microsoft notes it was superseded by `IFileOperation` in Vista, but it remains available |
| `internal/uninstall/registry_windows.go` | Win32 registry access through `x/sys/windows/registry` | Predates Windows 10 | None |
| `internal/cli/clipboard_windows.go` | bundled `clip.exe` command | Microsoft's current page covers Windows 10/11 and Server 2016+ | Consistent with the Go floor; clipboard copy is an auxiliary TUI action |

Microsoft API references:

- [`GetDiskFreeSpaceExW`](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexw)
- [`CreateToolhelp32Snapshot`](https://learn.microsoft.com/en-us/windows/win32/api/tlhelp32/nf-tlhelp32-createtoolhelp32snapshot)
- [`GetConsoleMode`](https://learn.microsoft.com/en-us/windows/console/getconsolemode)
- [`GetFileAttributesW`](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getfileattributesw)
- [`CreateFileW`](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew)
- [`GetFileInformationByHandle`](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getfileinformationbyhandle)
- [`CloseHandle`](https://learn.microsoft.com/en-us/windows/win32/api/handleapi/nf-handleapi-closehandle)
- [`GetVolumePathNameW`](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getvolumepathnamew)
- [`GetVolumeNameForVolumeMountPointW`](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getvolumenameforvolumemountpointw)
- [`SHQueryRecycleBinW`](https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shqueryrecyclebinw)
- [`SHFileOperationW`](https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shfileoperationw)
- [`clip`](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/clip)

The remaining Windows-specific code uses Go filesystem operations and file
attribute constants. It does not introduce a newer Windows API requirement.
Bubble Tea and Bubbles are pure-Go application dependencies in Foal's build;
no repository evidence was found that they impose a Windows version later than
the Go runtime baseline.

## Compatibility versus support promise

These terms should remain separate in release documentation:

1. **Theoretically buildable:** Go 1.25 can build Foal for Windows amd64 and
   arm64; Foal's GoReleaser configuration does so with cgo disabled.
2. **API-compatible by documentation:** Windows 10 / Server 2016 meets every
   direct API minimum found above. Older Windows versions meet many API minima
   but fail the supported Go 1.25 baseline.
3. **Project-tested:** only the GitHub-hosted `windows-latest` x64 environment
   is exercised. The workflow currently builds but does not execute arm64.
4. **Project-supported:** must be an explicit maintainer policy, ideally no
   broader than the tested matrix. A compatibility report is not a warranty.

The hosted-runner mapping is documented by the official GitHub runner-images
repository: `windows-latest` currently maps to Windows Server 2025 x64.

- [GitHub Actions runner image labels](https://github.com/actions/runner-images)

## Lifecycle note

Windows 10 Home and Pro reached Microsoft end of support on 2025-10-14. That
does not stop a Go 1.25 binary from running, but it matters for the wording of a
new 2026 release. Supporting Windows 10 is best described as compatibility for
users who remain on it (including applicable ESU/LTSC cases), while Windows 11
should be the primary desktop support target.

- [Windows 10 Home and Pro lifecycle](https://learn.microsoft.com/en-us/lifecycle/products/windows-10-home-and-pro)
- [Windows Server 2016 lifecycle](https://learn.microsoft.com/en-us/lifecycle/products/windows-server-2016)
