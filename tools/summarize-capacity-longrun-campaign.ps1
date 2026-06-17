param(
    [Parameter(Mandatory = $true)]
    [string]$PlanPath,
    [string]$SummaryPath = "",
    [string]$ReportPath = "",
    [string]$ReportRoot = "docs/runbook/loadtest",
    [string[]]$Services = @(),
    [int]$MinimumDurationSeconds = 1800,
    [switch]$AllowIncomplete
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Resolve-RepoPath {
    param([string]$PathValue)

    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return [System.IO.Path]::GetFullPath($PathValue)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $repoRoot $PathValue))
}

function Test-PathInsideDirectory {
    param(
        [string]$Path,
        [string]$Directory
    )

    $fullPath = [System.IO.Path]::GetFullPath($Path).TrimEnd(
        [System.IO.Path]::DirectorySeparatorChar,
        [System.IO.Path]::AltDirectorySeparatorChar
    )
    $fullDirectory = [System.IO.Path]::GetFullPath($Directory).TrimEnd(
        [System.IO.Path]::DirectorySeparatorChar,
        [System.IO.Path]::AltDirectorySeparatorChar
    )

    if ($fullPath.Equals($fullDirectory, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $true
    }

    $prefix = $fullDirectory + [System.IO.Path]::DirectorySeparatorChar
    return $fullPath.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)
}

function Get-JsonProperty {
    param(
        $Object,
        [string]$Name
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        return $null
    }
    return $Object.$Name
}

function Get-JsonPropertyString {
    param(
        $Object,
        [string]$Name
    )

    $value = Get-JsonProperty -Object $Object -Name $Name
    if ($null -eq $value) {
        return ""
    }
    return ([string]$value).Trim()
}

function Convert-ToStringArray {
    param([object]$Value)

    $items = New-Object System.Collections.Generic.List[string]
    foreach ($item in @($Value)) {
        $text = ([string]$item).Trim()
        if ($text.Length -gt 0) {
            $items.Add($text)
        }
    }
    return @($items.ToArray())
}

function Convert-ToRequestedServices {
    param([string[]]$Values)

    $items = New-Object System.Collections.Generic.List[string]
    $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($value in @($Values)) {
        foreach ($part in (([string]$value) -split "[,;]")) {
            $text = $part.Trim()
            if ($text.Length -gt 0 -and $seen.Add($text)) {
                $items.Add($text)
            }
        }
    }
    return @($items.ToArray())
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

function Get-DurationSeconds {
    param([object]$Capacity)

    $seconds = Convert-ToDoubleOrNull (Get-JsonProperty -Object $Capacity -Name "duration_seconds")
    if ($null -ne $seconds) {
        return $seconds
    }

    $milliseconds = Convert-ToDoubleOrNull (Get-JsonProperty -Object $Capacity -Name "duration_ms")
    if ($null -ne $milliseconds) {
        return [Math]::Round($milliseconds / 1000.0, 3)
    }

    return $null
}

function Get-PositiveMetric {
    param([object]$Capacity)

    foreach ($field in @("success_count", "logical_success_count", "allowed_action_count")) {
        $value = Convert-ToDoubleOrNull (Get-JsonProperty -Object $Capacity -Name $field)
        if ($null -ne $value) {
            return [pscustomobject]@{
                field = $field
                value = $value
                positive = ($value -gt 0)
            }
        }
    }

    foreach ($field in @(
            "accepted_rps",
            "logical_accepted_rps",
            "request_rps",
            "requests_per_second",
            "operations_per_second",
            "messages_per_second",
            "notify_frames_per_second",
            "decisions_per_second",
            "ops_per_second",
            "events_per_second",
            "acks_per_second",
            "items_per_second"
        )) {
        $value = Convert-ToDoubleOrNull (Get-JsonProperty -Object $Capacity -Name $field)
        if ($null -ne $value) {
            return [pscustomobject]@{
                field = $field
                value = $value
                positive = ($value -gt 0)
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
                    positive = ($value -gt 0)
                }
            }
        }
    }

    return [pscustomobject]@{
        field = ""
        value = $null
        positive = $false
    }
}

