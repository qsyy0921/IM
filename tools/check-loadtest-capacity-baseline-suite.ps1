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

    $pushStep = @($summary.steps | Where-Object { $_.service -eq "push-gateway" })[0]
    if ($pushStep.command_line -notmatch "--scenario full") {
        Write-Host "FAIL push-gateway capacity suite step must run full scenario." -ForegroundColor Red
        exit 1
    }

    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("Loadtest Capacity Baseline Suite") -or -not $markdown.Contains("not a production SLO")) {
        Write-Host "FAIL capacity suite markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   loadtest capacity baseline suite self-test"
