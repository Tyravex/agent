$ErrorActionPreference = "Stop"

Set-Location (Split-Path -Parent $PSScriptRoot)

$version = (Get-Content .\VERSION -Raw).Trim()
$commit = (git rev-parse --short HEAD).Trim()
$buildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

$ldflags = @(
    "-X github.com/Tyravex/agent/internal/version.Number=$version"
    "-X github.com/Tyravex/agent/internal/version.Commit=$commit"
    "-X github.com/Tyravex/agent/internal/version.BuildDate=$buildDate"
) -join " "

New-Item -ItemType Directory -Force bin | Out-Null

go build `
    -ldflags $ldflags `
    -o bin/tyravex-agent.exe `
    ./cmd/tyravex-agent

if ($LASTEXITCODE -ne 0) {
    throw "Agent build failed"
}

Write-Host "Tyravex Agent v$version built successfully"
.\bin\tyravex-agent.exe version