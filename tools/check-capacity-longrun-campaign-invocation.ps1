$ErrorActionPreference = "Stop"

$writer = Join-Path $PSScriptRoot "write-capacity-longrun-campaign-plan.ps1"
$invoker = Join-Path $PSScriptRoot "invoke-capacity-longrun-campaign.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

foreach ($path in @($writer, $invoker)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing capacity long-run campaign invocation dependency: $path"
    }
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

$repoRoot = Split-Path -Parent $PSScriptRoot
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-capacity-longrun-invoke-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $campaign = "invoke-selftest"

    $planResult = Invoke-Tool -Path $writer -Arguments @(
        "-OutputRoot", $tempRoot,
        "-CampaignName", $campaign,
        "-Services", "message-service,policy-service",
        "-Duration", "30m",
        "-Warmup", "2m",
        "-VUs", "2",
        "-MaxVUs", "4",
        "-TargetEnvironment", "fixture",
        "-Operator", "fixture-operator",
        "-Notes", "fixture campaign invocation plan"
    )
    if ($planResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity long-run invocation fixture plan should be written." -ForegroundColor Red
        Write-Host $planResult.Output -ForegroundColor Red
        exit 1
    }

    $planPath = Join-Path $tempRoot "$campaign\capacity-longrun-campaign-plan.json"
    $invokeResult = Invoke-Tool -Path $invoker -Arguments @(
        "-PlanPath", $planPath,
        "-DryRun",
        "-SkipSummary",
        "-PGDSN", "postgres://fixture:fixture@127.0.0.1:5432/fixture?sslmode=disable",
        "-KafkaBrokers", "127.0.0.1:9092"
    )
    if ($invokeResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity long-run campaign invocation dry-run should pass." -ForegroundColor Red
        Write-Host $invokeResult.Output -ForegroundColor Red
        exit 1
    }

    $suiteSummaryPath = Join-Path $tempRoot "$campaign\capacity-baseline-suite-summary.json"
    if (-not (Test-Path -LiteralPath $suiteSummaryPath -PathType Leaf)) {
        Write-Host "FAIL capacity long-run invocation should write suite summary in campaign directory." -ForegroundColor Red
        exit 1
    }
    $summary = Get-Content -LiteralPath $suiteSummaryPath -Raw | ConvertFrom-Json
    if ($summary.run_name -ne $campaign -or $summary.dry_run -ne $true -or $summary.status -ne "dry_run") {
        Write-Host "FAIL capacity long-run invocation suite summary has wrong dry-run fields." -ForegroundColor Red
        exit 1
    }
    if ($summary.scope -notmatch "not a production SLO" -or $summary.include_seeded_runners -ne $true -or $summary.include_stack_runners -ne $true) {
        Write-Host "FAIL capacity long-run invocation should keep non-SLO boundary and include planned runner classes by default." -ForegroundColor Red
        exit 1
    }
    $messageStep = @($summary.steps | Where-Object { $_.service -eq "message-service" })[0]
    if ($messageStep.status -ne "dry_run" -or $messageStep.command_line -notmatch "--duration 30m") {
        Write-Host "FAIL capacity long-run invocation should preserve plan duration in runner command." -ForegroundColor Red
        exit 1
    }
    if (Test-Path -LiteralPath (Join-Path $tempRoot "$campaign\capacity-longrun-campaign-preflight.json")) {
        Write-Host "FAIL capacity long-run invocation dry-run should not perform network preflight." -ForegroundColor Red
        exit 1
    }

    $repoPlanResult = Invoke-Tool -Path $writer -Arguments @(
        "-OutputRoot", $tempRoot,
        "-CampaignName", "repo-plan-fixture",
        "-Services", "policy-service",
        "-Duration", "30m"
    )
    if ($repoPlanResult.ExitCode -ne 0) {
        Write-Host "FAIL repository-local plan fixture should be written under temp root." -ForegroundColor Red
        Write-Host $repoPlanResult.Output -ForegroundColor Red
        exit 1
    }
    $repoPlanPath = Join-Path $tempRoot "repo-plan-fixture\capacity-longrun-campaign-plan.json"
    $repoCopyPath = Join-Path $repoRoot "tmp-capacity-longrun-plan.json"
    Copy-Item -LiteralPath $repoPlanPath -Destination $repoCopyPath -Force
    try {
        $badInvokeResult = Invoke-Tool -Path $invoker -Arguments @(
            "-PlanPath", $repoCopyPath,
            "-DryRun"
        )
        if ($badInvokeResult.ExitCode -eq 0 -or -not $badInvokeResult.Output.Contains("PlanPath must stay under plan output_root")) {
            Write-Host "FAIL capacity long-run invocation must reject repository-local copied plans." -ForegroundColor Red
            Write-Host $badInvokeResult.Output -ForegroundColor Red
            exit 1
        }
    }
    finally {
        if (Test-Path -LiteralPath $repoCopyPath) {
            Remove-Item -LiteralPath $repoCopyPath -Force
        }
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   capacity long-run campaign invocation self-test"
