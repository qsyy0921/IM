param(
    [switch]$SkipPowerShellParser
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
    Write-Host "== runbook entrypoints =="
    & (Join-Path $PSScriptRoot "check-runbook-entrypoints.ps1")

    Write-Host "== docs entrypoints =="
    & (Join-Path $PSScriptRoot "check-doc-entrypoints.ps1")

    Write-Host "== file size budgets =="
    & (Join-Path $PSScriptRoot "check-file-size-budget.ps1")

    Write-Host "== local prometheus config =="
    & (Join-Path $PSScriptRoot "check-local-prometheus-config.ps1")

    Write-Host "== local grafana config =="
    & (Join-Path $PSScriptRoot "check-local-grafana-config.ps1")

    Write-Host "== git whitespace =="
    git diff --check
    git diff --cached --check

    if (-not $SkipPowerShellParser) {
        Write-Host "== powershell parser =="
        $scripts = Get-ChildItem -LiteralPath $PSScriptRoot -Filter "*.ps1" -File
        foreach ($script in $scripts) {
            $parseErrors = $null
            [System.Management.Automation.PSParser]::Tokenize((Get-Content -LiteralPath $script.FullName -Raw), [ref]$parseErrors) | Out-Null
            if ($parseErrors -and $parseErrors.Count -gt 0) {
                $messages = $parseErrors | ForEach-Object { "$($script.Name): line $($_.Token.StartLine): $($_.Message)" }
                throw ($messages -join [Environment]::NewLine)
            }
            Write-Host "OK   $($script.Name)"
        }
    }
}
finally {
    Pop-Location
}
