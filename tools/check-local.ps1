param(
    [switch]$SkipPowerShellParser
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
    Write-Host "== runbook entrypoints =="
    & (Join-Path $PSScriptRoot "check-runbook-entrypoints.ps1")

    Write-Host "== service brief sync =="
    & (Join-Path $PSScriptRoot "check-service-brief-sync.ps1")

    Write-Host "== ddd boundaries =="
    & (Join-Path $PSScriptRoot "check-ddd-boundaries.ps1")

    Write-Host "== cross-service table access =="
    & (Join-Path $PSScriptRoot "check-cross-service-table-access.ps1")

    Write-Host "== docs entrypoints =="
    & (Join-Path $PSScriptRoot "check-doc-entrypoints.ps1")

    Write-Host "== project naming =="
    & (Join-Path $PSScriptRoot "check-project-naming.ps1")

    Write-Host "== file size budgets =="
    & (Join-Path $PSScriptRoot "check-file-size-budget.ps1")

    Write-Host "== local prometheus config =="
    & (Join-Path $PSScriptRoot "check-local-prometheus-config.ps1")

    Write-Host "== local grafana config =="
    & (Join-Path $PSScriptRoot "check-local-grafana-config.ps1")

    Write-Host "== api-gateway gates =="
    & (Join-Path $PSScriptRoot "check-api-gateway-gates.ps1")

    Write-Host "== otel sampling policy =="
    & (Join-Path $PSScriptRoot "check-otel-sampling-policy.ps1")

    Write-Host "== otel service wiring =="
    & (Join-Path $PSScriptRoot "check-otel-service-wiring.ps1")

    Write-Host "== otel span attributes =="
    & (Join-Path $PSScriptRoot "check-otel-span-attributes.ps1")

    Write-Host "== grpc correlation logs =="
    & (Join-Path $PSScriptRoot "check-grpc-correlation-logs.ps1")

    Write-Host "== debug listener exposure =="
    & (Join-Path $PSScriptRoot "check-debug-listener-exposure.ps1")

    Write-Host "== public listener auth boundaries =="
    & (Join-Path $PSScriptRoot "check-public-listener-auth-guards.ps1")

    Write-Host "== grpc/wss tls config guardrails =="
    & (Join-Path $PSScriptRoot "check-grpc-tls-config-guards.ps1")

    Write-Host "== kafka producer config =="
    & (Join-Path $PSScriptRoot "check-kafka-producer-config.ps1")

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
