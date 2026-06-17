$ErrorActionPreference = "Stop"

$validator = Join-Path $PSScriptRoot "validate-capacity-baseline-evidence.ps1"

if (-not (Test-Path -LiteralPath $validator -PathType Leaf)) {
    throw "Missing capacity baseline evidence validator: $validator"
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

$repoManifest = Join-Path (Split-Path -Parent $PSScriptRoot) "docs\runbook\capacity-baseline-evidence.json"
$schemaOnlyResult = Invoke-Validator -ManifestPath $repoManifest
if ($schemaOnlyResult.ExitCode -ne 0) {
    Write-Host "FAIL repo capacity baseline evidence manifest schema should pass." -ForegroundColor Red
    Write-Host $schemaOnlyResult.Output -ForegroundColor Red
    exit 1
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-capacity-evidence-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $reportPath = Join-Path $tempRoot "capacity-report.md"
    @(
        "# Fixture capacity report",
        "",
        "This is a local short capacity baseline. It is not a production SLO or sizing claim."
    ) | Set-Content -LiteralPath $reportPath -Encoding UTF8

    $summaryPath = Join-Path $tempRoot "sendmessage-summary.json"
    Write-JsonFile -Path $summaryPath -Value ([ordered]@{
        service = "message-service"
        started_at = "2026-06-17T00:00:00Z"
        finished_at = "2026-06-17T00:00:05Z"
        capacity_summary = [ordered]@{
            duration_seconds = 5
            request_count = 10
            accepted_rps = 2.0
        }
    })

    $manifestPath = Join-Path $tempRoot "capacity-baseline-evidence.json"
    $entries = @()
    foreach ($pair in @(
            @("api-gateway", "demo", "stack"),
            @("identity-service", "identity", "stack"),
            @("message-service", "sendmessage", "direct"),
            @("conversation-service", "memberchange", "seeded"),
            @("delivery-service", "delivery", "seeded"),
            @("push-gateway", "pushgateway", "stack"),
            @("receipt-service", "receipt", "stack"),
            @("contacts-service", "contacts", "stack"),
            @("policy-service", "policy", "direct")
        )) {
        $entries += [ordered]@{
            service = $pair[0]
            runner = $pair[1]
            baseline_type = $pair[2]
            summary_path = $summaryPath
            report_path = $reportPath
            note = "fixture"
        }
    }
    Write-JsonFile -Path $manifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = $entries
    })

    $markdownPath = Join-Path $tempRoot "capacity-baseline-evidence.md"
    $goodResult = Invoke-Validator -ManifestPath $manifestPath -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity baseline evidence fixture should pass." -ForegroundColor Red
        Write-Host $goodResult.Output -ForegroundColor Red
        exit 1
    }
    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("NexusIM Capacity Baseline Evidence") -or -not $markdown.Contains("message-service") -or -not $markdown.Contains("not a production SLO")) {
        Write-Host "FAIL capacity baseline evidence markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }

    $badManifestPath = Join-Path $tempRoot "bad-capacity-baseline-evidence.json"
    Write-JsonFile -Path $badManifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                service = "message-service"
                runner = "sendmessage"
                baseline_type = "direct"
                summary_path = $summaryPath
                report_path = $reportPath
                note = "fixture"
            }
        )
    })
    $badResult = Invoke-Validator -ManifestPath $badManifestPath
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL manifest missing services should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("missing service")) {
        Write-Host "FAIL bad capacity baseline evidence fixture returned unexpected error." -ForegroundColor Red
        Write-Host $badResult.Output -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   capacity baseline evidence self-test"
