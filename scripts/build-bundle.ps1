#!/usr/bin/env pwsh
[CmdletBinding()]
param(
    [string]$OutputDir = "dist",
    [string]$Name = "gist",
    [switch]$SkipFrontendInstall
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$frontendDir = Join-Path $root "frontend"
$backendDir = Join-Path $root "backend"
$frontendDist = Join-Path $frontendDir "dist"
$stagedDir = Join-Path $backendDir "internal/webui/dist"
$outputDir = Join-Path $root $OutputDir
$binaryName = if ($env:OS -eq "Windows_NT") { "$Name.exe" } else { $Name }
$binaryPath = Join-Path $outputDir $binaryName
$linuxPackageName = "$Name-linux-amd64"
$linuxDir = Join-Path $outputDir $linuxPackageName
$linuxArchive = Join-Path $outputDir "$linuxPackageName.tar.gz"

function Invoke-Step {
    param(
        [string]$Message,
        [scriptblock]$Action
    )

    Write-Host "==> $Message"
    & $Action
}

function Invoke-GoBuild {
    param(
        [Parameter(Mandatory)][string]$OutputPath,
        [string]$TargetOS,
        [string]$TargetArch
    )

    $savedEnv = @{}
    foreach ($name in @("CGO_ENABLED", "GOOS", "GOARCH")) {
        $savedEnv[$name] = [Environment]::GetEnvironmentVariable($name)
    }

    $env:CGO_ENABLED = "0"
    if ($TargetOS) { $env:GOOS = $TargetOS }
    if ($TargetArch) { $env:GOARCH = $TargetArch }

    Push-Location $backendDir
    try {
        go build -tags=embedstatic -trimpath -ldflags="-s -w" -o $OutputPath ./cmd/server
    }
    finally {
        Pop-Location
        foreach ($name in $savedEnv.Keys) {
            if ($null -eq $savedEnv[$name]) {
                Remove-Item "Env:\$name" -ErrorAction SilentlyContinue
            }
            else {
                Set-Item "Env:\$name" $savedEnv[$name]
            }
        }
    }
}

Invoke-Step "Building frontend" {
    Push-Location $frontendDir
    try {
        if (-not $SkipFrontendInstall) {
            bun install --frozen-lockfile
        }
        bun run build
    }
    finally {
        Pop-Location
    }
}

if (-not (Test-Path (Join-Path $frontendDist "index.html"))) {
    throw "Frontend build did not produce $frontendDist/index.html"
}

Invoke-Step "Staging frontend assets for embedding" {
    if (Test-Path $stagedDir) {
        Remove-Item -Recurse -Force $stagedDir
    }
    New-Item -ItemType Directory -Force -Path $stagedDir | Out-Null
    Copy-Item -Path (Join-Path $frontendDist "*") -Destination $stagedDir -Recurse -Force
}

try {
    Invoke-Step "Building single-file binary" {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
        Invoke-GoBuild -OutputPath $binaryPath
    }

    Invoke-Step "Building Linux amd64 bundle" {
        if (Test-Path $linuxDir) {
            Remove-Item -Recurse -Force $linuxDir
        }
        New-Item -ItemType Directory -Force -Path $linuxDir | Out-Null
        Invoke-GoBuild -OutputPath (Join-Path $linuxDir $Name) -TargetOS "linux" -TargetArch "amd64"
        Copy-Item -Path (Join-Path $PSScriptRoot "gist.sh") -Destination $linuxDir -Force

        Remove-Item $linuxArchive -Force -ErrorAction SilentlyContinue
        & tar -czf $linuxArchive -C $outputDir $linuxPackageName
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to create $linuxArchive"
        }
    }
}
finally {
    Remove-Item -Recurse -Force $stagedDir -ErrorAction SilentlyContinue
}

Write-Host ""
Write-Host "Built: $binaryPath"
Write-Host "Linux amd64 bundle: $linuxArchive"
Write-Host ""
Write-Host "Deploy the Linux bundle:"
Write-Host "  tar -xzf $linuxPackageName.tar.gz"
Write-Host "  cd $linuxPackageName && chmod +x gist gist.sh"
Write-Host "  create gist.conf next to gist.sh (format: ./gist.sh help), then: sudo ./gist.sh install"
Write-Host ""
Write-Host "The binary contains the frontend. Configure it with environment variables:"
Write-Host "  GIST_ADDR        listen address         (default :8080)"
Write-Host "  GIST_DATA_DIR    data directory         (default ./data)"
Write-Host "  GIST_DB_PATH     sqlite database file   (default <GIST_DATA_DIR>/gist.db)"
Write-Host "  GIST_LOG_LEVEL   debug|info|warn|error  (default info)"
Write-Host ""
Write-Host "Example: `$env:GIST_ADDR=':9000'; .\$binaryName"
