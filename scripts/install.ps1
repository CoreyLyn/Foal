#Requires -Version 5.1
<#
.SYNOPSIS
  Install Foal from GitHub Releases (Windows amd64 / arm64).

.DESCRIPTION
  Downloads the latest (or pinned) release ZIP, verifies SHA-256 against
  checksums.txt, installs foal.exe under the user Programs directory, and
  optionally prepends that directory to the user PATH.

  One-liner (latest, user install + PATH):
    irm https://raw.githubusercontent.com/CoreyLyn/Foal/main/scripts/install.ps1 | iex

  Pin a version or change options:
    & ([scriptblock]::Create((irm https://raw.githubusercontent.com/CoreyLyn/Foal/main/scripts/install.ps1))) -Version v0.2.2
    & ([scriptblock]::Create((irm https://raw.githubusercontent.com/CoreyLyn/Foal/main/scripts/install.ps1))) -NoPath
    & ([scriptblock]::Create((irm https://raw.githubusercontent.com/CoreyLyn/Foal/main/scripts/install.ps1))) -Uninstall

.PARAMETER Version
  Release tag (e.g. v0.2.2). Default: latest non-draft, non-prerelease GitHub release.

.PARAMETER InstallDir
  Destination directory for foal.exe. Default: %LOCALAPPDATA%\Programs\foal

.PARAMETER NoPath
  Do not modify the user PATH.

.PARAMETER Uninstall
  Remove foal.exe from InstallDir and drop that directory from the user PATH
  when it matches the default install location (or InstallDir when set).

.NOTES
  Binaries are not Authenticode-signed yet; Windows may show SmartScreen.
  ARM64 builds are preview until native smoke testing is available.
#>
[CmdletBinding(DefaultParameterSetName = 'Install')]
param(
    [Parameter(ParameterSetName = 'Install')]
    [string]$Version,

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Uninstall')]
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\foal'),

    [Parameter(ParameterSetName = 'Install')]
    [switch]$NoPath,

    [Parameter(ParameterSetName = 'Uninstall')]
    [switch]$Uninstall
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Repo = 'CoreyLyn/Foal'
$BinaryName = 'foal.exe'
$UserAgent = 'Foal-install.ps1'

function Write-Step {
    param([string]$Message)
    Write-Host "==> $Message"
}

function Write-Ok {
    param([string]$Message)
    Write-Host "    $Message" -ForegroundColor Green
}

function Write-WarnStep {
    param([string]$Message)
    Write-Host "    $Message" -ForegroundColor Yellow
}

function Get-TargetArch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch -Regex ($arch) {
        '^(AMD64|x86_64)$' { return 'amd64' }
        '^(ARM64|aarch64)$' { return 'arm64' }
        default {
            throw "Unsupported architecture '$arch'. Foal ships Windows amd64 and arm64 only."
        }
    }
}

function Invoke-GitHubJson {
    param([Parameter(Mandatory)][string]$Uri)

    $headers = @{
        'User-Agent' = $UserAgent
        'Accept'     = 'application/vnd.github+json'
    }
    if ($env:GITHUB_TOKEN) {
        $headers['Authorization'] = "Bearer $($env:GITHUB_TOKEN)"
    }

    return Invoke-RestMethod -Uri $Uri -Headers $headers -Method Get
}

function Get-Release {
    param([string]$Tag)

    if ([string]::IsNullOrWhiteSpace($Tag)) {
        Write-Step "Resolving latest GitHub release for $Repo"
        $release = Invoke-GitHubJson -Uri "https://api.github.com/repos/$Repo/releases/latest"
    }
    else {
        $normalized = if ($Tag.StartsWith('v')) { $Tag } else { "v$Tag" }
        Write-Step "Resolving GitHub release $normalized"
        $release = Invoke-GitHubJson -Uri "https://api.github.com/repos/$Repo/releases/tags/$normalized"
    }

    if (-not $release -or -not $release.tag_name) {
        throw 'Could not resolve a GitHub release.'
    }
    if ($release.draft) {
        throw "Release $($release.tag_name) is still a draft and is not installable."
    }

    return $release
}

function Get-AssetUrl {
    param(
        [Parameter(Mandatory)]$Release,
        [Parameter(Mandatory)][string]$Name
    )

    $asset = @($Release.assets) | Where-Object { $_.name -eq $Name } | Select-Object -First 1
    if (-not $asset) {
        $available = (@($Release.assets) | ForEach-Object { $_.name }) -join ', '
        throw "Release $($Release.tag_name) has no asset named '$Name'. Available: $available"
    }
    return [string]$asset.browser_download_url
}

function Get-ExpectedSha256 {
    param(
        [Parameter(Mandatory)][string]$ChecksumsPath,
        [Parameter(Mandatory)][string]$ArchiveName
    )

    foreach ($line in Get-Content -LiteralPath $ChecksumsPath) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        # GoReleaser: "<sha256>  <filename>" (two spaces)
        if ($line -match '^(?<hash>[0-9a-fA-F]{64})\s+(?<name>\S+)\s*$') {
            if ($Matches['name'] -eq $ArchiveName) {
                return $Matches['hash'].ToLowerInvariant()
            }
        }
    }
    throw "checksums.txt does not list '$ArchiveName'."
}

function Get-FileSha256Hex {
    param([Parameter(Mandatory)][string]$Path)

    # Prefer .NET so the script works even when Get-FileHash is unavailable
    # (some constrained or partially provisioned PowerShell hosts).
    $stream = [System.IO.File]::OpenRead($Path)
    try {
        $sha = [System.Security.Cryptography.SHA256]::Create()
        try {
            $hash = $sha.ComputeHash($stream)
            return ([System.BitConverter]::ToString($hash) -replace '-', '').ToLowerInvariant()
        }
        finally {
            $sha.Dispose()
        }
    }
    finally {
        $stream.Dispose()
    }
}

function Assert-FileSha256 {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Expected
    )

    $actual = Get-FileSha256Hex -Path $Path
    if ($actual -ne $Expected.ToLowerInvariant()) {
        throw "SHA-256 mismatch for $(Split-Path -Leaf $Path). expected=$Expected actual=$actual"
    }
}

function Ensure-Directory {
    param([Parameter(Mandatory)][string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) {
        New-Item -ItemType Directory -Path $Path -Force | Out-Null
    }
}

function Add-UserPathEntry {
    param([Parameter(Mandatory)][string]$Entry)

    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($null -eq $current) { $current = '' }

    $parts = @($current -split ';' | Where-Object { $_ -ne '' })
    $exists = $parts | Where-Object { $_.TrimEnd('\') -ieq $Entry.TrimEnd('\') }
    if ($exists) {
        Write-Ok "User PATH already contains $Entry"
        return
    }

    $newValue = if ([string]::IsNullOrWhiteSpace($current)) {
        $Entry
    }
    else {
        "$Entry;$current"
    }

    [Environment]::SetEnvironmentVariable('Path', $newValue, 'User')

    # Current session only (does not require restart)
    if (-not (($env:Path -split ';') | Where-Object { $_.TrimEnd('\') -ieq $Entry.TrimEnd('\') })) {
        $env:Path = "$Entry;$env:Path"
    }
    Write-Ok "Added $Entry to user PATH (new shells pick this up automatically)"
}

function Remove-UserPathEntry {
    param([Parameter(Mandatory)][string]$Entry)

    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ([string]::IsNullOrWhiteSpace($current)) { return }

    $kept = @(
        $current -split ';' |
            Where-Object { $_ -ne '' -and $_.TrimEnd('\') -ine $Entry.TrimEnd('\') }
    )
    $newValue = $kept -join ';'
    [Environment]::SetEnvironmentVariable('Path', $newValue, 'User')

    $env:Path = (
        ($env:Path -split ';') |
            Where-Object { $_ -ne '' -and $_.TrimEnd('\') -ine $Entry.TrimEnd('\') }
    ) -join ';'

    Write-Ok "Removed $Entry from user PATH (if present)"
}

function Uninstall-Foal {
    Write-Step "Uninstalling Foal from $InstallDir"

    $exe = Join-Path $InstallDir $BinaryName
    if (Test-Path -LiteralPath $exe) {
        Remove-Item -LiteralPath $exe -Force
        Write-Ok "Removed $exe"
    }
    else {
        Write-WarnStep "No $BinaryName found at $exe"
    }

    if ((Test-Path -LiteralPath $InstallDir) -and -not (Get-ChildItem -LiteralPath $InstallDir -Force -ErrorAction SilentlyContinue)) {
        Remove-Item -LiteralPath $InstallDir -Force
        Write-Ok "Removed empty directory $InstallDir"
    }

    Remove-UserPathEntry -Entry $InstallDir
    Write-Step 'Uninstall complete'
}

function Install-Foal {
    $arch = Get-TargetArch
    $release = Get-Release -Tag $Version
    $tag = [string]$release.tag_name
    $versionBare = $tag.TrimStart('v')
    $archiveName = "foal_${versionBare}_windows_${arch}.zip"
    $checksumName = 'checksums.txt'

    Write-Step "Target: Windows $arch, release $tag"
    if ($arch -eq 'arm64') {
        Write-WarnStep 'ARM64 builds are preview until native smoke testing is available.'
    }

    $zipUrl = Get-AssetUrl -Release $release -Name $archiveName
    $sumUrl = Get-AssetUrl -Release $release -Name $checksumName

    $tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("foal-install-" + [guid]::NewGuid().ToString('N'))
    Ensure-Directory -Path $tempRoot
    $zipPath = Join-Path $tempRoot $archiveName
    $sumPath = Join-Path $tempRoot $checksumName
    $extractDir = Join-Path $tempRoot 'extract'

    try {
        Write-Step "Downloading $archiveName"
        Invoke-WebRequest -Uri $zipUrl -OutFile $zipPath -UserAgent $UserAgent -UseBasicParsing

        Write-Step "Downloading $checksumName"
        Invoke-WebRequest -Uri $sumUrl -OutFile $sumPath -UserAgent $UserAgent -UseBasicParsing

        Write-Step 'Verifying SHA-256'
        $expected = Get-ExpectedSha256 -ChecksumsPath $sumPath -ArchiveName $archiveName
        Assert-FileSha256 -Path $zipPath -Expected $expected
        Write-Ok "checksum ok ($($expected.Substring(0, 12))...)"

        Write-Step 'Extracting archive'
        Expand-Archive -LiteralPath $zipPath -DestinationPath $extractDir -Force

        $sourceExe = Get-ChildItem -LiteralPath $extractDir -Filter $BinaryName -Recurse -File |
            Select-Object -First 1
        if (-not $sourceExe) {
            throw "Archive does not contain $BinaryName."
        }

        Ensure-Directory -Path $InstallDir
        $destExe = Join-Path $InstallDir $BinaryName

        # Replace atomically enough for a single-file CLI: remove then copy.
        if (Test-Path -LiteralPath $destExe) {
            Remove-Item -LiteralPath $destExe -Force
        }
        Copy-Item -LiteralPath $sourceExe.FullName -Destination $destExe -Force
        Write-Ok "Installed $destExe"

        if (-not $NoPath) {
            Write-Step 'Updating user PATH'
            Add-UserPathEntry -Entry $InstallDir
        }
        else {
            Write-WarnStep "Skipped PATH update. Add manually: $InstallDir"
        }

        Write-Step 'Checking binary'
        $reported = & $destExe --version 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw "Installed binary failed --version (exit $LASTEXITCODE): $reported"
        }
        Write-Ok ($reported | Out-String).Trim()

        Write-Host ''
        Write-Step 'Install complete'
        Write-Host "    Binary : $destExe"
        Write-Host "    Version: $tag"
        if (-not $NoPath) {
            Write-Host '    Tip    : open a new terminal if foal is not found yet'
        }
        Write-Host '    Try    : foal --help'
        Write-Host ''
        Write-WarnStep 'Current release binaries are not Authenticode-signed; SmartScreen may warn.'
    }
    finally {
        if (Test-Path -LiteralPath $tempRoot) {
            Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

try {
    if ($Uninstall) {
        Uninstall-Foal
    }
    else {
        Install-Foal
    }
}
catch {
    Write-Error $_
    exit 1
}