function Find-CapacitySummaryFile {
    param([string]$ResultDir)

    if (-not (Test-Path -LiteralPath $ResultDir -PathType Container)) {
        return $null
    }

    $files = @(Get-ChildItem -LiteralPath $ResultDir -Recurse -File -Filter "*summary.json" -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTimeUtc -Descending)
    foreach ($file in $files) {
        try {
            $json = Get-Content -LiteralPath $file.FullName -Raw | ConvertFrom-Json
            $capacity = Get-JsonProperty -Object $json -Name "capacity_summary"
            if ($null -ne $capacity) {
                return [pscustomobject]@{
                    path = $file.FullName
                    json = $json
                    capacity = $capacity
                }
            }
        }
        catch {
            continue
        }
    }

    return $null
}

function Format-NullableNumber {
    param([object]$Value)

    if ($null -eq $Value) {
        return ""
    }
    if ($Value -is [double] -or $Value -is [single] -or $Value -is [decimal]) {
        return ([double]$Value).ToString("0.###", [System.Globalization.CultureInfo]::InvariantCulture)
    }
    return [string]$Value
}

function Escape-MarkdownCell {
    param([string]$Value)

    return $Value.Replace("|", "\|").Replace("`r", " ").Replace("`n", " ").Trim()
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$resolvedPlanPath = Resolve-RepoPath $PlanPath
Assert-Condition (Test-Path -LiteralPath $resolvedPlanPath -PathType Leaf) "PlanPath does not exist: $resolvedPlanPath"
Assert-Condition ($MinimumDurationSeconds -ge 1800) "MinimumDurationSeconds must be at least 1800 for a long-run campaign."

$plan = Get-Content -LiteralPath $resolvedPlanPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$plan.schema_version -eq 1) "capacity long-run campaign plan schema_version must be 1."
Assert-Condition ((Get-JsonPropertyString -Object $plan -Name "scope") -match "not a production SLO") "capacity long-run campaign plan must state non-SLO boundary."
$planServices = Convert-ToStringArray -Value $plan.services
$requestedServices = Convert-ToRequestedServices -Values $Services
$planServiceSet = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
foreach ($service in $planServices) {
    [void]$planServiceSet.Add($service)
}
foreach ($service in $requestedServices) {
    Assert-Condition ($planServiceSet.Contains($service)) "Requested service is not in the capacity long-run campaign plan: $service"
}
$selectedServiceSet = $null
if ($requestedServices.Count -gt 0) {
    $selectedServiceSet = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($service in $requestedServices) {
        [void]$selectedServiceSet.Add($service)
    }
}

$campaignName = Get-JsonPropertyString -Object $plan -Name "campaign_name"
$outputRoot = Get-JsonPropertyString -Object $plan -Name "output_root"
$runDirectory = Get-JsonPropertyString -Object $plan -Name "run_directory"
Assert-Condition ($campaignName.Length -gt 0) "capacity long-run campaign plan campaign_name is required."
Assert-Condition ($outputRoot.Length -gt 0) "capacity long-run campaign plan output_root is required."
Assert-Condition ($runDirectory.Length -gt 0) "capacity long-run campaign plan run_directory is required."

$outputRootFullPath = [System.IO.Path]::GetFullPath($outputRoot)
$runDirectoryFullPath = [System.IO.Path]::GetFullPath($runDirectory)
Assert-ExternalOutputRoot -Value $outputRootFullPath -RepositoryRoot $repoRoot -Name "Plan output_root"
Assert-Condition (Test-PathInsideDirectory -Path $runDirectoryFullPath -Directory $outputRootFullPath) "Plan run_directory must stay under output_root."
Assert-Condition (Test-PathInsideDirectory -Path $resolvedPlanPath -Directory $outputRootFullPath) "PlanPath must stay under plan output_root."

