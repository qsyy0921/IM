param(
    [switch]$SkipPowerShellParser,
    [switch]$SkipShellParser
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
    function Invoke-LocalCheck {
        param(
            [string]$ScriptName
        )

        $scriptPath = Join-Path $PSScriptRoot $ScriptName
        & powershell -NoProfile -ExecutionPolicy Bypass -File $scriptPath
        if ($LASTEXITCODE -ne 0) {
            throw "$ScriptName failed with exit code $LASTEXITCODE"
        }
    }

    Write-Host "== runbook entrypoints =="
    Invoke-LocalCheck "check-runbook-entrypoints.ps1"

    Write-Host "== service brief sync =="
    Invoke-LocalCheck "check-service-brief-sync.ps1"

    Write-Host "== future service boundary =="
    Invoke-LocalCheck "check-future-service-boundary.ps1"

    Write-Host "== check-local coverage =="
    Invoke-LocalCheck "check-local-coverage.ps1"

    Write-Host "== runbook consistency =="
    Invoke-LocalCheck "check-runbook-consistency.ps1"

    Write-Host "== security gate catalog =="
    Invoke-LocalCheck "check-security-gate-catalog.ps1"

    Write-Host "== repair operator index =="
    Invoke-LocalCheck "check-repair-operator-index.ps1"

    Write-Host "== repair operator safety helper =="
    Invoke-LocalCheck "check-repair-operator-safety.ps1"

    Write-Host "== repair operator plan writer =="
    Invoke-LocalCheck "check-repair-operator-plan.ps1"

    Write-Host "== repair operator catalog plannable =="
    Invoke-LocalCheck "check-repair-operator-catalog-plannable.ps1"

    Write-Host "== repair approval request =="
    Invoke-LocalCheck "check-repair-approval-request.ps1"

    Write-Host "== repair approval decision =="
    Invoke-LocalCheck "check-repair-approval-decision.ps1"

    Write-Host "== repair approval chain =="
    Invoke-LocalCheck "check-repair-approval-chain.ps1"

    Write-Host "== approved repair invocation =="
    Invoke-LocalCheck "check-approved-repair-invocation.ps1"

    Write-Host "== repair batch manifest =="
    Invoke-LocalCheck "check-repair-batch-manifest.ps1"

    Write-Host "== repair audit bundle =="
    Invoke-LocalCheck "check-repair-audit-bundle.ps1"

    Write-Host "== message compliance proof manifest =="
    Invoke-LocalCheck "check-message-compliance-proof-manifest.ps1"

    Write-Host "== ddd boundaries =="
    Invoke-LocalCheck "check-ddd-boundaries.ps1"

    Write-Host "== cross-service table access =="
    Invoke-LocalCheck "check-cross-service-table-access.ps1"

    Write-Host "== docs entrypoints =="
    Invoke-LocalCheck "check-doc-entrypoints.ps1"

    Write-Host "== service cmd tests =="
    Invoke-LocalCheck "check-service-cmd-tests.ps1"

    Write-Host "== service cmd builds =="
    Invoke-LocalCheck "check-service-cmd-builds.ps1"

    Write-Host "== service linux builds =="
    Invoke-LocalCheck "check-service-linux-builds.ps1"

    Write-Host "== service package tests =="
    Invoke-LocalCheck "check-service-package-tests.ps1"

    Write-Host "== service runtime endpoints =="
    Invoke-LocalCheck "check-service-runtime-endpoints.ps1"

    Write-Host "== docker runtime coverage =="
    Invoke-LocalCheck "check-docker-runtime-coverage.ps1"

    Write-Host "== local docker image build script coverage =="
    Invoke-LocalCheck "check-service-docker-image-build-script.ps1"

    Write-Host "== mac docker image sync coverage =="
    Invoke-LocalCheck "check-mac-service-docker-sync.ps1"

    Write-Host "== local service compose =="
    Invoke-LocalCheck "check-local-service-compose.ps1"

    Write-Host "== project naming =="
    Invoke-LocalCheck "check-project-naming.ps1"

    Write-Host "== project naming self-test =="
    Invoke-LocalCheck "check-project-naming-selftest.ps1"

    Write-Host "== file size budgets =="
    Invoke-LocalCheck "check-file-size-budget.ps1"

    Write-Host "== file size budget summary =="
    Invoke-LocalCheck "check-file-size-budget-summary.ps1"

    Write-Host "== file size hotspot baseline =="
    Invoke-LocalCheck "check-file-size-hotspot-baseline.ps1"

    Write-Host "== loadtest output paths =="
    Invoke-LocalCheck "check-loadtest-output-paths.ps1"

    Write-Host "== loadtest output path guard self-test =="
    Invoke-LocalCheck "check-loadtest-output-paths-selftest.ps1"

    Write-Host "== output root safety helper =="
    Invoke-LocalCheck "check-output-root-safety.ps1"

    Write-Host "== evidence metadata safety helper =="
    Invoke-LocalCheck "check-evidence-metadata-safety.ps1"

    Write-Host "== loadtest capacity summaries =="
    Invoke-LocalCheck "check-loadtest-capacity-summaries.ps1"

    Write-Host "== loadtest capacity baseline summary =="
    Invoke-LocalCheck "check-loadtest-capacity-baseline-summary.ps1"

    Write-Host "== loadtest capacity baseline suite =="
    Invoke-LocalCheck "check-loadtest-capacity-baseline-suite.ps1"

    Write-Host "== capacity long-run campaign plan =="
    Invoke-LocalCheck "check-capacity-longrun-campaign-plan.ps1"

    Write-Host "== capacity long-run campaign evidence =="
    Invoke-LocalCheck "check-capacity-longrun-campaign-evidence.ps1"

    Write-Host "== capacity baseline evidence =="
    Invoke-LocalCheck "check-capacity-baseline-evidence.ps1"

    Write-Host "== resource snapshot summary =="
    Invoke-LocalCheck "check-resource-snapshot-summary.ps1"

    Write-Host "== resource snapshot evidence =="
    Invoke-LocalCheck "check-resource-snapshot-evidence.ps1"

    Write-Host "== distributed smoke evidence =="
    Invoke-LocalCheck "check-distributed-smoke-evidence.ps1"

    Write-Host "== observability smoke summary =="
    Invoke-LocalCheck "check-observability-smoke-summary.ps1"

    Write-Host "== observability target smoke =="
    Invoke-LocalCheck "check-observability-target-smoke.ps1"

    Write-Host "== observability evidence =="
    Invoke-LocalCheck "check-observability-evidence.ps1"

    Write-Host "== local observability image preflight =="
    Invoke-LocalCheck "check-local-observability-images.ps1"

    Write-Host "== local observability image prepare =="
    Invoke-LocalCheck "check-local-observability-image-prepare.ps1"

    Write-Host "== local prometheus config =="
    Invoke-LocalCheck "check-local-prometheus-config.ps1"

    Write-Host "== local alertmanager config =="
    Invoke-LocalCheck "check-local-alertmanager-config.ps1"

    Write-Host "== local grafana config =="
    Invoke-LocalCheck "check-local-grafana-config.ps1"

    Write-Host "== api-gateway gates =="
    Invoke-LocalCheck "check-api-gateway-gates.ps1"

    Write-Host "== otel sampling policy =="
    Invoke-LocalCheck "check-otel-sampling-policy.ps1"

    Write-Host "== otel service wiring =="
    Invoke-LocalCheck "check-otel-service-wiring.ps1"

    Write-Host "== otel span attributes =="
    Invoke-LocalCheck "check-otel-span-attributes.ps1"

    Write-Host "== grpc correlation logs =="
    Invoke-LocalCheck "check-grpc-correlation-logs.ps1"

    Write-Host "== debug listener exposure =="
    Invoke-LocalCheck "check-debug-listener-exposure.ps1"

    Write-Host "== public listener auth boundaries =="
    Invoke-LocalCheck "check-public-listener-auth-guards.ps1"

    Write-Host "== grpc/wss tls config guardrails =="
    Invoke-LocalCheck "check-grpc-tls-config-guards.ps1"

    Write-Host "== kafka producer config =="
    Invoke-LocalCheck "check-kafka-producer-config.ps1"

    Write-Host "== kafka producer config summary =="
    Invoke-LocalCheck "check-kafka-producer-config-summary.ps1"

    Write-Host "== kafka isr observation summary =="
    Invoke-LocalCheck "check-kafka-isr-observation-summary.ps1"

    Write-Host "== kafka isr flapping summary =="
    Invoke-LocalCheck "check-kafka-isr-flapping-summary.ps1"

    Write-Host "== kafka producer hardening summary =="
    Invoke-LocalCheck "check-kafka-producer-hardening-summary.ps1"

    Write-Host "== kafka producer fault summary =="
    Invoke-LocalCheck "check-kafka-producer-fault-summary.ps1"

    Write-Host "== kafka consumer rebalance summary =="
    Invoke-LocalCheck "check-kafka-consumer-rebalance-summary.ps1"

    Write-Host "== kafka consumer churn summary =="
    Invoke-LocalCheck "check-kafka-consumer-churn-summary.ps1"

    Write-Host "== redis smoke summary =="
    Invoke-LocalCheck "check-redis-smoke-summary.ps1"

    Write-Host "== postgres smoke summary =="
    Invoke-LocalCheck "check-postgres-smoke-summary.ps1"

    Write-Host "== git whitespace =="
    git diff --check
    if ($LASTEXITCODE -ne 0) {
        throw "git diff --check failed with exit code $LASTEXITCODE"
    }
    git diff --cached --check
    if ($LASTEXITCODE -ne 0) {
        throw "git diff --cached --check failed with exit code $LASTEXITCODE"
    }

    if (-not $SkipPowerShellParser) {
        Write-Host "== powershell parser =="
        Invoke-LocalCheck "check-powershell-scripts.ps1"
    }

    if (-not $SkipShellParser) {
        Write-Host "== shell parser =="
        Invoke-LocalCheck "check-shell-scripts.ps1"
    }
}
finally {
    Pop-Location
}
