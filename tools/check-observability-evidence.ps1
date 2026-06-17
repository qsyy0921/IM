$ErrorActionPreference = "Stop"

$validator = Join-Path $PSScriptRoot "validate-observability-evidence.ps1"

if (-not (Test-Path -LiteralPath $validator -PathType Leaf)) {
    throw "Missing observability evidence validator: $validator"
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

$repoManifest = Join-Path (Split-Path -Parent $PSScriptRoot) "docs\runbook\observability-evidence.json"
$schemaOnlyResult = Invoke-Validator -ManifestPath $repoManifest
if ($schemaOnlyResult.ExitCode -ne 0) {
    Write-Host "FAIL repo observability evidence manifest schema should pass." -ForegroundColor Red
    Write-Host $schemaOnlyResult.Output -ForegroundColor Red
    exit 1
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-observability-evidence-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $reportPath = Join-Path $tempRoot "observability-report.md"
    @(
        "# Fixture observability report",
        "",
        "This verifies local debug metrics. It is not a production SLO or observability platform claim."
    ) | Set-Content -LiteralPath $reportPath -Encoding UTF8

    $summaryPath = Join-Path $tempRoot "policy-smoke-summary.json"
    Write-JsonFile -Path $summaryPath -Value ([ordered]@{
        success = $true
        allow = [ordered]@{ git_dirty = $false }
        deny = [ordered]@{ git_dirty = $false }
        allow_debug_metrics = [ordered]@{
            service = "policy-service"
            grpc = [ordered]@{ total_requests = 4; total_errors = 0 }
            decisions = [ordered]@{ total = 4; allowed = 4; denied = 0; errors = 0 }
        }
        deny_debug_metrics = [ordered]@{
            service = "policy-service"
            grpc = [ordered]@{ total_requests = 4; total_errors = 0 }
            decisions = [ordered]@{ total = 4; allowed = 0; denied = 4; errors = 0 }
        }
    })

    $manifestPath = Join-Path $tempRoot "observability-evidence.json"
    Write-JsonFile -Path $manifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "policy fixture"
                kind = "service-debug-smoke"
                service = "policy-service"
                summary_path = $summaryPath
                report_path = $reportPath
                require_clean_git = $true
                note = "fixture"
            }
        )
    })

    $markdownPath = Join-Path $tempRoot "observability-evidence.md"
    $goodResult = Invoke-Validator -ManifestPath $manifestPath -RequireFiles -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL observability evidence fixture should pass." -ForegroundColor Red
        Write-Host $goodResult.Output -ForegroundColor Red
        exit 1
    }
    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("NexusIM Observability Evidence") -or -not $markdown.Contains("policy fixture") -or -not $markdown.Contains("not a production SLO")) {
        Write-Host "FAIL observability evidence markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }

    $badManifestPath = Join-Path $tempRoot "bad-observability-evidence.json"
    Write-JsonFile -Path $badManifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "bad duplicate"
                kind = "service-debug-smoke"
                service = "policy-service"
                summary_path = $summaryPath
                report_path = $reportPath
                note = "fixture"
            },
            [ordered]@{
                name = "bad duplicate"
                kind = "service-debug-smoke"
                service = "policy-service"
                summary_path = $summaryPath
                report_path = $reportPath
                note = "fixture"
            }
        )
    })
    $badResult = Invoke-Validator -ManifestPath $badManifestPath
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL duplicate observability evidence entries should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("duplicate")) {
        Write-Host "FAIL bad observability evidence fixture returned unexpected error." -ForegroundColor Red
        Write-Host $badResult.Output -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   observability evidence self-test"
