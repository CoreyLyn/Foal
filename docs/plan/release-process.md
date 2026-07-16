# Release process

Foal uses tag-driven GitHub draft releases. Automation builds and uploads a
candidate; a maintainer validates the candidate on Windows before publishing
it. Pushing a tag never makes binaries immediately public.

## Versioning

- Tags use Semantic Versioning with a `v` prefix.
- The initial public channel uses prerelease tags such as `v0.1.0-rc.1`.
- During `v0`, breaking CLI or JSON-contract changes require a minor-version
  increment. Compatible fixes use a patch-version increment.
- Release tags must point to commits reachable from `main`.

## Build and artifacts

GoReleaser builds the canonical command and its convenience alias with
`CGO_ENABLED=0` and `-trimpath`:

- `foal.exe`
- `fo.exe`

Each release contains one ZIP for Windows amd64 and one for Windows arm64. Each
archive includes both executables, README, and the selected repository license.
The release also includes SHA-256 checksums. Linker flags inject only the tag
and full commit into the shared version read model; no build timestamp is
injected.

GitHub Actions creates provenance attestations for the ZIP archives. Windows
Authenticode signing remains required before broad stable distribution and
before adding WinGet as an official channel; provenance attestations do not
replace Windows code signing.

## Gates

The release workflow fails closed unless all of these conditions hold:

1. GoReleaser accepts the annotated tag as valid SemVer and its commit is on
   `main`.
2. The repository contains `LICENSE`, `LICENSE.md`, or `LICENSE.txt`.
3. Module verification and the full test suite pass on Windows.
4. GoReleaser successfully produces both architecture archives and checksums.

The license choice and minimum supported Windows versions are product-owner
decisions. They must be recorded before the first public prerelease; automation
does not infer either decision.

## Publishing

1. Confirm `main` is green and the intended commit contains no unrelated work.
2. Create and push an annotated prerelease tag, initially `v0.1.0-rc.1`.
3. Wait for the Release workflow to create the draft and attest its ZIP files.
4. Download both archives and verify checksums.
5. Smoke-test amd64 and arm64 on clean supported Windows environments, including
   `--version`, `--help`, read-only JSON commands, Clean dry-run, and the TUI.
6. Review generated release notes and publish the draft manually.

Enable GitHub immutable releases before the first public release. Do not replace
assets or retarget a published tag; publish a new patch or prerelease instead.

## Distribution phases

1. GitHub prereleases are the only initial public channel.
2. Stable `v0.x.y` GitHub releases follow after deletion semantics, support
   boundaries, signing, and upgrade expectations are stable.
3. Scoop may follow for the developer audience.
4. WinGet follows only after signed stable releases have demonstrated a stable
   asset layout and command contract.

MSI, MSIX, NSIS, Chocolatey, nightly releases, and `go install` are not primary
distribution channels for the initial release phase.
