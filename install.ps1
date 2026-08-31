# finst - One-line installer for Windows PowerShell
# Repository: https://github.com/arisolta/finst
# Usage: irm https://raw.githubusercontent.com/arisolta/finst/main/install.ps1 | iex

$ErrorActionPreference = 'Stop'
$Repo = "arisolta/finst"

Write-Host "==> Installing finst (Financial Terminal CLI)..." -ForegroundColor Cyan

# 1. Detect Architecture
$Arch = "amd64"
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
    $Arch = "arm64"
}

# 2. Get latest release tag
Write-Host "==> Fetching latest release info from GitHub..." -ForegroundColor Cyan
$Tag = "v1.0.0"
try {
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    if ($Release.tag_name) {
        $Tag = $Release.tag_name
    }
} catch {
    # Fallback to v1.0.0
}

$FileName = "finst_windows_$Arch.zip"
$DownloadUrl = "https://github.com/$Repo/releases/download/$Tag/$FileName"

# 3. Target Install Directory
$InstallDir = "$env:LOCALAPPDATA\Programs\finst"
if (!(Test-Path -Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$TempZip = "$env:TEMP\$FileName"

Write-Host "==> Downloading $Tag for Windows ($Arch)..." -ForegroundColor Cyan
try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $TempZip -UseBasicParsing
    Expand-Archive -Path $TempZip -DestinationPath $InstallDir -Force
    Remove-Item $TempZip -Force
} catch {
    Write-Host "Release asset not available yet. If Go is installed, trying 'go install'..." -ForegroundColor Yellow
    if (Get-Command go -ErrorAction SilentlyContinue) {
        go install "github.com/$Repo/cmd/finst@latest"
        Write-Host "✓ Successfully installed finst via go install!" -ForegroundColor Green
        exit 0
    }
    Write-Error "Failed to download $DownloadUrl"
}

# 4. Add to User PATH if not present
$UserPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", [EnvironmentVariableTarget]::User)
    $env:Path = "$env:Path;$InstallDir"
    Write-Host "==> Added $InstallDir to User PATH." -ForegroundColor Cyan
}

Write-Host "✓ finst $Tag installed successfully to $InstallDir\finst.exe!" -ForegroundColor Green
Write-Host "`nQuick Start:" -ForegroundColor White
Write-Host "  finst AAPL" -ForegroundColor White
Write-Host "  finst MSFT --view compact" -ForegroundColor White
Write-Host "  finst NVDA --export csv" -ForegroundColor White
