# install.ps1 -- One-line installer for nodex CLI on Windows
# Installs nodex.exe to $LOCALAPPDATA\nodex\bin and adds it to the User PATH.
#
# Usage:
#   irm https://raw.githubusercontent.com/adhuldas/nodexa-cli/main/install.ps1 | iex
#
# Or with parameters:
#   powershell -ExecutionPolicy Bypass -File .\install.ps1 [-InstallDir <path>] [-Version <version>]

[CmdletBinding()]
param(
    [string]$InstallDir = $(
        if ($env:LOCALAPPDATA) {
            Join-Path $env:LOCALAPPDATA "nodex\bin"
        } else {
            Join-Path $HOME ".nodex\bin"
        }
    ),
    [string]$Version = ""
)

$ErrorActionPreference = 'Stop'

$Repo = "adhuldas/nodexa-cli"
$BinName = "nodex.exe"

# Architecture detection
$rawArch = $env:PROCESSOR_ARCHITECTURE
if ($env:PROCESSOR_ARCHITEW6432) {
    $rawArch = $env:PROCESSOR_ARCHITEW6432
}

switch -Regex ($rawArch) {
    '(AMD64|x64|x86_64)' { $Arch = "amd64" }
    '(ARM64|aarch64)'   { $Arch = "arm64" }
    default {
        Write-Error "Unsupported architecture: $rawArch"
        exit 1
    }
}

Write-Host "==> Installing nodex CLI for windows-${Arch}..."

function Add-ToUserPath {
    param([string]$PathToAdd)

    $userPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
    $pathParts = if ($userPath) { $userPath -split ';' | Where-Object { $_ -ne '' } } else { @() }

    if ($pathParts -notcontains $PathToAdd) {
        Write-Host "  Adding $PathToAdd to User PATH..."
        $newUserPath = ($pathParts + $PathToAdd) -join ';'
        [Environment]::SetEnvironmentVariable("Path", $newUserPath, [EnvironmentVariableTarget]::User)
    }

    if (($env:PATH -split ';') -notcontains $PathToAdd) {
        $env:PATH = "$PathToAdd;$env:PATH"
    }
}

# 1. If Go is installed and repository is cloned locally, build directly
if ((Test-Path "./cmd/nodex/main.go") -and (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "  Building from local source..."
    go build -ldflags "-s -w" -o $BinName ./cmd/nodex
    if (Test-Path $BinName) {
        if (-not (Test-Path $InstallDir)) {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }
        $target = Join-Path $InstallDir $BinName
        Move-Item -Path $BinName -Destination $target -Force
        Add-ToUserPath $InstallDir

        Write-Host "==> Successfully installed $BinName to $target"
        & $target --version
        exit 0
    }
}

# 2. Query GitHub Releases for the target release asset
$apiUrl = if ($Version) {
    "https://api.github.com/repos/${Repo}/releases/tags/${Version}"
} else {
    "https://api.github.com/repos/${Repo}/releases/latest"
}

$release = $null
try {
    $headers = @{ "User-Agent" = "nodex-installer" }
    $release = Invoke-RestMethod -Uri $apiUrl -Headers $headers -Method Get
} catch {
    Write-Verbose "Could not fetch release from GitHub API: $_"
}

if (-not $release -or -not $release.tag_name) {
    # Fallback to building with go install if Go is available
    if (Get-Command go -ErrorAction SilentlyContinue) {
        Write-Host "  No GitHub release found. Building with go install..."
        go install "github.com/${Repo}/cmd/nodex@latest"
        $gopath = (go env GOPATH).Trim()
        $gopathBin = Join-Path $gopath "bin\$BinName"
        if (Test-Path $gopathBin) {
            if (-not (Test-Path $InstallDir)) {
                New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
            }
            $target = Join-Path $InstallDir $BinName
            Copy-Item -Path $gopathBin -Destination $target -Force
            Add-ToUserPath $InstallDir

            Write-Host "==> Successfully installed $BinName to $target"
            & $target --version
            exit 0
        }
    }
    Write-Error "Could not find a GitHub release or local Go toolchain to install nodex."
    exit 1
}

$tag = $release.tag_name
Write-Host "  Found release ${tag}"

# Locate suitable asset in release, or construct default download URL
$downloadUrl = $null
$archiveExt = ".tar.gz"

if ($release.assets) {
    # Check for zip or tar.gz asset matching windows and current architecture
    $matchedAsset = $release.assets | Where-Object {
        $_.name -match "windows" -and $_.name -match $Arch -and ($_.name -match "\.tar\.gz$" -or $_.name -match "\.zip$")
    } | Select-Object -First 1

    if ($matchedAsset) {
        $downloadUrl = $matchedAsset.browser_download_url
        if ($matchedAsset.name -match "\.zip$") {
            $archiveExt = ".zip"
        }
    }
}

if (-not $downloadUrl) {
    $downloadUrl = "https://github.com/${Repo}/releases/download/${tag}/nodex_windows_${Arch}.tar.gz"
}

Write-Host "  Downloading ${downloadUrl}..."
$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
$archiveFile = Join-Path $tempDir ("nodex_archive" + $archiveExt)

try {
    # Ensure TLS 1.2+
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13
    Invoke-WebRequest -Uri $downloadUrl -OutFile $archiveFile -UseBasicParsing

    # Extract archive
    if ($archiveExt -eq ".zip") {
        Expand-Archive -Path $archiveFile -DestinationPath $tempDir -Force
    } else {
        # .tar.gz extraction
        $tarCmd = Get-Command tar -ErrorAction SilentlyContinue
        if ($tarCmd) {
            & tar.exe -xzf $archiveFile -C $tempDir
        } else {
            throw "tar command not found to extract .tar.gz archive. Please install tar or Windows 10/11 build tools."
        }
    }

    $extractedBin = Get-ChildItem -Path $tempDir -Filter $BinName -Recurse | Select-Object -First 1
    if (-not $extractedBin) {
        throw "Could not find $BinName in the downloaded release archive."
    }

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    $target = Join-Path $InstallDir $BinName
    Move-Item -Path $extractedBin.FullName -Destination $target -Force
    Add-ToUserPath $InstallDir

    Write-Host "==> Successfully installed $BinName to $target"
    & $target --version
} finally {
    if (Test-Path $tempDir) {
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
