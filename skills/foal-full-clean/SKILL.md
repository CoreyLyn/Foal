---
name: foal-full-clean
description: Run the broadest supported Foal Clean cleanup on a Windows computer, including standard categories, exact-selection-only categories, permanent actions, and Windows component-store servicing. Use this skill whenever the user asks Foal to fully clean, deeply clean, reclaim as much supported space as possible, run every supported cleanup category, or install Foal and then clean the PC. Check for Foal first and try the official installation script when it is missing.
compatibility: Windows PowerShell 5.1 or PowerShell 7; network access is required only when Foal must be installed.
---

# Foal Full Clean

Use Foal's CLI to run the broadest cleanup that the shipped Clean catalog supports.

## Scope

- Treat "full" as all default Clean categories, the `all` standard-selection group, and every exact-selection-only category.
- Keep `purge` and `uninstall` out of scope. They require explicit project roots or application selections and are not part of Clean.
- Do not empty the Recycle Bin, stop applications, modify ACLs, add cleanup paths, or bypass Foal's safety gates.
- Explain that permanent cleanup is ordinary irreversible filesystem removal, not secure erasure.
- Explain that Windows component-store servicing may request UAC, runs last, never deletes WinSxS files directly, and may require a restart.

## 1. Find Foal

Require Windows. Stop with a clear explanation on another operating system.

Resolve the executable without assuming that a newly updated user `PATH` is visible:

```powershell
$foalCommand = Get-Command 'foal.exe' -CommandType Application -ErrorAction SilentlyContinue
$foalExe = if ($foalCommand) {
    $foalCommand.Source
} else {
    Join-Path $env:LOCALAPPDATA 'Programs\foal\foal.exe'
}
```

If `$foalExe` does not exist, install Foal before continuing.

## 2. Install Foal when missing

Prefer the checked-in installer when the current workspace is the Foal repository:

```powershell
$repoRoot = git rev-parse --show-toplevel 2>$null
$localInstaller = if ($repoRoot) {
    Join-Path $repoRoot 'scripts\install.ps1'
}

if ($localInstaller -and (Test-Path -LiteralPath $localInstaller -PathType Leaf)) {
    & $localInstaller
}
```

If no checked-in installer is available, use the official Foal installer published by this repository:

```powershell
$installerUri = 'https://raw.githubusercontent.com/CoreyLyn/Foal/main/scripts/install.ps1'
$installerPath = Join-Path ([System.IO.Path]::GetTempPath()) ('foal-install-' + [guid]::NewGuid().ToString('N') + '.ps1')

try {
    Invoke-WebRequest -Uri $installerUri -OutFile $installerPath
    & $installerPath
} finally {
    Remove-Item -LiteralPath $installerPath -Force -ErrorAction SilentlyContinue
}
```

The installer verifies the release archive against Foal's published SHA-256 checksums. Do not fall back to an unrelated package, source build, or unofficial download. If installation fails, stop and report the error; do not attempt cleanup.

After installation, set the executable path directly and verify it:

```powershell
$foalExe = Join-Path $env:LOCALAPPDATA 'Programs\foal\foal.exe'
& $foalExe --version
```

Stop if the executable is still missing or `--version` fails.

## 3. Build the full Clean selection

The `all` token deliberately excludes high-risk exact-selection-only categories. Add all of them explicitly:

```powershell
$fullOptIns = @(
    'all'
    'nvidia_installer_cache'
    'lghub-cache'
    'thunder-update-download'
    'windows-temp'
    'windows-update-download-cache'
    'winsxs_component_store'
)

$fullOptInArgs = foreach ($category in $fullOptIns) {
    '--opt-in'
    $category
}
```

Do not replace this list with guessed category names. If the installed Foal rejects a name, run `foal --help`, report the version mismatch, and stop before mutation. Do not silently reduce the requested scope.

## 4. Preview first

Run the exact full selection in dry-run mode:

```powershell
& $foalExe clean --dry-run --json @fullOptInArgs
```

The exact `winsxs_component_store` preview may request UAC for read-only DISM analysis. It still performs no cleanup.

Check the exit code and inspect the structured result. Summarize:

- reclaimable candidates and bytes by planned action;
- skipped or unavailable categories and their reasons;
- permanent-deletion impact;
- Windows servicing readiness and possible UAC/restart impact.

If preview fails, JSON is invalid, or the selected scope is not represented, stop before execution.

## 5. Confirm destructive intent

A current-turn request such as "run the full cleanup now" is explicit execution authorization. A request to inspect, explain, prepare, or preview is not.

When execution intent is missing or ambiguous, show the preview summary and ask for confirmation before continuing. Make the confirmation disclose both:

- irreversible permanent deletion through `--allow-permanent`;
- Windows servicing through the independent `--allow-servicing` authorization.

Never infer either authorization from a preview-only request.

## 6. Execute the same full selection

After explicit authorization, run:

```powershell
& $foalExe clean --execute --json @fullOptInArgs --allow-permanent --allow-servicing
```

Foal fresh-resolves and revalidates candidates before mutation. Let Foal isolate local failures and preserve its action order: Recycle Bin first, permanent deletion next, servicing last.

Do not:

- retry skipped categories with broader permissions;
- turn a Recycle Bin failure into permanent deletion;
- kill Foal, DISM, TrustedInstaller, or the elevated helper after component-store servicing starts;
- claim that all previewed bytes were reclaimed;
- claim rollback, secure erasure, or automatic application stopping.

## 7. Report the outcome

Report the installed Foal version and:

- completed, skipped, failed, and canceled counts;
- bytes moved to the Recycle Bin;
- bytes permanently deleted;
- Windows servicing status and whether a restart is required;
- important skip/failure reasons;
- whether cleanup was complete, partial, or not started.

Use Foal's actual Result fields. Do not convert "affected bytes" or an approximate servicing observation into a claim about exact free disk space.
