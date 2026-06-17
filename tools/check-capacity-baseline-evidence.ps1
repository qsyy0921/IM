$ErrorActionPreference = "Stop"

$validator = Join-Path $PSScriptRoot "validate-capacity-baseline-evidence.ps1"
$adder = Join-Path $PSScriptRoot "add-capacity-baseline-evidence.ps1"

if (-not (Test-Path -LiteralPath $validator -PathType Leaf)) {
    throw "Missing capacity baseline evidence validator: $validator"
}
if (-not (Test-Path -LiteralPath $adder -PathType Leaf)) {
    throw "Missing capacity baseline evidence adder: $adder"
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
    $reportPath = "docs/runbook/loadtest/distributed/loadtest-report-20260616-seeded-capacity-baseline.md"
    $summaryPath = "H:\NexusIM\loadtest-results\capacity-evidence-selftest\sendmessage-summary.json"

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

    $repoSummaryManifestPath = Join-Path $tempRoot "repo-summary-capacity-baseline-evidence.json"
    Write-JsonFile -Path $repoSummaryManifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @($entries | ForEach-Object {
            $copy = [ordered]@{}
            foreach ($property in $_.GetEnumerator()) {
                $copy[$property.Key] = $property.Value
            }
            if ($copy.service -eq "message-service") {
                $copy.summary_path = "docs/runbook/loadtest/distributed/loadtest-report-20260616-seeded-capacity-baseline.md"
            }
            $copy
        })
    })

    $repoSummaryResult = Invoke-Validator -ManifestPath $repoSummaryManifestPath
    if ($repoSummaryResult.ExitCode -eq 0) {
        Write-Host "FAIL capacity baseline evidence with repo-local summary_path should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $repoSummaryResult.Output.Contains("must point under")) {
        Write-Host "FAIL repo-local summary_path fixture returned unexpected error." -ForegroundColor Red
        Write-Host $repoSummaryResult.Output -ForegroundColor Red
        exit 1
    }

    $externalReportManifestPath = Join-Path $tempRoot "external-report-capacity-baseline-evidence.json"
    $externalReportPath = Join-Path $tempRoot "capacity-report.md"
    @(
        "# Fixture capacity report",
        "",
        "This is a local short capacity baseline. It is not a production SLO or sizing claim."
    ) | Set-Content -LiteralPath $externalReportPath -Encoding UTF8
    Write-JsonFile -Path $externalReportManifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @($entries | ForEach-Object {
            $copy = [ordered]@{}
            foreach ($property in $_.GetEnumerator()) {
                $copy[$property.Key] = $property.Value
            }
            if ($copy.service -eq "message-service") {
                $copy.report_path = $externalReportPath
            }
            $copy
        })
    })

    $externalReportResult = Invoke-Validator -ManifestPath $externalReportManifestPath
    if ($externalReportResult.ExitCode -eq 0) {
        Write-Host "FAIL capacity baseline evidence with external report_path should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $externalReportResult.Output.Contains("must stay under docs/runbook/loadtest")) {
        Write-Host "FAIL external report_path fixture returned unexpected error." -ForegroundColor Red
        Write-Host $externalReportResult.Output -ForegroundColor Red
        exit 1
    }

    & $adder `
        -ManifestPath $manifestPath `
        -Service "message-service" `
        -Runner "sendmessage" `
        -BaselineType "seeded" `
        -SummaryPath $summaryPath `
        -ReportPath $reportPath `
        -Note "replacement fixture" `
        -Replace | Out-Null

    $updatedManifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $updatedEntries = @($updatedManifest.entries)
    $updatedMessageEntries = @($updatedEntries | Where-Object { $_.service -eq "message-service" })
    if ($updatedEntries.Count -ne 9 -or $updatedMessageEntries.Count -ne 1 -or $updatedMessageEntries[0].note -ne "replacement fixture" -or $updatedMessageEntries[0].baseline_type -ne "seeded") {
        Write-Host "FAIL capacity baseline evidence adder did not replace exactly one service entry." -ForegroundColor Red
        exit 1
    }

    $duplicateFailed = $false
    try {
        & $adder `
            -ManifestPath $manifestPath `
            -Service "message-service" `
            -Runner "sendmessage" `
            -BaselineType "seeded" `
            -SummaryPath $summaryPath `
            -ReportPath $reportPath `
            -Note "duplicate fixture" | Out-Null
    }
    catch {
        $duplicateFailed = $_.Exception.Message.Contains("already exists")
    }
    if (-not $duplicateFailed) {
        Write-Host "FAIL capacity baseline evidence adder should reject duplicate service without -Replace." -ForegroundColor Red
        exit 1
    }

    $mismatchFailed = $false
    try {
        & $adder `
            -ManifestPath $manifestPath `
            -Service "message-service" `
            -Runner "delivery" `
            -BaselineType "seeded" `
            -SummaryPath $summaryPath `
            -ReportPath $reportPath `
            -Note "mismatch fixture" `
            -Replace | Out-Null
    }
    catch {
        $mismatchFailed = $_.Exception.Message.Contains("does not match")
    }
    if (-not $mismatchFailed) {
        Write-Host "FAIL capacity baseline evidence adder should reject service/runner mismatch." -ForegroundColor Red
        exit 1
    }

    $beforeFailedAdd = Get-Content -LiteralPath $manifestPath -Raw
    $failedAddRolledBack = $false
    try {
        & $adder `
            -ManifestPath $manifestPath `
            -Service "message-service" `
            -Runner "sendmessage" `
            -BaselineType "seeded" `
            -SummaryPath "docs/runbook/loadtest/distributed/loadtest-report-20260616-seeded-capacity-baseline.md" `
            -ReportPath $reportPath `
            -Note "bad path fixture" `
            -Replace | Out-Null
    }
    catch {
        $failedAddRolledBack = $_.Exception.Message.Contains("must point under")
    }
    if (-not $failedAddRolledBack) {
        Write-Host "FAIL add-capacity-baseline-evidence.ps1 should fail validation for repo-local summary_path." -ForegroundColor Red
        exit 1
    }
    $afterFailedAdd = Get-Content -LiteralPath $manifestPath -Raw
    if ($afterFailedAdd -ne $beforeFailedAdd) {
        Write-Host "FAIL failed add-capacity-baseline-evidence.ps1 call should restore original manifest." -ForegroundColor Red
        exit 1
    }

    $sensitiveFailed = $false
    try {
        & $adder `
            -ManifestPath $manifestPath `
            -Service "message-service" `
            -Runner "sendmessage" `
            -BaselineType "seeded" `
            -SummaryPath "${summaryPath}?token=secret" `
            -ReportPath $reportPath `
            -Note "sensitive fixture" `
            -Replace | Out-Null
    }
    catch {
        $sensitiveFailed = $_.Exception.Message.Contains("low-sensitive evidence metadata")
    }
    if (-not $sensitiveFailed) {
        Write-Host "FAIL capacity baseline evidence adder should reject sensitive metadata." -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   capacity baseline evidence self-test"
