$ErrorActionPreference = "Stop"

$summarizer = Join-Path $PSScriptRoot "summarize-loadtest-capacity-baselines.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

if (-not (Test-Path -LiteralPath $summarizer -PathType Leaf)) {
    throw "Missing loadtest capacity baseline summarizer: $summarizer"
}

function Write-CapacityFixture {
    param(
        [string]$Directory,
        [string]$FileName,
        [string]$StartedAt,
        [object]$Capacity
    )

    New-Item -ItemType Directory -Force -Path $Directory | Out-Null
    $summary = [pscustomobject]@{
        started_at = $StartedAt
        finished_at = "2026-06-16T00:01:00Z"
        capacity_summary = $Capacity
    }
    $summary | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath (Join-Path $Directory $FileName) -Encoding UTF8
}

function Invoke-Summarizer {
    param(
        [string]$ResultRoot,
        [string]$OutputPath,
        [string]$MarkdownPath,
        [string[]]$ExpectedServices,
        [bool]$RequireAllServices
    )

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $args = @(
            "-NoProfile",
            "-ExecutionPolicy",
            "Bypass",
            "-File",
            $summarizer,
            "-ResultRoot",
            $ResultRoot,
            "-OutputPath",
            $OutputPath,
            "-MarkdownPath",
            $MarkdownPath,
            "-ExpectedServices",
            ($ExpectedServices -join ",")
        )

        if ($RequireAllServices) {
            $args += "-RequireAllServices"
        }

        $output = & $powerShellExe @args 2>&1
        return [pscustomobject]@{
            ExitCode = $LASTEXITCODE
            Output = (($output | Out-String).Trim())
        }
    }
    finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-capacity-baseline-summary-" + [System.Guid]::NewGuid().ToString("N"))
try {
    Write-CapacityFixture `
        -Directory (Join-Path $tempRoot "sendmessage-run") `
        -FileName "sendmessage-summary.json" `
        -StartedAt "2026-06-16T00:00:00Z" `
        -Capacity ([pscustomobject]@{
            duration_ms = 1000
            request_count = 10
            success_count = 10
            accepted_rps = 10.0
            p95_ms = 12.5
        })

    Write-CapacityFixture `
        -Directory (Join-Path $tempRoot "push-run") `
        -FileName "pushgateway-summary.json" `
        -StartedAt "2026-06-16T00:00:10Z" `
        -Capacity ([pscustomobject]@{
            duration_ms = 2000
            message_count = 4
            notify_frames_per_second = 2.0
        })

    $ignored = [pscustomobject]@{
        run_name = "no-capacity"
    }
    $ignoredDir = Join-Path $tempRoot "ignored"
    New-Item -ItemType Directory -Force -Path $ignoredDir | Out-Null
    $ignored | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $ignoredDir "ignored-summary.json") -Encoding UTF8

    $jsonPath = Join-Path $tempRoot "capacity-baseline-summary.json"
    $markdownPath = Join-Path $tempRoot "capacity-baseline-summary.md"
    $goodResult = Invoke-Summarizer `
        -ResultRoot $tempRoot `
        -OutputPath $jsonPath `
        -MarkdownPath $markdownPath `
        -ExpectedServices @("message-service", "push-gateway") `
        -RequireAllServices $true
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity baseline fixture should pass." -ForegroundColor Red
        if ($goodResult.Output) {
            Write-Host $goodResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $summary = Get-Content -LiteralPath $jsonPath -Raw | ConvertFrom-Json
    if ($summary.summary_count -ne 2 -or $summary.service_count_found -ne 2) {
        Write-Host "FAIL capacity baseline summary produced wrong counts." -ForegroundColor Red
        exit 1
    }
    if (($summary.services_found -notcontains "message-service") -or ($summary.services_found -notcontains "push-gateway")) {
        Write-Host "FAIL capacity baseline summary missed expected services." -ForegroundColor Red
        exit 1
    }
    $messageRow = @($summary.summaries | Where-Object { $_.service -eq "message-service" })[0]
    if ([double]$messageRow.duration_seconds -ne 1.0 -or $messageRow.primary_rate_field -ne "accepted_rps") {
        Write-Host "FAIL capacity baseline summary normalized duration or primary rate incorrectly." -ForegroundColor Red
        exit 1
    }

    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("Loadtest Capacity Baseline Summary") -or -not $markdown.Contains("message-service")) {
        Write-Host "FAIL capacity baseline markdown missing expected content." -ForegroundColor Red
        exit 1
    }

    $badResult = Invoke-Summarizer `
        -ResultRoot $tempRoot `
        -OutputPath (Join-Path $tempRoot "bad.json") `
        -MarkdownPath (Join-Path $tempRoot "bad.md") `
        -ExpectedServices @("message-service", "delivery-service") `
        -RequireAllServices $true
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL missing service fixture should fail when RequireAllServices is set." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("Missing capacity_summary for service")) {
        Write-Host "FAIL missing service fixture did not report missing service." -ForegroundColor Red
        if ($badResult.Output) {
            Write-Host $badResult.Output -ForegroundColor Red
        }
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   loadtest capacity baseline summary self-test"