if ($SummaryPath.Trim().Length -eq 0) {
    $resolvedSummaryPath = Join-Path $runDirectoryFullPath "capacity-longrun-campaign-summary.json"
}
else {
    $resolvedSummaryPath = Resolve-RepoPath $SummaryPath
}
Assert-Condition (Test-PathInsideDirectory -Path $resolvedSummaryPath -Directory $outputRootFullPath) "SummaryPath must stay under plan output_root."

$reportRootFullPath = Resolve-RepoPath $ReportRoot
if ($ReportPath.Trim().Length -eq 0) {
    $resolvedReportPath = Join-Path $reportRootFullPath "$campaignName-report.md"
}
else {
    $resolvedReportPath = Resolve-RepoPath $ReportPath
}
Assert-Condition (Test-PathInsideDirectory -Path $resolvedReportPath -Directory $reportRootFullPath) "ReportPath must stay under ReportRoot."

$rows = New-Object System.Collections.Generic.List[object]
foreach ($step in @($plan.steps)) {
    $service = Get-JsonPropertyString -Object $step -Name "service"
    if ($null -ne $selectedServiceSet -and -not $selectedServiceSet.Contains($service)) {
        continue
    }
    $runner = Get-JsonPropertyString -Object $step -Name "runner"
    $runnerMode = Get-JsonPropertyString -Object $step -Name "runner_mode"
    $resultDir = Get-JsonPropertyString -Object $step -Name "result_dir"

    $status = "passed"
    $reason = ""
    $summaryPathForStep = ""
    $durationSeconds = $null
    $metricField = ""
    $metricValue = $null

    if ($service.Length -eq 0 -or $runner.Length -eq 0 -or $resultDir.Length -eq 0) {
        $status = "invalid_plan_step"
        $reason = "missing service, runner, or result_dir"
    }
    elseif (-not (Test-PathInsideDirectory -Path $resultDir -Directory $runDirectoryFullPath)) {
        $status = "invalid_result_dir"
        $reason = "result_dir is outside run_directory"
    }
    else {
        $summary = Find-CapacitySummaryFile -ResultDir $resultDir
        if ($null -eq $summary) {
            $status = "missing_summary"
            $reason = "no summary JSON with capacity_summary under result_dir"
        }
        else {
            $summaryPathForStep = $summary.path
            $success = Get-JsonProperty -Object $summary.json -Name "success"
            if ($success -is [bool] -and -not $success) {
                $status = "summary_failed"
                $reason = "summary success=false"
            }
            else {
                $durationSeconds = Get-DurationSeconds -Capacity $summary.capacity
                if ($null -eq $durationSeconds) {
                    $status = "missing_duration"
                    $reason = "capacity_summary lacks duration"
                }
                elseif ([double]$durationSeconds -lt [double]$MinimumDurationSeconds) {
                    $status = "short_duration"
                    $reason = "duration_seconds $durationSeconds is below required $MinimumDurationSeconds"
                }
                else {
                    $metric = Get-PositiveMetric -Capacity $summary.capacity
                    $metricField = $metric.field
                    $metricValue = $metric.value
                    if (-not [bool]$metric.positive) {
                        $status = "non_positive_capacity"
                        $reason = "capacity_summary lacks positive success or throughput metric"
                    }
                    else {
                        $reason = "$metricField=$metricValue"
                    }
                }
            }
        }
    }

    $rows.Add([pscustomobject]@{
        service = $service
        runner = $runner
        runner_mode = $runnerMode
        result_dir = $resultDir
        summary_path = $summaryPathForStep
        status = $status
        reason = $reason
        duration_seconds = $durationSeconds
        primary_rate_field = $metricField
        primary_rate = $metricValue
    })
}

$allRows = @($rows.ToArray())
$selectedServiceCount = $allRows.Count
Assert-Condition ($selectedServiceCount -gt 0) "capacity long-run campaign selected services produced no plan rows."
$passedRows = @($allRows | Where-Object { $_.status -eq "passed" })
$failedRows = @($allRows | Where-Object { $_.status -ne "passed" })
$durationValues = @($passedRows | ForEach-Object { $_.duration_seconds } | Where-Object { $null -ne $_ })
$statusValue = "completed"
if ($failedRows.Count -gt 0 -or $passedRows.Count -ne $selectedServiceCount) {
    $statusValue = "incomplete"
}

