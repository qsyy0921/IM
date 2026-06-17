$ErrorActionPreference = "Stop"

$writer = Join-Path $PSScriptRoot "write-capacity-longrun-campaign-plan.ps1"
$summarizer = Join-Path $PSScriptRoot "summarize-capacity-longrun-campaign.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

foreach ($path in @($writer, $summarizer)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing capacity long-run campaign summary dependency: $path"
    }
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
    $Value | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Invoke-Tool {
    param(
        [string]$Path,
        [string[]]$Arguments
    )

    try {
        $output = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $Path @Arguments 2>&1
        return [pscustomobject]@{
            ExitCode = $LASTEXITCODE
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

function Write-StepSummary {
    param(
        [object]$Step,
        [double]$DurationSeconds,
        [double]$Rate = 2.5,
        [bool]$Success = $true
    )

    $summaryPath = Join-Path ([string]$Step.result_dir) "$($Step.service)-summary.json"
    Write-JsonFile -Path $summaryPath -Value ([ordered]@{
        service = [string]$Step.service
        runner = [string]$Step.runner
        success = $Success
        capacity_summary = [ordered]@{
            duration_seconds = $DurationSeconds
            accepted_rps = $Rate
            success_count = [int]($DurationSeconds * $Rate)
        }
    })
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-capacity-longrun-summary-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $campaignRoot = Join-Path $tempRoot "campaigns"
    $reportRoot = Join-Path $tempRoot "reports"
    $campaign = "summary-selftest"

    $planResult = Invoke-Tool -Path $writer -Arguments @(
        "-OutputRoot", $campaignRoot,
        "-CampaignName", $campaign,
        "-Services", "message-service,push-gateway",
        "-Profile", "stack",
        "-WorkloadMode", "soak",
        "-Duration", "30m",
        "-Warmup", "2m",
        "-VUs", "2",
        "-MaxVUs", "4",
        "-TargetEnvironment", "fixture",
        "-Operator", "fixture-operator",
        "-Notes", "fixture campaign plan"
    )
    if ($planResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity long-run campaign plan fixture should be written." -ForegroundColor Red
        Write-Host $planResult.Output -ForegroundColor Red
        exit 1
    }

    $planPath = Join-Path $campaignRoot "$campaign\capacity-longrun-campaign-plan.json"
    $plan = Get-Content -LiteralPath $planPath -Raw | ConvertFrom-Json
    foreach ($step in @($plan.steps)) {
        Write-StepSummary -Step $step -DurationSeconds 1800 -Rate 3.0
    }

    $summaryPath = Join-Path $campaignRoot "$campaign\capacity-longrun-campaign-summary.json"
    $reportPath = Join-Path $reportRoot "$campaign-report.md"
    $goodResult = Invoke-Tool -Path $summarizer -Arguments @(
        "-PlanPath", $planPath,
        "-SummaryPath", $summaryPath,
        "-ReportRoot", $reportRoot,
        "-ReportPath", $reportPath
    )
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity long-run campaign summarizer should pass for complete fixture." -ForegroundColor Red
        Write-Host $goodResult.Output -ForegroundColor Red
        exit 1
    }

    $summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
    if ($summary.schema_version -ne 1 -or $summary.status -ne "completed" -or $summary.service_count -ne 2 -or $summary.completed_service_count -ne 2) {
        Write-Host "FAIL capacity long-run campaign summary has incorrect completion fields." -ForegroundColor Red
        exit 1
    }
    if ($summary.minimum_duration_seconds -lt 1800 -or $summary.scope -notmatch "not a production SLO") {
        Write-Host "FAIL capacity long-run campaign summary missing duration or boundary." -ForegroundColor Red
        exit 1
    }
    $report = Get-Content -LiteralPath $reportPath -Raw
    if (-not $report.Contains("Capacity Long-Run Campaign Report") -or -not $report.Contains("not a production SLO") -or -not $report.Contains("message-service")) {
        Write-Host "FAIL capacity long-run campaign report missing expected text." -ForegroundColor Red
        exit 1
    }

    $missingCampaign = "summary-missing"
    $missingPlanResult = Invoke-Tool -Path $writer -Arguments @(
        "-OutputRoot", $campaignRoot,
        "-CampaignName", $missingCampaign,
        "-Services", "message-service,push-gateway",
        "-Duration", "30m"
    )
    if ($missingPlanResult.ExitCode -ne 0) {
        Write-Host "FAIL missing summary fixture plan should be written." -ForegroundColor Red
        Write-Host $missingPlanResult.Output -ForegroundColor Red
        exit 1
    }
    $missingPlanPath = Join-Path $campaignRoot "$missingCampaign\capacity-longrun-campaign-plan.json"
    $missingPlan = Get-Content -LiteralPath $missingPlanPath -Raw | ConvertFrom-Json
    $firstStep = @($missingPlan.steps)[0]
    Write-StepSummary -Step $firstStep -DurationSeconds 1800 -Rate 1.0
    $missingResult = Invoke-Tool -Path $summarizer -Arguments @(
        "-PlanPath", $missingPlanPath,
        "-ReportRoot", $reportRoot
    )
    if ($missingResult.ExitCode -eq 0 -or -not $missingResult.Output.Contains("incomplete")) {
        Write-Host "FAIL campaign summary should fail when a service summary is missing." -ForegroundColor Red
        Write-Host $missingResult.Output -ForegroundColor Red
        exit 1
    }
    $subsetSummaryPath = Join-Path $campaignRoot "$missingCampaign\capacity-longrun-campaign-subset-summary.json"
    $subsetReportPath = Join-Path $reportRoot "$missingCampaign-subset-report.md"
    $subsetResult = Invoke-Tool -Path $summarizer -Arguments @(
        "-PlanPath", $missingPlanPath,
        "-SummaryPath", $subsetSummaryPath,
        "-ReportRoot", $reportRoot,
        "-ReportPath", $subsetReportPath,
        "-Services", ([string]$firstStep.service)
    )
    if ($subsetResult.ExitCode -ne 0) {
        Write-Host "FAIL campaign summary should pass when limited to a completed service subset." -ForegroundColor Red
        Write-Host $subsetResult.Output -ForegroundColor Red
        exit 1
    }
    $subsetSummary = Get-Content -LiteralPath $subsetSummaryPath -Raw | ConvertFrom-Json
    if ($subsetSummary.status -ne "completed" -or $subsetSummary.service_count -ne 1 -or $subsetSummary.plan_service_count -ne 2) {
        Write-Host "FAIL campaign subset summary has incorrect service counts." -ForegroundColor Red
        exit 1
    }

    $shortCampaign = "summary-short"
    $shortPlanResult = Invoke-Tool -Path $writer -Arguments @(
        "-OutputRoot", $campaignRoot,
        "-CampaignName", $shortCampaign,
        "-Services", "message-service",
        "-Duration", "30m"
    )
    if ($shortPlanResult.ExitCode -ne 0) {
        Write-Host "FAIL short duration fixture plan should be written." -ForegroundColor Red
        Write-Host $shortPlanResult.Output -ForegroundColor Red
        exit 1
    }
    $shortPlanPath = Join-Path $campaignRoot "$shortCampaign\capacity-longrun-campaign-plan.json"
    $shortPlan = Get-Content -LiteralPath $shortPlanPath -Raw | ConvertFrom-Json
    Write-StepSummary -Step (@($shortPlan.steps)[0]) -DurationSeconds 1799 -Rate 1.0
    $shortResult = Invoke-Tool -Path $summarizer -Arguments @(
        "-PlanPath", $shortPlanPath,
        "-ReportRoot", $reportRoot
    )
    if ($shortResult.ExitCode -eq 0 -or -not $shortResult.Output.Contains("incomplete")) {
        Write-Host "FAIL campaign summary should fail when duration is below 30m." -ForegroundColor Red
        Write-Host $shortResult.Output -ForegroundColor Red
        exit 1
    }

    $repoLocalSummaryResult = Invoke-Tool -Path $summarizer -Arguments @(
        "-PlanPath", $planPath,
        "-SummaryPath", (Join-Path $repoRoot "tmp-longrun-summary.json"),
        "-ReportRoot", $reportRoot
    )
    if ($repoLocalSummaryResult.ExitCode -eq 0 -or -not $repoLocalSummaryResult.Output.Contains("SummaryPath must stay under plan output_root")) {
        Write-Host "FAIL campaign summary should reject repository-local summary path." -ForegroundColor Red
        Write-Host $repoLocalSummaryResult.Output -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   capacity long-run campaign summary self-test"
