param(
    [string]$OutputRoot = "H:\NexusIM\loadtest-results",
    [string]$CampaignName = ("capacity-longrun-" + (Get-Date).ToString("yyyyMMdd-HHmmss")),
    [string[]]$Services = @(
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
    [ValidateSet("single-service", "stack", "distributed")]
    [string]$Profile = "stack",
    [ValidateSet("steady", "ramp", "soak")]
    [string]$WorkloadMode = "soak",
    [string]$Duration = "30m",
    [string]$Warmup = "2m",
    [int]$VUs = 10,
    [int]$MaxVUs = 50,
    [string]$TargetEnvironment = "local-or-target",
    [string]$Operator = "local-operator",
    [string]$Notes = "Long-run capacity campaign plan only; raw evidence must stay outside the repository.",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
. (Join-Path $PSScriptRoot "evidence-metadata-safety.ps1")

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Convert-ToDurationSeconds {
    param(
        [string]$Value,
        [string]$Name
    )

    $text = ([string]$Value).Trim().ToLowerInvariant()
    if ($text -notmatch "^([0-9]+)(s|m|h)$") {
        throw "$Name must use a simple duration suffix like 30m or 2h."
    }

    $amount = [int]$Matches[1]
    $unit = $Matches[2]
    Assert-Condition ($amount -gt 0) "$Name must be greater than zero."
    switch ($unit) {
        "s" { return $amount }
        "m" { return $amount * 60 }
        "h" { return $amount * 3600 }
        default { throw "$Name has unsupported duration unit: $unit" }
    }
}

function Convert-ToServiceList {
    param([string[]]$Values)

    $allowed = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($service in @(
            "api-gateway",
            "identity-service",
            "message-service",
            "conversation-service",
            "delivery-service",
            "push-gateway",
            "receipt-service",
            "contacts-service",
            "policy-service"
        )) {
        [void]$allowed.Add($service)
    }

    $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    $list = New-Object System.Collections.Generic.List[string]
    foreach ($value in @($Values)) {
        foreach ($part in (([string]$value) -split "[,;]")) {
            $service = $part.Trim()
            if ($service.Length -eq 0) {
                continue
            }
            Assert-Condition ($allowed.Contains($service)) "Unknown service for capacity long-run campaign: $service"
            if ($seen.Add($service)) {
                $list.Add($service)
            }
        }
    }
    Assert-Condition ($list.Count -gt 0) "At least one service is required."
    return @($list.ToArray())
}

function Get-RunnerForService {
    param([string]$Service)

    switch ($Service) {
        "api-gateway" { return "demo" }
        "identity-service" { return "identity" }
        "message-service" { return "sendmessage" }
        "conversation-service" { return "memberchange" }
        "delivery-service" { return "delivery" }
        "push-gateway" { return "pushgateway" }
        "receipt-service" { return "receipt" }
        "contacts-service" { return "contacts" }
        "policy-service" { return "policy" }
        default { throw "No loadtest runner mapping for service: $Service" }
    }
}

function Get-RunnerMode {
    param([string]$Service)

    switch ($Service) {
        "message-service" { return "seeded" }
        "conversation-service" { return "seeded" }
        "delivery-service" { return "seeded" }
        "api-gateway" { return "stack" }
        "identity-service" { return "stack" }
        "push-gateway" { return "stack" }
        "receipt-service" { return "stack" }
        "contacts-service" { return "stack" }
        default { return "direct" }
    }
}

function New-Step {
    param(
        [string]$Service,
        [string]$RunDirectory
    )

    $runner = Get-RunnerForService -Service $Service
    $mode = Get-RunnerMode -Service $Service
    return [ordered]@{
        service = $Service
        runner = $runner
        runner_mode = $mode
        result_dir = (Join-Path $RunDirectory $Service)
        command_hint = "go run ./loadtest/$runner --duration $Duration --vus $VUs --result-root <external-result-root>"
        requires_seed = ($mode -eq "seeded")
        requires_runtime_stack = ($mode -eq "stack")
        capacity_summary_required = $true
    }
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
    $Value | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Write-MarkdownFile {
    param(
        [string]$Path,
        $Plan
    )

    $lines = New-Object System.Collections.Generic.List[string]
    $lines.Add("# NexusIM Capacity Long-Run Campaign Plan")
    $lines.Add("")
    $lines.Add("This is an execution plan, not a production SLO or sizing proof.")
    $lines.Add("")
    $lines.Add("- Campaign: ``$($Plan.campaign_name)``")
    $lines.Add("- Profile: ``$($Plan.profile)``")
    $lines.Add("- Workload mode: ``$($Plan.workload_mode)``")
    $lines.Add("- Duration: ``$($Plan.duration)``")
    $lines.Add("- Warmup: ``$($Plan.warmup)``")
    $lines.Add("- VUs: ``$($Plan.vus)``")
    $lines.Add("- Max VUs: ``$($Plan.max_vus)``")
    $lines.Add("- Raw output root: ``$($Plan.output_root)``")
    $lines.Add("")
    $lines.Add("## Service Steps")
    $lines.Add("")
    $lines.Add("| Service | Runner | Mode | Seed | Stack |")
    $lines.Add("| --- | --- | --- | --- | --- |")
    foreach ($step in @($Plan.steps)) {
        $lines.Add("| ``$($step.service)`` | ``$($step.runner)`` | ``$($step.runner_mode)`` | ``$($step.requires_seed)`` | ``$($step.requires_runtime_stack)`` |")
    }
    $lines.Add("")
    $lines.Add("Raw summaries and detailed artifacts must stay under the external output root. Repository documents should only keep low-sensitive reports and evidence references.")
    $lines | Set-Content -LiteralPath $Path -Encoding UTF8
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
Assert-ExternalOutputRoot -Value $OutputRoot -RepositoryRoot $repoRoot -Name "OutputRoot"
Assert-Condition ($CampaignName -match "^[A-Za-z0-9][A-Za-z0-9._-]{2,120}$") "CampaignName must be a safe file name segment."
Assert-Condition (-not $CampaignName.Contains("..")) "CampaignName must not contain path traversal."
Assert-Condition ($VUs -gt 0) "VUs must be greater than zero."
Assert-Condition ($MaxVUs -ge $VUs) "MaxVUs must be greater than or equal to VUs."
Assert-LowSensitiveEvidenceText -Value $CampaignName -FieldName "CampaignName" -MaxLength 128
Assert-LowSensitiveEvidenceText -Value $TargetEnvironment -FieldName "TargetEnvironment" -MaxLength 128
Assert-LowSensitiveEvidenceText -Value $Operator -FieldName "Operator" -MaxLength 128
Assert-LowSensitiveEvidenceText -Value $Notes -FieldName "Notes" -MaxLength 512

$durationSeconds = Convert-ToDurationSeconds -Value $Duration -Name "Duration"
$warmupSeconds = Convert-ToDurationSeconds -Value $Warmup -Name "Warmup"
Assert-Condition ($durationSeconds -ge 1800) "Duration must be at least 30m for a long-run capacity campaign plan."
Assert-Condition ($warmupSeconds -lt $durationSeconds) "Warmup must be shorter than Duration."

$serviceList = Convert-ToServiceList -Values $Services
$outputFullRoot = [System.IO.Path]::GetFullPath($OutputRoot)
$runDirectory = Join-Path $outputFullRoot $CampaignName
if ((Test-Path -LiteralPath $runDirectory) -and -not $Force) {
    throw "Campaign output directory already exists: $runDirectory. Use -Force to overwrite the plan files."
}
New-Item -ItemType Directory -Force -Path $runDirectory | Out-Null

$steps = @()
foreach ($service in $serviceList) {
    $steps += [pscustomobject](New-Step -Service $service -RunDirectory $runDirectory)
}

$plan = [ordered]@{
    schema_version = 1
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "NexusIM long-run capacity campaign plan; not a production SLO, benchmark result, or sizing proof"
    campaign_name = $CampaignName
    profile = $Profile
    workload_mode = $WorkloadMode
    duration = $Duration
    duration_seconds = $durationSeconds
    warmup = $Warmup
    warmup_seconds = $warmupSeconds
    vus = $VUs
    max_vus = $MaxVUs
    target_environment = $TargetEnvironment
    operator = $Operator
    notes = $Notes
    output_root = $outputFullRoot
    run_directory = $runDirectory
    service_count = @($steps).Count
    services = @($serviceList)
    steps = @($steps)
}

$planPath = Join-Path $runDirectory "capacity-longrun-campaign-plan.json"
$markdownPath = Join-Path $runDirectory "capacity-longrun-campaign-plan.md"
Write-JsonFile -Path $planPath -Value $plan
Write-MarkdownFile -Path $markdownPath -Plan ([pscustomobject]$plan)

Write-Host "OK   capacity long-run campaign plan written: $planPath"
Write-Host "OK   capacity long-run campaign markdown written: $markdownPath"
