param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$OutputPath = "",
    [string]$MarkdownPath = "",
    [string[]]$ExpectedServices = @(
        "api-gateway",
        "identity-service",
        "message-service",
        "conversation-service",
        "delivery-service",
        "push-gateway",
        "receipt-service",
        "contacts-service",
        "policy-service"
    ),
    [switch]$RequireAllServices,
    [string]$Scope = "aggregate existing loadtest capacity_summary files; not a production SLO or capacity guarantee"
)

$ErrorActionPreference = "Stop"

function Get-PropertyValue {
    param(
        [object]$Object,
        [string]$Name
    )

    if ($null -eq $Object) {
        return $null
    }

    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) {
        return $null
    }

    return $property.Value
}

function Test-IsNumber {
    param([object]$Value)

    if ($null -eq $Value) {
        return $false
    }

    return $Value -is [byte] -or
        $Value -is [int16] -or
        $Value -is [int32] -or
        $Value -is [int64] -or
        $Value -is [single] -or
        $Value -is [double] -or
        $Value -is [decimal]
}

function Convert-ToDoubleOrNull {
    param([object]$Value)

    if (Test-IsNumber -Value $Value) {
        return [double]$Value
    }

    return $null
}

function Get-RunnerName {
    param([string]$SummaryPath)

    $leaf = [System.IO.Path]::GetFileName($SummaryPath).ToLowerInvariant()
    $parent = Split-Path -Leaf (Split-Path -Parent $SummaryPath)

    if ($leaf -like "*sendmessage*") { return "sendmessage" }
    if ($leaf -like "*pushgateway*") { return "pushgateway" }
    if ($leaf -like "*memberchange*") { return "memberchange" }
    if ($leaf -like "*delivery*") { return "delivery" }
    if ($leaf -like "*receipt*") { return "receipt" }
    if ($leaf -like "*contacts*") { return "contacts" }
    if ($leaf -like "*policy*") { return "policy" }
    if ($leaf -like "*identity*") { return "identity" }
    if ($leaf -like "*demo*") { return "demo" }

    return $parent
}

function Get-ServiceName {
    param(
        [object]$Summary,
        [string]$Runner
    )

    $service = [string](Get-PropertyValue -Object $Summary -Name "service")
    if ($service.Trim().Length -gt 0) {
        return $service.Trim()
    }

    switch ($Runner) {
        "demo" { return "api-gateway" }
        "identity" { return "identity-service" }
        "sendmessage" { return "message-service" }
        "memberchange" { return "conversation-service" }
        "delivery" { return "delivery-service" }
        "pushgateway" { return "push-gateway" }
        "receipt" { return "receipt-service" }
        "contacts" { return "contacts-service" }
        "policy" { return "policy-service" }
        default { return "unknown" }
    }
}

function Get-DurationSeconds {
    param([object]$Capacity)

    $seconds = Convert-ToDoubleOrNull (Get-PropertyValue -Object $Capacity -Name "duration_seconds")
    if ($null -ne $seconds) {
        return $seconds
    }

    $milliseconds = Convert-ToDoubleOrNull (Get-PropertyValue -Object $Capacity -Name "duration_ms")
    if ($null -ne $milliseconds) {
        return [Math]::Round($milliseconds / 1000.0, 3)
    }

    return $null
}

function Get-PrimaryRate {
    param([object]$Capacity)

    $preferred = @(
        "accepted_rps",
        "logical_accepted_rps",
        "request_rps",
        "operations_per_second",
        "messages_per_second",
        "notify_frames_per_second",
        "decisions_per_second",
        "ops_per_second",
        "events_per_second",
        "acks_per_second",
        "items_per_second"
    )

    foreach ($name in $preferred) {
        $value = Convert-ToDoubleOrNull (Get-PropertyValue -Object $Capacity -Name $name)
        if ($null -ne $value) {
            return [pscustomobject]@{
                field = $name
                value = $value
            }
        }
    }

    foreach ($property in $Capacity.PSObject.Properties) {
        if ($property.Name -match "(per_second|_rps|rate)$") {
            $value = Convert-ToDoubleOrNull $property.Value
            if ($null -ne $value) {
                return [pscustomobject]@{
                    field = $property.Name
                    value = $value
                }
            }
        }
    }

    return [pscustomobject]@{
        field = ""
        value = $null
    }
}

function Format-NullableNumber {
    param([object]$Value)

    if ($null -eq $Value) {
        return ""
    }

    return ([double]$Value).ToString("0.###", [System.Globalization.CultureInfo]::InvariantCulture)
}

if (-not (Test-Path -LiteralPath $ResultRoot -PathType Container)) {
    throw "ResultRoot does not exist: $ResultRoot"
}

