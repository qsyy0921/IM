$ErrorActionPreference = "Stop"

$writer = Join-Path $PSScriptRoot "write-capacity-longrun-campaign-plan.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

if (-not (Test-Path -LiteralPath $writer -PathType Leaf)) {
    throw "Missing capacity long-run campaign plan writer: $writer"
}

function Invoke-Writer {
    param(
        [string[]]$Arguments
    )

    try {
        $output = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $writer @Arguments 2>&1
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
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-capacity-longrun-plan-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $campaign = "longrun-selftest"

    $goodResult = Invoke-Writer -Arguments @(
        "-OutputRoot", $tempRoot,
        "-CampaignName", $campaign,
        "-Services", "message-service,push-gateway",
        "-Profile", "stack",
        "-WorkloadMode", "soak",
        "-Duration", "30m",
        "-Warmup", "2m",
        "-VUs", "3",
        "-MaxVUs", "5",
        "-TargetEnvironment", "local-fixture",
        "-Operator", "fixture-operator",
        "-Notes", "fixture long-run campaign plan"
    )
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity long-run campaign plan writer should pass." -ForegroundColor Red
        Write-Host $goodResult.Output -ForegroundColor Red
        exit 1
    }

    $planPath = Join-Path $tempRoot "$campaign\capacity-longrun-campaign-plan.json"
    $markdownPath = Join-Path $tempRoot "$campaign\capacity-longrun-campaign-plan.md"
    if (-not (Test-Path -LiteralPath $planPath -PathType Leaf)) {
        Write-Host "FAIL capacity long-run campaign plan JSON was not written." -ForegroundColor Red
        exit 1
    }
    if (-not (Test-Path -LiteralPath $markdownPath -PathType Leaf)) {
        Write-Host "FAIL capacity long-run campaign plan Markdown was not written." -ForegroundColor Red
        exit 1
    }

    $plan = Get-Content -LiteralPath $planPath -Raw | ConvertFrom-Json
    if ($plan.schema_version -ne 1 -or $plan.campaign_name -ne $campaign -or $plan.duration_seconds -ne 1800 -or $plan.service_count -ne 2) {
        Write-Host "FAIL capacity long-run campaign plan has incorrect core fields." -ForegroundColor Red
        exit 1
    }
    if ($plan.scope -notmatch "not a production SLO" -or $plan.output_root -notmatch [regex]::Escape($tempRoot)) {
        Write-Host "FAIL capacity long-run campaign plan missing boundary or output root." -ForegroundColor Red
        exit 1
    }
    $services = @($plan.steps | ForEach-Object { $_.service } | Sort-Object)
    $expected = @("message-service", "push-gateway")
    $diff = Compare-Object -ReferenceObject $expected -DifferenceObject $services
    if ($diff) {
        Write-Host "FAIL capacity long-run campaign plan has wrong services." -ForegroundColor Red
        exit 1
    }
    $messageStep = @($plan.steps | Where-Object { $_.service -eq "message-service" })[0]
    if ($messageStep.runner -ne "sendmessage" -or $messageStep.runner_mode -ne "seeded" -or $messageStep.requires_seed -ne $true) {
        Write-Host "FAIL message-service long-run plan step should be seeded sendmessage." -ForegroundColor Red
        exit 1
    }
    $pushStep = @($plan.steps | Where-Object { $_.service -eq "push-gateway" })[0]
    if ($pushStep.runner -ne "pushgateway" -or $pushStep.runner_mode -ne "stack" -or $pushStep.requires_runtime_stack -ne $true) {
        Write-Host "FAIL push-gateway long-run plan step should be stack pushgateway." -ForegroundColor Red
        exit 1
    }

    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("Capacity Long-Run Campaign Plan") -or -not $markdown.Contains("not a production SLO") -or -not $markdown.Contains("message-service")) {
        Write-Host "FAIL capacity long-run campaign plan Markdown missing expected text." -ForegroundColor Red
        exit 1
    }

    $repoRootResult = Invoke-Writer -Arguments @(
        "-OutputRoot", (Join-Path $repoRoot "tmp-capacity-longrun"),
        "-CampaignName", "bad-repo-root",
        "-Services", "message-service",
        "-Duration", "30m"
    )
    if ($repoRootResult.ExitCode -eq 0 -or -not $repoRootResult.Output.Contains("must not be inside the repository")) {
        Write-Host "FAIL capacity long-run campaign writer must reject repository-local output roots." -ForegroundColor Red
        Write-Host $repoRootResult.Output -ForegroundColor Red
        exit 1
    }

    $shortDurationResult = Invoke-Writer -Arguments @(
        "-OutputRoot", $tempRoot,
        "-CampaignName", "bad-short-duration",
        "-Services", "message-service",
        "-Duration", "10s"
    )
    if ($shortDurationResult.ExitCode -eq 0 -or -not $shortDurationResult.Output.Contains("at least 30m")) {
        Write-Host "FAIL capacity long-run campaign writer must reject short durations." -ForegroundColor Red
        Write-Host $shortDurationResult.Output -ForegroundColor Red
        exit 1
    }

    $unknownServiceResult = Invoke-Writer -Arguments @(
        "-OutputRoot", $tempRoot,
        "-CampaignName", "bad-unknown-service",
        "-Services", "search-service",
        "-Duration", "30m"
    )
    if ($unknownServiceResult.ExitCode -eq 0 -or -not $unknownServiceResult.Output.Contains("Unknown service")) {
        Write-Host "FAIL capacity long-run campaign writer must reject future/unimplemented services." -ForegroundColor Red
        Write-Host $unknownServiceResult.Output -ForegroundColor Red
        exit 1
    }

    $sensitiveResult = Invoke-Writer -Arguments @(
        "-OutputRoot", $tempRoot,
        "-CampaignName", "bad-sensitive-note",
        "-Services", "message-service",
        "-Duration", "30m",
        "-Notes", "token=super-secret-value"
    )
    if ($sensitiveResult.ExitCode -eq 0 -or -not $sensitiveResult.Output.Contains("must be low-sensitive")) {
        Write-Host "FAIL capacity long-run campaign writer must reject sensitive notes." -ForegroundColor Red
        Write-Host $sensitiveResult.Output -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   capacity long-run campaign plan self-test"
