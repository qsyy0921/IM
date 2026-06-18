param()

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$outputRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-ai-eval-cases-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $outputRoot | Out-Null

& powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot "validate-ai-eval-cases.ps1") `
    -OutputPath (Join-Path $outputRoot "ai-eval-case-validation.json") `
    -MarkdownPath (Join-Path $outputRoot "ai-eval-cases.md")

if ($LASTEXITCODE -ne 0) {
    throw "validate-ai-eval-cases.ps1 failed with exit code $LASTEXITCODE"
}

Write-Host "OK   ai eval case schema guardrails"
