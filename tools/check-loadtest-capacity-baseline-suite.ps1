$ErrorActionPreference = "Stop"

$runner = Join-Path $PSScriptRoot "run-loadtest-capacity-baseline-suite.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

if (-not (Test-Path -LiteralPath $runner -PathType Leaf)) {
    throw "Missing capacity baseline suite runner: $runner"
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-capacity-suite-" + [System.Guid]::NewGuid().ToString("N"))
try {
    $output = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $runner `
        -ResultRoot $tempRoot `
        -RunName "capacity-suite-selftest" `
        -Services "api-gateway,message-service,push-gateway" `
        -DryRun 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL capacity suite dry-run should pass." -ForegroundColor Red
        if ($output) {
            Write-Host (($output | Out-String).Trim()) -ForegroundColor Red
        }
        exit 1
    }

    $summaryPath = Join-Path $tempRoot "capacity-suite-selftest\capacity-baseline-suite-summary.json"
    $markdownPath = Join-Path $tempRoot "capacity-suite-selftest\capacity-baseline-suite-summary.md"
    if (-not (Test-Path -LiteralPath $summaryPath -PathType Leaf)) {
        Write-Host "FAIL capacity suite dry-run did not write summary JSON." -ForegroundColor Red
        exit 1
    }
    if (-not (Test-Path -LiteralPath $markdownPath -PathType Leaf)) {
        Write-Host "FAIL capacity suite dry-run did not write summary Markdown." -ForegroundColor Red
        exit 1
    }

    $summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
    if ($summary.dry_run -ne $true -or $summary.status -ne "dry_run" -or $summary.service_count -ne 3) {
        Write-Host "FAIL capacity suite summary has incorrect dry-run state." -ForegroundColor Red
        exit 1
    }

    $services = @($summary.steps | ForEach-Object { $_.service } | Sort-Object)
    $expected = @("api-gateway", "message-service", "push-gateway")
    $diff = Compare-Object -ReferenceObject $expected -DifferenceObject $services
    if ($diff) {
        Write-Host "FAIL capacity suite summary has wrong service list." -ForegroundColor Red
        exit 1
    }

    $apiStep = @($summary.steps | Where-Object { $_.service -eq "api-gateway" })[0]
    if ($apiStep.command_line -notmatch "--gateway-facade" -or $apiStep.command_line -notmatch "--gateway-auth-mode mock") {
        Write-Host "FAIL api-gateway capacity suite step must use GatewayService facade in mock auth mode." -ForegroundColor Red
        exit 1
    }
    if ($apiStep.command_line -notmatch "--duration 10s" -or $apiStep.command_line -notmatch "--vus 2") {
        Write-Host "FAIL api-gateway capacity suite step must pass duration and VU controls." -ForegroundColor Red
        exit 1
    }
    if ($apiStep.requires_runtime_stack -ne $true -or $apiStep.baseline_mode -ne "requires_stack" -or $apiStep.status -ne "skipped_stack_required") {
        Write-Host "FAIL api-gateway capacity suite step must be marked stack-required by default." -ForegroundColor Red
        exit 1
    }

    $pushStep = @($summary.steps | Where-Object { $_.service -eq "push-gateway" })[0]
    if ($pushStep.command_line -notmatch "--scenario full") {
        Write-Host "FAIL push-gateway capacity suite step must run full scenario." -ForegroundColor Red
        exit 1
    }
    if ($pushStep.command_line -notmatch "--duration 10s" -or $pushStep.command_line -notmatch "--vus 2") {
        Write-Host "FAIL push-gateway capacity suite step must pass duration and VU controls." -ForegroundColor Red
        exit 1
    }
    if ($pushStep.requires_runtime_stack -ne $true -or $pushStep.baseline_mode -ne "requires_stack" -or $pushStep.status -ne "skipped_stack_required") {
        Write-Host "FAIL push-gateway capacity suite step must be marked stack-required by default." -ForegroundColor Red
        exit 1
    }

    $messageStep = @($summary.steps | Where-Object { $_.service -eq "message-service" })[0]
    if ($messageStep.requires_seed -ne $true -or $messageStep.baseline_mode -ne "requires_seed" -or $messageStep.status -ne "skipped_seed_required") {
        Write-Host "FAIL message-service capacity suite step must be marked seeded-only by default." -ForegroundColor Red
        exit 1
    }
    if ($messageStep.command_line -notmatch "--tenant-id tenant-capacity-message" -or $messageStep.command_line -notmatch "--conversation-prefix conv-capacity-message") {
        Write-Host "FAIL message-service capacity suite step must use capacity seed fixture ids." -ForegroundColor Red
        exit 1
    }

    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("Loadtest Capacity Baseline Suite") -or -not $markdown.Contains("not a production SLO")) {
        Write-Host "FAIL capacity suite markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }
    if ($markdown.Contains('$(@')) {
        Write-Host "FAIL capacity suite markdown contains unevaluated PowerShell interpolation." -ForegroundColor Red
        exit 1
    }

    $seededRoot = Join-Path $tempRoot "seeded"
    $seededOutput = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $runner `
        -ResultRoot $seededRoot `
        -RunName "capacity-suite-seeded-selftest" `
        -Services "delivery-service" `
        -DryRun 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL capacity suite seeded dry-run should pass." -ForegroundColor Red
        if ($seededOutput) {
            Write-Host (($seededOutput | Out-String).Trim()) -ForegroundColor Red
        }
        exit 1
    }

    $seededSummaryPath = Join-Path $seededRoot "capacity-suite-seeded-selftest\capacity-baseline-suite-summary.json"
    $seededSummary = Get-Content -LiteralPath $seededSummaryPath -Raw | ConvertFrom-Json
    $deliveryStep = @($seededSummary.steps | Where-Object { $_.service -eq "delivery-service" })[0]
    if ($deliveryStep.requires_seed -ne $true -or $deliveryStep.baseline_mode -ne "requires_seed" -or $deliveryStep.status -ne "skipped_seed_required") {
        Write-Host "FAIL delivery-service capacity runner must be marked seeded-only by default." -ForegroundColor Red
        exit 1
    }
    if ($deliveryStep.command_line -notmatch "--tenant-id tenant-capacity-delivery" -or
        $deliveryStep.command_line -notmatch "--conversation-id conv-capacity-delivery" -or
        $deliveryStep.command_line -notmatch "--user-id delivery-user-1") {
        Write-Host "FAIL delivery-service capacity suite step must use capacity seed fixture ids." -ForegroundColor Red
        exit 1
    }

    $conversationRoot = Join-Path $tempRoot "conversation"
    $conversationOutput = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $runner `
        -ResultRoot $conversationRoot `
        -RunName "capacity-suite-conversation-selftest" `
        -Services "conversation-service" `
        -DryRun 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL capacity suite conversation dry-run should pass." -ForegroundColor Red
        if ($conversationOutput) {
            Write-Host (($conversationOutput | Out-String).Trim()) -ForegroundColor Red
        }
        exit 1
    }

    $conversationSummaryPath = Join-Path $conversationRoot "capacity-suite-conversation-selftest\capacity-baseline-suite-summary.json"
    $conversationSummary = Get-Content -LiteralPath $conversationSummaryPath -Raw | ConvertFrom-Json
    $conversationStep = @($conversationSummary.steps | Where-Object { $_.service -eq "conversation-service" })[0]
    if ($conversationStep.requires_seed -ne $true -or $conversationStep.baseline_mode -ne "requires_seed" -or $conversationStep.status -ne "skipped_seed_required") {
        Write-Host "FAIL conversation-service capacity runner must be marked seeded-only by default." -ForegroundColor Red
        exit 1
    }
    if ($conversationStep.command_line -notmatch "--tenant-id tenant-capacity-conversation" -or
        $conversationStep.command_line -notmatch "--conversation-id conv-capacity-memberchange" -or
        $conversationStep.command_line -notmatch "--operator-user-id owner-1") {
        Write-Host "FAIL conversation-service capacity suite step must use capacity seed fixture ids." -ForegroundColor Red
        exit 1
    }

    $includeSeededRoot = Join-Path $tempRoot "include-seeded"
    $includeSeededOutput = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $runner `
        -ResultRoot $includeSeededRoot `
        -RunName "capacity-suite-include-seeded-selftest" `
        -Services "delivery-service" `
        -IncludeSeededRunners `
        -DryRun 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL capacity suite include-seeded dry-run should pass." -ForegroundColor Red
        if ($includeSeededOutput) {
            Write-Host (($includeSeededOutput | Out-String).Trim()) -ForegroundColor Red
        }
        exit 1
    }

    $includeSeededSummaryPath = Join-Path $includeSeededRoot "capacity-suite-include-seeded-selftest\capacity-baseline-suite-summary.json"
    $includeSeededSummary = Get-Content -LiteralPath $includeSeededSummaryPath -Raw | ConvertFrom-Json
    $includeSeededStep = @($includeSeededSummary.steps | Where-Object { $_.service -eq "delivery-service" })[0]
    if ($includeSeededSummary.include_seeded_runners -ne $true -or $includeSeededStep.status -ne "dry_run") {
        Write-Host "FAIL delivery-service capacity runner should be planned when seeded runners are enabled." -ForegroundColor Red
        exit 1
    }

    $includeStackRoot = Join-Path $tempRoot "include-stack"
    $includeStackOutput = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $runner `
        -ResultRoot $includeStackRoot `
        -RunName "capacity-suite-include-stack-selftest" `
        -Services "api-gateway,identity-service,push-gateway,contacts-service,receipt-service" `
        -IncludeStackRunners `
        -DryRun 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL capacity suite include-stack dry-run should pass." -ForegroundColor Red
        if ($includeStackOutput) {
            Write-Host (($includeStackOutput | Out-String).Trim()) -ForegroundColor Red
        }
        exit 1
    }

    $includeStackSummaryPath = Join-Path $includeStackRoot "capacity-suite-include-stack-selftest\capacity-baseline-suite-summary.json"
    $includeStackSummary = Get-Content -LiteralPath $includeStackSummaryPath -Raw | ConvertFrom-Json
    $stackStatuses = @($includeStackSummary.steps | ForEach-Object { $_.status } | Sort-Object -Unique)
    if ($includeStackSummary.include_stack_runners -ne $true -or $stackStatuses.Count -ne 1 -or $stackStatuses[0] -ne "dry_run") {
        Write-Host "FAIL stack-required capacity runners should be planned when stack runners are enabled." -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   loadtest capacity baseline suite self-test"
