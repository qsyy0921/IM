param(
    [Parameter(Mandatory = $true)]
    [string]$PlanPath,
    [string[]]$Services = @(),
    [switch]$DryRun,
    [switch]$ContinueOnError,
    [switch]$SkipSeededRunners,
    [switch]$SkipStackRunners,
    [switch]$SkipPreflight,
    [switch]$SkipSummary,
    [string]$ReportRoot = "docs/runbook/loadtest",
    [string]$PGDSN = $env:NEXUSIM_PG_DSN,
    [string]$KafkaBrokers = "localhost:9092",
    [int]$ConversationCount = 10,
    [string]$ApiGatewayTarget = "127.0.0.1:12000",
    [string]$ConversationTarget = "127.0.0.1:10496",
    [string]$MessageTarget = "127.0.0.1:10495",
    [string]$DeliveryTarget = "127.0.0.1:10497",
    [string]$ReceiptTarget = "127.0.0.1:10499",
    [string]$PushURL = "ws://127.0.0.1:10498",
    [string]$ContactsTarget = "127.0.0.1:10500",
    [string]$IdentityTarget = "127.0.0.1:10600",
    [string]$PolicyTarget = "127.0.0.1:10800"
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

function Get-JsonPropertyString {
    param(
        $Object,
        [string]$Name
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        return ""
    }
    return ([string]$Object.$Name).Trim()
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

function Invoke-Tool {
    param(
        [string]$Path,
        [string[]]$Arguments
    )

    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $Path @Arguments 2>&1
    foreach ($line in @($output)) {
        Write-Host $line
    }
    if ($null -eq $LASTEXITCODE) {
        return 0
    }
    return [int]$LASTEXITCODE
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$resolvedPlanPath = Resolve-RepoPath $PlanPath
Assert-Condition (Test-Path -LiteralPath $resolvedPlanPath -PathType Leaf) "PlanPath does not exist: $resolvedPlanPath"

$suite = Join-Path $PSScriptRoot "run-loadtest-capacity-baseline-suite.ps1"
$summarizer = Join-Path $PSScriptRoot "summarize-capacity-longrun-campaign.ps1"
$preflight = Join-Path $PSScriptRoot "test-capacity-longrun-campaign-preflight.ps1"
Assert-Condition (Test-Path -LiteralPath $suite -PathType Leaf) "Missing capacity baseline suite runner: $suite"
Assert-Condition (Test-Path -LiteralPath $summarizer -PathType Leaf) "Missing capacity long-run campaign summarizer: $summarizer"
Assert-Condition (Test-Path -LiteralPath $preflight -PathType Leaf) "Missing capacity long-run campaign preflight: $preflight"

$plan = Get-Content -LiteralPath $resolvedPlanPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$plan.schema_version -eq 1) "capacity long-run campaign plan schema_version must be 1."
Assert-Condition ((Get-JsonPropertyString -Object $plan -Name "scope") -match "not a production SLO") "capacity long-run campaign plan must state non-SLO boundary."
Assert-Condition ([int]$plan.duration_seconds -ge 1800) "capacity long-run campaign duration must be at least 30m."

$campaignName = Get-JsonPropertyString -Object $plan -Name "campaign_name"
$outputRoot = Get-JsonPropertyString -Object $plan -Name "output_root"
$runDirectory = Get-JsonPropertyString -Object $plan -Name "run_directory"
$duration = Get-JsonPropertyString -Object $plan -Name "duration"
$planServices = Convert-ToStringArray -Value $plan.services
$requestedServices = Convert-ToRequestedServices -Values $Services
$planServiceSet = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
foreach ($service in $planServices) {
    [void]$planServiceSet.Add($service)
}
$services = if ($requestedServices.Count -gt 0) { $requestedServices } else { $planServices }
foreach ($service in $services) {
    Assert-Condition ($planServiceSet.Contains($service)) "Requested service is not in the capacity long-run campaign plan: $service"
}

Assert-Condition ($campaignName.Length -gt 0) "capacity long-run campaign plan campaign_name is required."
Assert-Condition ($outputRoot.Length -gt 0) "capacity long-run campaign plan output_root is required."
Assert-Condition ($runDirectory.Length -gt 0) "capacity long-run campaign plan run_directory is required."
Assert-Condition ($duration.Length -gt 0) "capacity long-run campaign plan duration is required."
Assert-Condition ($planServices.Count -gt 0) "capacity long-run campaign plan services are required."
Assert-Condition ($services.Count -gt 0) "capacity long-run campaign invocation services are required."

$outputRootFullPath = [System.IO.Path]::GetFullPath($outputRoot)
$runDirectoryFullPath = [System.IO.Path]::GetFullPath($runDirectory)
Assert-ExternalOutputRoot -Value $outputRootFullPath -RepositoryRoot $repoRoot -Name "Plan output_root"
Assert-Condition (Test-PathInsideDirectory -Path $resolvedPlanPath -Directory $outputRootFullPath) "PlanPath must stay under plan output_root."
Assert-Condition (Test-PathInsideDirectory -Path $runDirectoryFullPath -Directory $outputRootFullPath) "Plan run_directory must stay under output_root."

$runDirectoryName = Split-Path -Leaf $runDirectoryFullPath
Assert-Condition ($runDirectoryName -eq $campaignName) "Plan run_directory leaf must match campaign_name so suite output lands where the summarizer expects it."

$suiteArgs = New-Object System.Collections.Generic.List[string]
$suiteArgs.Add("-ResultRoot")
$suiteArgs.Add($outputRootFullPath)
$suiteArgs.Add("-RunName")
$suiteArgs.Add($campaignName)
$suiteArgs.Add("-Services")
$suiteArgs.Add(($services -join ","))
$suiteArgs.Add("-Duration")
$suiteArgs.Add($duration)
$suiteArgs.Add("-VUs")
$suiteArgs.Add([string][int]$plan.vus)
$suiteArgs.Add("-ConversationCount")
$suiteArgs.Add([string]$ConversationCount)
$suiteArgs.Add("-ApiGatewayTarget")
$suiteArgs.Add($ApiGatewayTarget)
$suiteArgs.Add("-ConversationTarget")
$suiteArgs.Add($ConversationTarget)
$suiteArgs.Add("-MessageTarget")
$suiteArgs.Add($MessageTarget)
$suiteArgs.Add("-DeliveryTarget")
$suiteArgs.Add($DeliveryTarget)
$suiteArgs.Add("-ReceiptTarget")
$suiteArgs.Add($ReceiptTarget)
$suiteArgs.Add("-PushURL")
$suiteArgs.Add($PushURL)
$suiteArgs.Add("-ContactsTarget")
$suiteArgs.Add($ContactsTarget)
$suiteArgs.Add("-IdentityTarget")
$suiteArgs.Add($IdentityTarget)
$suiteArgs.Add("-PolicyTarget")
$suiteArgs.Add($PolicyTarget)
$suiteArgs.Add("-KafkaBrokers")
$suiteArgs.Add($KafkaBrokers)
$suiteArgs.Add("-Scope")
$suiteArgs.Add("long-run capacity campaign execution for NexusIM; not a production SLO or sizing claim")

if ($PGDSN.Trim().Length -gt 0) {
    $suiteArgs.Add("-PGDSN")
    $suiteArgs.Add($PGDSN)
}
if ($DryRun) {
    $suiteArgs.Add("-DryRun")
}
if ($ContinueOnError) {
    $suiteArgs.Add("-ContinueOnError")
}
if (-not $SkipSeededRunners) {
    $suiteArgs.Add("-IncludeSeededRunners")
}
if (-not $SkipStackRunners) {
    $suiteArgs.Add("-IncludeStackRunners")
}

if (-not $DryRun -and -not $SkipPreflight) {
    $preflightArgs = @(
        "-PlanPath", $resolvedPlanPath,
        "-Services", ($services -join ","),
        "-PGDSN", $PGDSN,
        "-KafkaBrokers", $KafkaBrokers,
        "-ApiGatewayTarget", $ApiGatewayTarget,
        "-ConversationTarget", $ConversationTarget,
        "-MessageTarget", $MessageTarget,
        "-DeliveryTarget", $DeliveryTarget,
        "-ReceiptTarget", $ReceiptTarget,
        "-PushURL", $PushURL,
        "-ContactsTarget", $ContactsTarget,
        "-IdentityTarget", $IdentityTarget,
        "-PolicyTarget", $PolicyTarget
    )
    Write-Host "== capacity long-run campaign preflight =="
    $preflightExit = Invoke-Tool -Path $preflight -Arguments $preflightArgs
    if ($preflightExit -ne 0) {
        exit $preflightExit
    }
}

Write-Host "== capacity long-run campaign suite =="
Write-Host "Plan: $resolvedPlanPath"
Write-Host "Campaign: $campaignName"
Write-Host "Services: $($services -join ', ')"
Write-Host "Duration: $duration"
Write-Host "Raw output root: $outputRootFullPath"
$suiteExit = Invoke-Tool -Path $suite -Arguments @($suiteArgs.ToArray())
if ($suiteExit -ne 0) {
    exit $suiteExit
}

if ($DryRun -or $SkipSummary) {
    Write-Host "OK   capacity long-run campaign suite completed without summary generation"
    exit 0
}

$summaryArgs = @(
    "-PlanPath", $resolvedPlanPath,
    "-ReportRoot", $ReportRoot
)
Write-Host "== capacity long-run campaign summary =="
$summaryExit = Invoke-Tool -Path $summarizer -Arguments $summaryArgs
if ($summaryExit -ne 0) {
    exit $summaryExit
}
