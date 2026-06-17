$ErrorActionPreference = "Stop"

$validator = Join-Path $PSScriptRoot "validate-resource-snapshot-evidence.ps1"
$adder = Join-Path $PSScriptRoot "add-resource-snapshot-evidence.ps1"

if (-not (Test-Path -LiteralPath $validator -PathType Leaf)) {
    throw "Missing resource snapshot evidence validator: $validator"
}
if (-not (Test-Path -LiteralPath $adder -PathType Leaf)) {
    throw "Missing resource snapshot evidence adder: $adder"
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
    $Value | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Invoke-Validator {
    param(
        [string]$ManifestPath,
        [switch]$RequireFiles,
        [string]$MarkdownPath = ""
    )

    try {
        $invocationArgs = @{
            ManifestPath = $ManifestPath
        }
        if ($RequireFiles) {
            $invocationArgs.RequireFiles = $true
        }
        if ($MarkdownPath.Trim().Length -gt 0) {
            $invocationArgs.MarkdownPath = $MarkdownPath
        }
        $output = & $validator @invocationArgs 2>&1
        return [pscustomobject]@{
            ExitCode = 0
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

function Write-ResourceSummaryFixture {
    param(
        [string]$SummaryPath,
        [string]$MarkdownPath,
        [bool]$GoodBoundary = $true
    )

    $scope = if ($GoodBoundary) {
        "single no-stream Docker stats snapshot after healthz/readyz pass; not a capacity benchmark"
    }
    else {
        "single no-stream Docker stats snapshot"
    }

    Write-JsonFile -Path $SummaryPath -Value ([ordered]@{
        run_name = "resource-evidence-selftest"
        created_at = "2026-06-17T00:00:00Z"
        summarized_at = "2026-06-17T00:00:01Z"
        scope = $scope
        snapshot_dir = (Split-Path -Parent $SummaryPath)
        service_count = 9
        totals = [ordered]@{
            service_containers = 9
            base_containers = 3
            all_containers = 12
        }
        endpoints = [ordered]@{
            total = 9
            healthy = 9
            unready = 0
            unhealthy = 0
        }
        max_cpu_percent = 2.0
        max_mem_percent = 3.0
        rows = @(
            [ordered]@{
                name = "nexusim-api-gateway-grpc"
                role = "service"
                cpu_percent = 1.0
                mem_percent = 1.0
            }
        )
    })

    $boundary = if ($GoodBoundary) {
        "This summary is a health-state snapshot only. It is not a capacity benchmark or production SLO measurement."
    }
    else {
        "This summary is a health-state snapshot only."
    }
    @(
        "# Local Service Resource Snapshot",
        "",
        "- Endpoints: 9/9 healthy",
        "",
        $boundary
    ) | Set-Content -LiteralPath $MarkdownPath -Encoding UTF8
}

$repoManifest = Join-Path (Split-Path -Parent $PSScriptRoot) "docs\runbook\resource-snapshot-evidence.json"
$schemaOnlyResult = Invoke-Validator -ManifestPath $repoManifest
if ($schemaOnlyResult.ExitCode -ne 0) {
    Write-Host "FAIL repo resource snapshot evidence manifest schema should pass." -ForegroundColor Red
    Write-Host $schemaOnlyResult.Output -ForegroundColor Red
    exit 1
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-resource-evidence-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $summaryPath = Join-Path $tempRoot "resource-summary.json"
    $markdownSummaryPath = Join-Path $tempRoot "resource-summary.md"
    Write-ResourceSummaryFixture -SummaryPath $summaryPath -MarkdownPath $markdownSummaryPath

    $manifestPath = Join-Path $tempRoot "resource-snapshot-evidence.json"
    Write-JsonFile -Path $manifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "resource-evidence-selftest"
                summary_path = $summaryPath
                markdown_path = $markdownSummaryPath
                require_clean_git = $false
                note = "fixture"
            }
        )
    })

    $markdownPath = Join-Path $tempRoot "resource-snapshot-evidence.md"
    $goodResult = Invoke-Validator -ManifestPath $manifestPath -RequireFiles -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL resource snapshot evidence fixture should pass." -ForegroundColor Red
        Write-Host $goodResult.Output -ForegroundColor Red
        exit 1
    }
    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("NexusIM Resource Snapshot Evidence") -or -not $markdown.Contains("not a capacity benchmark")) {
        Write-Host "FAIL resource snapshot evidence markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }

    $badSummaryPath = Join-Path $tempRoot "bad-resource-summary.json"
    $badMarkdownSummaryPath = Join-Path $tempRoot "bad-resource-summary.md"
    Write-ResourceSummaryFixture -SummaryPath $badSummaryPath -MarkdownPath $badMarkdownSummaryPath -GoodBoundary $false
    $badManifestPath = Join-Path $tempRoot "bad-resource-snapshot-evidence.json"
    Write-JsonFile -Path $badManifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "bad-resource-evidence-selftest"
                summary_path = $badSummaryPath
                markdown_path = $badMarkdownSummaryPath
                require_clean_git = $false
                note = "fixture"
            }
        )
    })
    $badResult = Invoke-Validator -ManifestPath $badManifestPath -RequireFiles
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL resource snapshot evidence fixture missing boundary should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("not a capacity benchmark")) {
        Write-Host "FAIL bad resource snapshot evidence fixture returned unexpected error." -ForegroundColor Red
        Write-Host $badResult.Output -ForegroundColor Red
        exit 1
    }

    & $adder `
        -ManifestPath $manifestPath `
        -Name "resource-evidence-selftest-2" `
        -SummaryPath $summaryPath `
        -MarkdownPath $markdownSummaryPath `
        -Note "fixture 2" | Out-Null
    $updatedManifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    if (@($updatedManifest.entries).Count -ne 2) {
        Write-Host "FAIL resource snapshot evidence adder did not append one entry." -ForegroundColor Red
        exit 1
    }

    $duplicateFailed = $false
    try {
        & $adder `
            -ManifestPath $manifestPath `
            -Name "resource-evidence-selftest-2" `
            -SummaryPath $summaryPath `
            -MarkdownPath $markdownSummaryPath `
            -Note "duplicate fixture" | Out-Null
    }
    catch {
        $duplicateFailed = $_.Exception.Message.Contains("already exists")
    }
    if (-not $duplicateFailed) {
        Write-Host "FAIL resource snapshot evidence adder should reject duplicate name without -Replace." -ForegroundColor Red
        exit 1
    }

    & $adder `
        -ManifestPath $manifestPath `
        -Name "resource-evidence-selftest-2" `
        -SummaryPath $summaryPath `
        -MarkdownPath $markdownSummaryPath `
        -Note "replacement fixture" `
        -Replace | Out-Null
    $replacedManifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $replacedEntry = @($replacedManifest.entries | Where-Object { $_.name -eq "resource-evidence-selftest-2" })
    if ($replacedEntry.Count -ne 1 -or $replacedEntry[0].note -ne "replacement fixture") {
        Write-Host "FAIL resource snapshot evidence adder did not replace expected entry." -ForegroundColor Red
        exit 1
    }

    $sensitiveFailed = $false
    try {
        & $adder `
            -ManifestPath $manifestPath `
            -Name "resource-evidence-sensitive" `
            -SummaryPath $summaryPath `
            -MarkdownPath $markdownSummaryPath `
            -Note "eyJaaaaaaaaaaa.payload.signature" | Out-Null
    }
    catch {
        $sensitiveFailed = $_.Exception.Message.Contains("low-sensitive evidence metadata")
    }
    if (-not $sensitiveFailed) {
        Write-Host "FAIL resource snapshot evidence adder should reject sensitive metadata." -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   resource snapshot evidence self-test"