$minimumObservedDuration = $null
$maximumObservedDuration = $null
if ($durationValues.Count -gt 0) {
    $minimumObservedDuration = ($durationValues | Measure-Object -Minimum).Minimum
    $maximumObservedDuration = ($durationValues | Measure-Object -Maximum).Maximum
}

$summaryObject = [ordered]@{
    schema_version = 1
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "NexusIM long-run capacity campaign summary; not a production SLO, benchmark guarantee, or sizing proof"
    plan_path = $resolvedPlanPath
    campaign_name = $campaignName
    status = $statusValue
    service_count = $selectedServiceCount
    plan_service_count = [int]$plan.service_count
    selected_services = @($allRows | ForEach-Object { $_.service })
    completed_service_count = $passedRows.Count
    failed_service_count = $failedRows.Count
    minimum_required_duration_seconds = $MinimumDurationSeconds
    minimum_duration_seconds = $minimumObservedDuration
    maximum_duration_seconds = $maximumObservedDuration
    rows = @($allRows)
}

$summaryDir = Split-Path -Parent $resolvedSummaryPath
if ($summaryDir -and -not (Test-Path -LiteralPath $summaryDir)) {
    New-Item -ItemType Directory -Force -Path $summaryDir | Out-Null
}
$summaryObject | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $resolvedSummaryPath -Encoding UTF8

$reportDir = Split-Path -Parent $resolvedReportPath
if ($reportDir -and -not (Test-Path -LiteralPath $reportDir)) {
    New-Item -ItemType Directory -Force -Path $reportDir | Out-Null
}

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("# NexusIM Capacity Long-Run Campaign Report")
$lines.Add("")
$lines.Add("- Campaign: $campaignName")
$lines.Add("- Status: $statusValue")
$lines.Add("- Plan: $resolvedPlanPath")
$lines.Add("- Summary: $resolvedSummaryPath")
$lines.Add("- Services completed: $($passedRows.Count)/$selectedServiceCount selected services")
$lines.Add("- Plan services: $([int]$plan.service_count)")
$lines.Add("- Minimum required duration: $MinimumDurationSeconds seconds")
$lines.Add("- Minimum observed duration: $(Format-NullableNumber $minimumObservedDuration)")
$lines.Add("- Boundary: local long-run campaign evidence only; not a production SLO, benchmark guarantee, or sizing proof.")
$lines.Add("")
$lines.Add("| Service | Runner | Mode | Status | Duration seconds | Primary metric | Summary path | Reason |")
$lines.Add("| --- | --- | --- | --- | --- | --- | --- | --- |")
foreach ($row in $allRows) {
    $metricText = ""
    if ($row.primary_rate_field -and $null -ne $row.primary_rate) {
        $metricText = "$($row.primary_rate_field)=$(Format-NullableNumber $row.primary_rate)"
    }
    $lines.Add("| $(Escape-MarkdownCell $row.service) | $(Escape-MarkdownCell $row.runner) | $(Escape-MarkdownCell $row.runner_mode) | $(Escape-MarkdownCell $row.status) | $(Format-NullableNumber $row.duration_seconds) | $(Escape-MarkdownCell $metricText) | $(Escape-MarkdownCell $row.summary_path) | $(Escape-MarkdownCell $row.reason) |")
}
$lines.Add("")
$lines.Add("Completed status means every selected service produced a capacity_summary with duration at least the configured minimum and a positive success or throughput metric. It is still not a production sizing claim.")
$lines | Set-Content -LiteralPath $resolvedReportPath -Encoding UTF8

Write-Host "OK   capacity long-run campaign summary written: $resolvedSummaryPath"
Write-Host "OK   capacity long-run campaign report written: $resolvedReportPath"

if ($statusValue -ne "completed" -and -not [bool]$AllowIncomplete) {
    throw "capacity long-run campaign is incomplete: $($failedRows.Count) failed service(s). Use -AllowIncomplete to keep an incomplete diagnostic summary."
}
