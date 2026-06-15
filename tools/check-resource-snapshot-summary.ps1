$ErrorActionPreference = "Stop"

$summarizer = Join-Path $PSScriptRoot "summarize-local-service-resource-snapshot.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

if (-not (Test-Path -LiteralPath $summarizer -PathType Leaf)) {
    throw "Missing resource snapshot summarizer: $summarizer"
}

function Write-SnapshotFixture {
    param(
        [string]$Directory,
        [bool]$Ready = $true
    )

    New-Item -ItemType Directory -Force -Path $Directory | Out-Null

    $runSummary = [pscustomobject]@{
        run_name = "resource-summary-selftest"
        created_at = "2026-06-15T00:00:00Z"
        result_root = $Directory
        service_count = 2
        service_containers = @(
            "nexusim-api-gateway-grpc",
            "nexusim-message-service-grpc"
        )
        base_containers = @(
            "nexusim-postgres"
        )
        docker_stats_path = (Join-Path $Directory "docker-stats.jsonl")
        endpoint_summary_path = (Join-Path $Directory "endpoint-summary.json")
        scope = "self-test fixture"
    }
    $runSummary | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $Directory "run-summary.json") -Encoding UTF8

    $endpoints = @(
        [pscustomobject]@{
            service = "api-gateway"
            healthz = $true
            readyz = $true
            url = "http://127.0.0.1:11904"
        },
        [pscustomobject]@{
            service = "message-service"
            healthz = $true
            readyz = $Ready
            url = "http://127.0.0.1:11910"
        }
    )
    $endpoints | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $Directory "endpoint-summary.json") -Encoding UTF8

    $stats = @(
        [pscustomobject]@{
            Name = "nexusim-api-gateway-grpc"
            Container = "nexusim-api-gateway-grpc"
            CPUPerc = "0.10%"
            MemPerc = "0.20%"
            MemUsage = "4MiB / 8GiB"
            NetIO = "1kB / 1kB"
            BlockIO = "0B / 0B"
            PIDs = "8"
        },
        [pscustomobject]@{
            Name = "nexusim-message-service-grpc"
            Container = "nexusim-message-service-grpc"
            CPUPerc = "1.50%"
            MemPerc = "0.40%"
            MemUsage = "8MiB / 8GiB"
            NetIO = "2kB / 2kB"
            BlockIO = "0B / 0B"
            PIDs = "9"
        },
        [pscustomobject]@{
            Name = "nexusim-postgres"
            Container = "nexusim-postgres"
            CPUPerc = "2.00%"
            MemPerc = "3.00%"
            MemUsage = "256MiB / 8GiB"
            NetIO = "3kB / 3kB"
            BlockIO = "0B / 4kB"
            PIDs = "12"
        }
    )
    $stats |
        ForEach-Object { $_ | ConvertTo-Json -Compress -Depth 4 } |
        Set-Content -LiteralPath (Join-Path $Directory "docker-stats.jsonl") -Encoding UTF8
}

function Invoke-Summarizer {
    param(
        [string]$SnapshotDir,
        [string]$OutputPath,
        [string]$MarkdownPath
    )

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $output = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $summarizer `
            -SnapshotDir $SnapshotDir `
            -OutputPath $OutputPath `
            -MarkdownPath $MarkdownPath 2>&1
        return [pscustomobject]@{
            ExitCode = $LASTEXITCODE
            Output = (($output | Out-String).Trim())
        }
    }
    finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-resource-summary-" + [System.Guid]::NewGuid().ToString("N"))
try {
    $goodSnapshot = Join-Path $tempRoot "good"
    Write-SnapshotFixture -Directory $goodSnapshot -Ready $true
    $jsonPath = Join-Path $goodSnapshot "summary.json"
    $markdownPath = Join-Path $goodSnapshot "summary.md"
    $goodResult = Invoke-Summarizer -SnapshotDir $goodSnapshot -OutputPath $jsonPath -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL resource summary fixture should pass." -ForegroundColor Red
        if ($goodResult.Output) {
            Write-Host $goodResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $summary = Get-Content -LiteralPath $jsonPath -Raw | ConvertFrom-Json
    if ($summary.service_count -ne 2 -or $summary.endpoints.healthy -ne 2 -or $summary.totals.all_containers -ne 3) {
        Write-Host "FAIL resource summary fixture produced wrong totals." -ForegroundColor Red
        exit 1
    }
    if ([double]$summary.max_cpu_percent -ne 2.0 -or [double]$summary.max_mem_percent -ne 3.0) {
        Write-Host "FAIL resource summary fixture produced wrong max resource values." -ForegroundColor Red
        exit 1
    }

    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("2/2 healthy") -or -not $markdown.Contains("nexusim-message-service-grpc")) {
        Write-Host "FAIL resource summary markdown missing expected content." -ForegroundColor Red
        exit 1
    }

    $badSnapshot = Join-Path $tempRoot "bad"
    Write-SnapshotFixture -Directory $badSnapshot -Ready $false
    $badResult = Invoke-Summarizer -SnapshotDir $badSnapshot -OutputPath (Join-Path $badSnapshot "summary.json") -MarkdownPath (Join-Path $badSnapshot "summary.md")
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL unhealthy endpoint fixture should fail resource summary." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("unhealthy services")) {
        Write-Host "FAIL unhealthy endpoint fixture did not report unhealthy services." -ForegroundColor Red
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

Write-Host "OK   resource snapshot summary self-test"
