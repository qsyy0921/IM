$ErrorActionPreference = "Stop"

$writer = Join-Path $PSScriptRoot "write-capacity-longrun-campaign-plan.ps1"
$preflight = Join-Path $PSScriptRoot "test-capacity-longrun-campaign-preflight.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

foreach ($path in @($writer, $preflight)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing capacity long-run campaign preflight dependency: $path"
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
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-capacity-longrun-preflight-" + [System.Guid]::NewGuid().ToString("N"))
$listener = $null
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $campaign = "preflight-selftest"

    $planResult = Invoke-Tool -Path $writer -Arguments @(
        "-OutputRoot", $tempRoot,
        "-CampaignName", $campaign,
        "-Services", "policy-service,contacts-service",
        "-Duration", "30m",
        "-Warmup", "2m",
        "-VUs", "2",
        "-MaxVUs", "4",
        "-TargetEnvironment", "fixture",
        "-Operator", "fixture-operator",
        "-Notes", "fixture campaign preflight plan"
    )
    if ($planResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity long-run preflight fixture plan should be written." -ForegroundColor Red
        Write-Host $planResult.Output -ForegroundColor Red
        exit 1
    }

    $planPath = Join-Path $tempRoot "$campaign\capacity-longrun-campaign-plan.json"
    $skipResult = Invoke-Tool -Path $preflight -Arguments @(
        "-PlanPath", $planPath,
        "-Services", "policy-service",
        "-SkipNetworkChecks",
        "-PGDSN", "postgres://fixture:fixture@127.0.0.1:5432/fixture?sslmode=disable",
        "-KafkaBrokers", "127.0.0.1:9092"
    )
    if ($skipResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity long-run preflight skip-network mode should pass." -ForegroundColor Red
        Write-Host $skipResult.Output -ForegroundColor Red
        exit 1
    }
    $preflightPath = Join-Path $tempRoot "$campaign\capacity-longrun-campaign-preflight.json"
    if (-not (Test-Path -LiteralPath $preflightPath -PathType Leaf)) {
        Write-Host "FAIL capacity long-run preflight should write default summary under campaign directory." -ForegroundColor Red
        exit 1
    }
    $summary = Get-Content -LiteralPath $preflightPath -Raw | ConvertFrom-Json
    if ($summary.status -ne "passed" -or $summary.skip_network_checks -ne $true -or $summary.endpoint_count -ne 1) {
        Write-Host "FAIL capacity long-run preflight summary has wrong skip-network fields." -ForegroundColor Red
        exit 1
    }
    if ($summary.scope -notmatch "not a production SLO") {
        Write-Host "FAIL capacity long-run preflight summary must keep non-SLO boundary." -ForegroundColor Red
        exit 1
    }

    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Parse("127.0.0.1"), 0)
    $listener.Start()
    $port = $listener.LocalEndpoint.Port
    $liveCampaign = "preflight-live"
    $livePlanResult = Invoke-Tool -Path $writer -Arguments @(
        "-OutputRoot", $tempRoot,
        "-CampaignName", $liveCampaign,
        "-Services", "policy-service",
        "-Duration", "30m",
        "-Warmup", "2m",
        "-VUs", "1",
        "-MaxVUs", "1"
    )
    if ($livePlanResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity long-run live preflight fixture plan should be written." -ForegroundColor Red
        Write-Host $livePlanResult.Output -ForegroundColor Red
        exit 1
    }
    $livePlanPath = Join-Path $tempRoot "$liveCampaign\capacity-longrun-campaign-plan.json"
    $liveResult = Invoke-Tool -Path $preflight -Arguments @(
        "-PlanPath", $livePlanPath,
        "-PolicyTarget", "127.0.0.1:$port",
        "-TimeoutMilliseconds", "1000"
    )
    if ($liveResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity long-run preflight should pass for reachable TCP endpoint." -ForegroundColor Red
        Write-Host $liveResult.Output -ForegroundColor Red
        exit 1
    }

    $badResult = Invoke-Tool -Path $preflight -Arguments @(
        "-PlanPath", $livePlanPath,
        "-PolicyTarget", "127.0.0.1:1",
        "-TimeoutMilliseconds", "250"
    )
    if ($badResult.ExitCode -eq 0 -or -not $badResult.Output.Contains("FAIL policy-service")) {
        Write-Host "FAIL capacity long-run preflight should fail for unreachable TCP endpoint." -ForegroundColor Red
        Write-Host $badResult.Output -ForegroundColor Red
        exit 1
    }

    $repoCopyPath = Join-Path $repoRoot "tmp-capacity-longrun-preflight-plan.json"
    Copy-Item -LiteralPath $livePlanPath -Destination $repoCopyPath -Force
    try {
        $badPlanResult = Invoke-Tool -Path $preflight -Arguments @(
            "-PlanPath", $repoCopyPath,
            "-Services", "policy-service",
            "-SkipNetworkChecks"
        )
        if ($badPlanResult.ExitCode -eq 0 -or -not $badPlanResult.Output.Contains("PlanPath must stay under plan output_root")) {
            Write-Host "FAIL capacity long-run preflight must reject repository-local copied plans." -ForegroundColor Red
            Write-Host $badPlanResult.Output -ForegroundColor Red
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
    if ($null -ne $listener) {
        $listener.Stop()
    }
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   capacity long-run campaign preflight self-test"
