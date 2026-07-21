# freexnats installer for Windows PowerShell.
#
# Usage:
#   irm https://raw.githubusercontent.com/CooDdk/freexnats/master/install.ps1 | iex
#
# Environment overrides:
#   $env:FREEXNATS_VERSION       Install a specific tag (default: latest release).
#   $env:FREEXNATS_INSTALL_DIR   Install directory (default: %LOCALAPPDATA%\Programs\FreeXNats).

$ErrorActionPreference = 'Stop'

$Repo       = 'CooDdk/freexnats'
$BinaryName = 'freexnats.exe'
$InstallDir = if ($env:FREEXNATS_INSTALL_DIR) { $env:FREEXNATS_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\FreeXNats' }

function Get-Architecture {
    switch -Regex ($env:PROCESSOR_ARCHITECTURE) {
        '^(AMD64|x86_64)$' { return 'amd64' }
        '^ARM64$'          { return 'arm64' }
        default            { throw "unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
    }
}

function Resolve-FreexnatsVersion {
    if ($env:FREEXNATS_VERSION) { return $env:FREEXNATS_VERSION }
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    if (-not $release.tag_name) { throw 'failed to resolve latest version (set $env:FREEXNATS_VERSION to override)' }
    return $release.tag_name
}

function Install-Binary {
    param(
        [Parameter(Mandatory)] [string] $Version,
        [Parameter(Mandatory)] [string] $Arch
    )
    $assetName = "freexnats-windows-$Arch.exe"
    $url       = "https://github.com/$Repo/releases/download/$Version/$assetName"
    $target    = Join-Path $InstallDir $BinaryName

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }
    Write-Host "Downloading $url"
    Invoke-WebRequest -Uri $url -OutFile $target -UseBasicParsing
}

function Add-InstallDirToPath {
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not $userPath) { $userPath = '' }
    $segments = $userPath -split ';' | Where-Object { $_ -ne '' }
    if ($segments -contains $InstallDir) { return }
    $newPath = if ($userPath.Trim().Length -eq 0) { $InstallDir } else { "$userPath;$InstallDir" }
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
    Write-Host "Added $InstallDir to user PATH (open a new terminal to pick it up)"
}

$arch    = Get-Architecture
$version = Resolve-FreexnatsVersion
Write-Host "Installing freexnats $version for windows-$arch"

Install-Binary -Version $version -Arch $arch
Add-InstallDirToPath

Write-Host ''
Write-Host ("Installed: {0}" -f (Join-Path $InstallDir $BinaryName))
Write-Host 'Open a new terminal and run: freexnats'
