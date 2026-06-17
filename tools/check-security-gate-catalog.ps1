$ErrorActionPreference = "Stop"

$validator = Join-Path $PSScriptRoot "validate-security-gate-catalog.ps1"
if (-not (Test-Path -LiteralPath $validator -PathType Leaf)) {
    throw "Missing security gate catalog validator: $validator"
}

function Write-JsonFile {
    param(
        [string]$Path,
        $Value
    )

    $dir = Split-Path -Parent $Path
    if ($dir -and -not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
    $Value | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Invoke-Validator {
    param(
        [string]$CatalogPath
    )

    try {
        $output = & $validator -CatalogPath $CatalogPath 2>&1
        return [pscustomobject]@{
            ExitCode = 0
            Output = (($output | Out-String).Trim())
        }
    }
    catch {
        return [pscustomobject]@{
            ExitCode = 1
            Output = [string]$_.Exception.Message
        }
    }
}

$repoCatalog = Join-Path (Split-Path -Parent $PSScriptRoot) "docs\runbook\security-gate-catalog.json"
$repoResult = Invoke-Validator -CatalogPath $repoCatalog
if ($repoResult.ExitCode -ne 0) {
    Write-Host "FAIL repo security gate catalog should pass." -ForegroundColor Red
    Write-Host $repoResult.Output -ForegroundColor Red
    exit 1
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-security-gate-catalog-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $badCatalog = Join-Path $tempRoot "bad-security-gate-catalog.json"
    Write-JsonFile -Path $badCatalog -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "missing check-local entry"
                category = "listener-boundary"
                script = "tools/check-public-listener-auth-guards.ps1"
                check_local_label = "missing label"
                note = "fixture"
            },
            [ordered]@{
                name = "covered child gate"
                category = "gateway-security"
                script = "tools/check-api-gateway-legacy-observation-window.ps1"
                covered_by_script = "tools/check-api-gateway-gates.ps1"
                check_local_label = "api-gateway gates"
                note = "fixture"
            }
        )
    })

    $badResult = Invoke-Validator -CatalogPath $badCatalog
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL catalog with missing check-local label should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("label")) {
        Write-Host "FAIL bad security gate catalog returned unexpected error." -ForegroundColor Red
        Write-Host $badResult.Output -ForegroundColor Red
        exit 1
    }

    $missingGatewayCatalog = Join-Path $tempRoot "missing-gateway-security-catalog.json"
    Write-JsonFile -Path $missingGatewayCatalog -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "DDD boundary imports"
                category = "architecture-boundary"
                script = "tools/check-ddd-boundaries.ps1"
                check_local_label = "ddd boundaries"
                note = "fixture"
            },
            [ordered]@{
                name = "debug listener exposure"
                category = "listener-boundary"
                script = "tools/check-debug-listener-exposure.ps1"
                check_local_label = "debug listener exposure"
                note = "fixture"
            },
            [ordered]@{
                name = "grpc and websocket tls config"
                category = "transport-security"
                script = "tools/check-grpc-tls-config-guards.ps1"
                check_local_label = "grpc/wss tls config guardrails"
                note = "fixture"
            },
            [ordered]@{
                name = "repair operator catalog"
                category = "operator-safety"
                script = "tools/check-repair-operator-index.ps1"
                check_local_label = "repair operator index"
                note = "fixture"
            }
        )
    })

    $missingGatewayResult = Invoke-Validator -CatalogPath $missingGatewayCatalog
    if ($missingGatewayResult.ExitCode -eq 0) {
        Write-Host "FAIL catalog without gateway-security category should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $missingGatewayResult.Output.Contains("gateway-security")) {
        Write-Host "FAIL missing-gateway-security catalog returned unexpected error." -ForegroundColor Red
        Write-Host $missingGatewayResult.Output -ForegroundColor Red
        exit 1
    }

    $missingRequiredGateCatalog = Join-Path $tempRoot "missing-required-repair-gate-catalog.json"
    $repoCatalogObject = Get-Content -LiteralPath $repoCatalog -Raw | ConvertFrom-Json
    Write-JsonFile -Path $missingRequiredGateCatalog -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @($repoCatalogObject.entries | Where-Object { $_.script -ne "tools/check-repair-approval-request.ps1" })
    })

    $missingRequiredGateResult = Invoke-Validator -CatalogPath $missingRequiredGateCatalog
    if ($missingRequiredGateResult.ExitCode -eq 0) {
        Write-Host "FAIL catalog without a required repair gate should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $missingRequiredGateResult.Output.Contains("tools/check-repair-approval-request.ps1")) {
        Write-Host "FAIL missing-required-repair-gate catalog returned unexpected error." -ForegroundColor Red
        Write-Host $missingRequiredGateResult.Output -ForegroundColor Red
        exit 1
    }

    $missingRequiredGatewayGateCatalog = Join-Path $tempRoot "missing-required-gateway-gate-catalog.json"
    Write-JsonFile -Path $missingRequiredGatewayGateCatalog -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @($repoCatalogObject.entries | Where-Object { $_.script -ne "tools/check-api-gateway-legacy-removal-plan.ps1" })
    })

    $missingRequiredGatewayGateResult = Invoke-Validator -CatalogPath $missingRequiredGatewayGateCatalog
    if ($missingRequiredGatewayGateResult.ExitCode -eq 0) {
        Write-Host "FAIL catalog without a required api-gateway gate should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $missingRequiredGatewayGateResult.Output.Contains("tools/check-api-gateway-legacy-removal-plan.ps1")) {
        Write-Host "FAIL missing-required-gateway-gate catalog returned unexpected error." -ForegroundColor Red
        Write-Host $missingRequiredGatewayGateResult.Output -ForegroundColor Red
        exit 1
    }

    $missingRequiredBoundaryGateCatalog = Join-Path $tempRoot "missing-required-boundary-gate-catalog.json"
    Write-JsonFile -Path $missingRequiredBoundaryGateCatalog -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @($repoCatalogObject.entries | Where-Object { $_.script -ne "tools/check-future-service-boundary.ps1" })
    })

    $missingRequiredBoundaryGateResult = Invoke-Validator -CatalogPath $missingRequiredBoundaryGateCatalog
    if ($missingRequiredBoundaryGateResult.ExitCode -eq 0) {
        Write-Host "FAIL catalog without a required architecture boundary gate should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $missingRequiredBoundaryGateResult.Output.Contains("tools/check-future-service-boundary.ps1")) {
        Write-Host "FAIL missing-required-boundary-gate catalog returned unexpected error." -ForegroundColor Red
        Write-Host $missingRequiredBoundaryGateResult.Output -ForegroundColor Red
        exit 1
    }

    $missingRequiredListenerGateCatalog = Join-Path $tempRoot "missing-required-listener-gate-catalog.json"
    Write-JsonFile -Path $missingRequiredListenerGateCatalog -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @($repoCatalogObject.entries | Where-Object { $_.script -ne "tools/check-public-listener-auth-guards.ps1" })
    })

    $missingRequiredListenerGateResult = Invoke-Validator -CatalogPath $missingRequiredListenerGateCatalog
    if ($missingRequiredListenerGateResult.ExitCode -eq 0) {
        Write-Host "FAIL catalog without a required listener/auth guard should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $missingRequiredListenerGateResult.Output.Contains("tools/check-public-listener-auth-guards.ps1")) {
        Write-Host "FAIL missing-required-listener-gate catalog returned unexpected error." -ForegroundColor Red
        Write-Host $missingRequiredListenerGateResult.Output -ForegroundColor Red
        exit 1
    }

    $missingRequiredEvidenceGateCatalog = Join-Path $tempRoot "missing-required-evidence-gate-catalog.json"
    Write-JsonFile -Path $missingRequiredEvidenceGateCatalog -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @($repoCatalogObject.entries | Where-Object { $_.script -ne "tools/check-observability-evidence.ps1" })
    })

    $missingRequiredEvidenceGateResult = Invoke-Validator -CatalogPath $missingRequiredEvidenceGateCatalog
    if ($missingRequiredEvidenceGateResult.ExitCode -eq 0) {
        Write-Host "FAIL catalog without a required evidence gate should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $missingRequiredEvidenceGateResult.Output.Contains("tools/check-observability-evidence.ps1")) {
        Write-Host "FAIL missing-required-evidence-gate catalog returned unexpected error." -ForegroundColor Red
        Write-Host $missingRequiredEvidenceGateResult.Output -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   security gate catalog self-test"