if ($OutputPath.Trim().Length -eq 0 -or $MarkdownPath.Trim().Length -eq 0) {
    $outputDir = Join-Path $ResultRoot ("capacity-baseline-summary-" + (Get-Date).ToString("yyyyMMdd-HHmmss"))
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    if ($OutputPath.Trim().Length -eq 0) {
        $OutputPath = Join-Path $outputDir "capacity-baseline-summary.json"
    }
    if ($MarkdownPath.Trim().Length -eq 0) {
        $MarkdownPath = Join-Path $outputDir "capacity-baseline-summary.md"
    }
}

$summaryFiles = Get-ChildItem -LiteralPath $ResultRoot -Recurse -File -Filter "*summary.json" |
    Sort-Object FullName

$rows = New-Object System.Collections.Generic.List[object]
foreach ($file in $summaryFiles) {
    $json = Get-Content -LiteralPath $file.FullName -Raw | ConvertFrom-Json
    $capacity = Get-PropertyValue -Object $json -Name "capacity_summary"
    if ($null -eq $capacity) {
        continue
    }

    $runner = Get-RunnerName -SummaryPath $file.FullName
    $service = Get-ServiceName -Summary $json -Runner $runner
    $durationSeconds = Get-DurationSeconds -Capacity $capacity
    $primaryRate = Get-PrimaryRate -Capacity $capacity

    $rows.Add([pscustomobject]@{
        service = $service
        runner = $runner
        result_dir = (Split-Path -Parent $file.FullName)
        summary_path = $file.FullName
        summary_file = $file.Name
        started_at = [string](Get-PropertyValue -Object $json -Name "started_at")
        finished_at = [string](Get-PropertyValue -Object $json -Name "finished_at")
        duration_seconds = $durationSeconds
        primary_rate_field = $primaryRate.field
        primary_rate_value = $primaryRate.value
        capacity_summary = $capacity
    })
}

$servicesFound = @($rows | ForEach-Object { $_.service } | Where-Object { $_ -ne "unknown" } | Sort-Object -Unique)
$expected = @(
    $ExpectedServices |
        ForEach-Object { ([string]$_) -split "[,;]" } |
        ForEach-Object { ([string]$_).Trim() } |
        Where-Object { $_.Length -gt 0 } |
        Sort-Object -Unique
)
$foundSet = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
foreach ($service in $servicesFound) {
    [void]$foundSet.Add($service)
}
$missingServices = @($expected | Where-Object { -not $foundSet.Contains($_) })

if ($RequireAllServices -and $missingServices.Count -gt 0) {
    throw "Missing capacity_summary for service(s): $($missingServices -join ', ')"
}

$rowArray = @($rows.ToArray())
$servicesFoundArray = @($servicesFound)
$missingServicesArray = @($missingServices)

$output = [pscustomobject]@{
    created_at = (Get-Date).ToUniversalTime().ToString("o")
    result_root = ([System.IO.Path]::GetFullPath($ResultRoot))
    scope = $Scope
    summary_count = $rowArray.Count
    expected_service_count = $expected.Count
    service_count_found = $servicesFoundArray.Count
    services_found = $servicesFoundArray
    missing_services = $missingServicesArray
    summaries = $rowArray
}

$outputDirectory = Split-Path -Parent $OutputPath
if ($outputDirectory.Trim().Length -gt 0) {
    New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
}
$markdownDirectory = Split-Path -Parent $MarkdownPath
if ($markdownDirectory.Trim().Length -gt 0) {
    New-Item -ItemType Directory -Force -Path $markdownDirectory | Out-Null
}

$output | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $OutputPath -Encoding UTF8

$markdown = @()
$markdown += "# Loadtest Capacity Baseline Summary"
$markdown += ""
$markdown += "- Created at: $($output.created_at)"
$markdown += "- Result root: $($output.result_root)"
$markdown += "- Scope: $Scope"
$markdown += "- Summary files with capacity_summary: $($rows.Count)"
$markdown += "- Services found: $($servicesFound.Count)/$($expected.Count)"
if ($missingServices.Count -gt 0) {
    $markdown += "- Missing services: $($missingServices -join ', ')"
}
else {
    $markdown += "- Missing services: none"
}
$markdown += ""
$markdown += "| Service | Runner | Duration(s) | Primary rate | Summary |"
$markdown += "| --- | --- | ---: | ---: | --- |"
foreach ($row in @($rows | Sort-Object service, summary_path)) {
    $durationText = Format-NullableNumber -Value $row.duration_seconds
    $rateText = ""
    if ($row.primary_rate_field.Trim().Length -gt 0) {
        $rateText = "$($row.primary_rate_field)=$(Format-NullableNumber -Value $row.primary_rate_value)"
    }
    $markdown += "| $($row.service) | $($row.runner) | $durationText | $rateText | `$($row.summary_path)` |"
}
$markdown += ""
$markdown += "This summary aggregates existing loadtest output files. It is evidence for local capacity-baseline tracking only; it is not a production SLO, production sizing claim, or HA proof."

$markdown | Set-Content -LiteralPath $MarkdownPath -Encoding UTF8

Write-Host "OK   loadtest capacity baseline summary written: $OutputPath"
Write-Host "OK   loadtest capacity baseline report written: $MarkdownPath"
