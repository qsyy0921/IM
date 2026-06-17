$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"

if (-not (Test-Path -LiteralPath $plannerPath -PathType Leaf)) {
    throw "Missing repair operator plan writer: $plannerPath"
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-repair-operator-plan-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

try {
    $messagePlanPath = Join-Path $tempRoot "message-plan.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "message-service" `
        -Mode "outbox-repair-cleanup" `
        -DryRun `
        -OutputEnv "NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_OUTPUT" `
        -OutputPath "H:\NexusIM\operator-plans\message-cleanup.json" `
        -PlanOutputPath $messagePlanPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed for message-service"
    }

    $messagePlan = Get-Content -LiteralPath $messagePlanPath -Raw | ConvertFrom-Json
    if ($messagePlan.executes -ne $false -or $messagePlan.service -ne "message-service" -or $messagePlan.mode -ne "outbox-repair-cleanup") {
        throw "message-service repair operator plan has unexpected identity fields."
    }
    if ($messagePlan.environment.NEXUSIM_MESSAGE_SERVICE_MODE -ne "outbox-repair-cleanup" -or
        $messagePlan.environment.NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_DRY_RUN -ne "true" -or
        $messagePlan.environment.NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_OUTPUT -ne "H:\NexusIM\operator-plans\message-cleanup.json") {
        throw "message-service repair operator plan has unexpected environment."
    }

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $ambiguousOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
            -Service "delivery-service" `
            -Mode "projection-checkpoint-repair" `
            -DryRun 2>&1
        $ambiguousExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    if ($ambiguousExitCode -eq 0 -or ($ambiguousOutput -join "`n") -notmatch "DryRun requires DryRunEnv") {
        throw "delivery-service ambiguous dry-run plan should require explicit DryRunEnv."
    }

    $deliveryPlanJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "delivery-service" `
        -Mode "projection-checkpoint-repair" `
        -DryRun `
        -DryRunEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN" `
        -Env "NEXUSIM_DELIVERY_PROJECTION_REPAIR_OPERATOR=manual"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed for delivery-service"
    }
    $deliveryPlan = $deliveryPlanJson | ConvertFrom-Json
    if ($deliveryPlan.environment.NEXUSIM_DELIVERY_SERVICE_MODE -ne "projection-checkpoint-repair" -or
        $deliveryPlan.environment.NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN -ne "true" -or
        $deliveryPlan.environment.NEXUSIM_DELIVERY_PROJECTION_REPAIR_OPERATOR -ne "manual") {
        throw "delivery-service repair operator plan has unexpected environment."
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   repair operator plan writer self-test"
