param(
    [Parameter(Mandatory = $true)]
    [string]$PlanPath,
    [switch]$DryRun,
    [switch]$ContinueOnError,
    [switch]$SkipSeededRunners,
    [switch]$SkipStackRunners,
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

function Invoke-Tool {
    param(
        [string]$Path,
        [string[]]$Arguments
    )

    & powershell -NoProfile -ExecutionPolicy Bypass -File $Path @Arguments
    return $LASTEXITCODE
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$resolvedPlanPath = Resolve-RepoPath $PlanPath
Assert-Condition (Test-Path -LiteralPath $resolvedPlanPath -PathType Leaf) "PlanPath does not exist: $resolvedPlanPath"

$suite = Join-Path $PSScriptRoot "run-loadtest-capacity-baseline-suite.ps1"
$summarizer = Join-Path $PSScriptRoot "summarize-capacity-longrun-campaign.ps1"
Assert-Condition (Test-Path -LiteralPath $suite -PathType Leaf) "Missing capacity baseline suite runner: $suite"
Assert-Condition (Test-Path -LiteralPath $summarizer -PathType Leaf) "Missing capacity long-run campaign summarizer: $summarizer"

$plan = Get-Content -LiteralPath $resolvedPlanPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$plan.schema_version -eq 1) "capacity long-run campaign plan schema_version must be 1."
Assert-Condition ((Get-JsonPropertyString -Object $plan -Name "scope") -match "not a production SLO") "capacity long-run campaign plan must state non-SLO boundary."
Assert-Condition ([int]$plan.duration_seconds -ge 1800) "capacity long-run campaign duration must be at least 30m."

$campaignName = Get-JsonPropertyString -Object $plan -Name "campaign_name"
$outputRoot = Get-JsonPropertyString -Object $plan -Name "output_root"
$runDirectory = Get-JsonPropertyString -Object $plan -Name "run_directory"
$duration = Get-JsonPropertyString -Object $plan -Name "duration"
$services = Convert-ToStringArray -Value $plan.services

Assert-Condition ($campaignName.Length -gt 0) "capacity long-run campaign plan campaign_name is required."
Assert-Condition ($outputRoot.Length -gt 0) "capacity long-run campaign plan output_root is required."
Assert-Condition ($runDirectory.Length -gt 0) "capacity long-run campaign plan run_directory is required."
Assert-Condition ($duration.Length -gt 0) "capacity long-run campaign plan duration is required."
Assert-Condition ($services.Count -gt 0) "capacity long-run campaign plan services are required."

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
