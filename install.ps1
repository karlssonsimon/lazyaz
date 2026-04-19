<#
.SYNOPSIS
    Install lazyaz from GitHub Releases.

.DESCRIPTION
    Downloads the appropriate lazyaz release for this machine, verifies its
    SHA-256 checksum, and installs the binary into a directory on PATH.

.EXAMPLE
    iwr -useb https://raw.githubusercontent.com/karlssonsimon/lazyaz/master/install.ps1 | iex

.EXAMPLE
    $env:LAZYAZ_VERSION = "v0.1.0"
    iwr -useb https://raw.githubusercontent.com/karlssonsimon/lazyaz/master/install.ps1 | iex

.PARAMETER Version
    Release tag to install (default: latest, or $env:LAZYAZ_VERSION).

.PARAMETER InstallDir
    Target directory (default: $env:LAZYAZ_INSTALL_DIR or
    $env:LOCALAPPDATA\Programs\lazyaz).
#>
[CmdletBinding()]
param(
    [string]$Version = $env:LAZYAZ_VERSION,
    [string]$InstallDir = $env:LAZYAZ_INSTALL_DIR
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$Repo = "karlssonsimon/lazyaz"
$Bin = "lazyaz"

function Write-Log($msg) { Write-Host "==> $msg" }

if (-not $Version) { $Version = "latest" }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

if ($Version -eq "latest") {
    Write-Log "resolving latest release"
    $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
    $Version = $release.tag_name
    if (-not $Version) { throw "could not determine latest version" }
}

$numVersion = $Version.TrimStart("v")
$archive = "{0}_{1}_windows_{2}.zip" -f $Bin, $numVersion, $arch
$baseUrl = "https://github.com/$Repo/releases/download/$Version"

$tmp = Join-Path $env:TEMP ("lazyaz-install-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

try {
    Write-Log "downloading $archive ($Version)"
    $archivePath = Join-Path $tmp $archive
    Invoke-WebRequest "$baseUrl/$archive" -OutFile $archivePath
    Invoke-WebRequest "$baseUrl/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt")

    Write-Log "verifying checksum"
    $line = Get-Content (Join-Path $tmp "checksums.txt") | Where-Object { $_ -match " $([regex]::Escape($archive))$" }
    if (-not $line) { throw "no checksum entry for $archive" }
    $expected = ($line -split "\s+")[0]
    $actual = (Get-FileHash -Algorithm SHA256 $archivePath).Hash.ToLower()
    if ($actual -ne $expected.ToLower()) {
        throw "checksum mismatch ($actual != $expected)"
    }

    Write-Log "extracting"
    Expand-Archive -Path $archivePath -DestinationPath $tmp -Force
    $exePath = Join-Path $tmp "$Bin.exe"
    if (-not (Test-Path $exePath)) { throw "binary not found in archive" }

    if (-not $InstallDir) {
        $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\lazyaz"
    }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null

    $target = Join-Path $InstallDir "$Bin.exe"
    Copy-Item $exePath $target -Force
    Write-Log "installed $Bin $Version to $target"

    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    $onPath = $userPath -split ";" | Where-Object { $_ -eq $InstallDir }
    if (-not $onPath) {
        [Environment]::SetEnvironmentVariable("PATH", "$userPath;$InstallDir", "User")
        Write-Log "added $InstallDir to user PATH — restart your shell to pick it up"
    }
}
finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
