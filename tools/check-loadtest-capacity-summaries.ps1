$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot

function New-CapacityContract {
    param(
        [string]$Service,
        [string]$Runner,
        [string]$Brief,
        [string]$BriefNeedle
    )

    return [pscustomobject]@{
        Service = $Service
        Runner = $Runner
        Brief = $Brief
        BriefNeedle = $BriefNeedle
    }
}

function Read-TextFile {
    param([string]$Path)

    return [System.IO.File]::ReadAllText($Path, [System.Text.Encoding]::UTF8)
}

function Read-GoDirectoryText {
    param([string]$Directory)

    $goFiles = Get-ChildItem -LiteralPath $Directory -Recurse -File -Filter '*.go' |
        Where-Object { $_.FullName -notmatch '\\vendor\\' } |
        Sort-Object FullName
    if (-not $goFiles) {
        throw ('No Go files found under ' + $Directory)
    }

    $parts = New-Object System.Collections.Generic.List[string]
    foreach ($file in $goFiles) {
        $parts.Add((Read-TextFile -Path $file.FullName))
    }
    return [string]::Join([Environment]::NewLine, $parts)
}

function Assert-Contains {
    param(
        [string]$Text,
        [string]$Needle,
        [string]$Message
    )

    if (-not $Text.Contains($Needle)) {
        throw $Message
    }
}

$contracts = @(
    (New-CapacityContract 'api-gateway' 'demo' 'api-gateway.md' 'loadtest/demo --gateway-facade'),
    (New-CapacityContract 'identity-service' 'identity' 'identity-service.md' 'capacity_summary'),
    (New-CapacityContract 'conversation-service' 'memberchange' 'conversation-service.md' 'capacity_summary'),
    (New-CapacityContract 'message-service' 'sendmessage' 'message-service.md' 'capacity_summary'),
    (New-CapacityContract 'delivery-service' 'delivery' 'delivery-service.md' 'capacity_summary'),
    (New-CapacityContract 'push-gateway' 'pushgateway' 'push-gateway.md' 'capacity_summary'),
    (New-CapacityContract 'receipt-service' 'receipt' 'receipt-service.md' 'capacity_summary'),
    (New-CapacityContract 'contacts-service' 'contacts' 'contacts-service.md' 'capacity_summary'),
    (New-CapacityContract 'policy-service' 'policy' 'policy-service.md' 'capacity_summary')
)

$remainingGoalsPath = Join-Path $repoRoot 'docs\runbook\remaining-goals.md'
if (-not (Test-Path -LiteralPath $remainingGoalsPath -PathType Leaf)) {
    throw ('Missing remaining goals document: ' + $remainingGoalsPath)
}
$remainingGoals = Read-TextFile -Path $remainingGoalsPath
Assert-Contains $remainingGoals 'capacity_summary' 'remaining-goals.md must mention capacity_summary.'

foreach ($contract in $contracts) {
    $runnerDir = Join-Path $repoRoot ('loadtest\' + $contract.Runner)
    if (-not (Test-Path -LiteralPath $runnerDir -PathType Container)) {
        throw ($contract.Service + ' capacity runner directory missing: loadtest\' + $contract.Runner)
    }

    $runnerText = Read-GoDirectoryText -Directory $runnerDir
    Assert-Contains $runnerText 'capacity_summary' ($contract.Service + ' runner loadtest\' + $contract.Runner + ' must expose capacity_summary.')
    Assert-Contains $runnerText 'type capacitySummary' ($contract.Service + ' runner loadtest\' + $contract.Runner + ' must define capacitySummary.')
    Assert-Contains $runnerText 'buildCapacitySummary' ($contract.Service + ' runner loadtest\' + $contract.Runner + ' must build capacity summary.')
    Assert-Contains $runnerText 'TestBuildCapacitySummary' ($contract.Service + ' runner loadtest\' + $contract.Runner + ' must test capacity summary construction.')

    $briefPath = Join-Path $repoRoot ('docs\runbook\service-briefs\' + $contract.Brief)
    if (-not (Test-Path -LiteralPath $briefPath -PathType Leaf)) {
        throw ($contract.Service + ' service brief missing: docs\runbook\service-briefs\' + $contract.Brief)
    }

    $brief = Read-TextFile -Path $briefPath
    Assert-Contains $brief $contract.BriefNeedle ($contract.Service + ' service brief must document its capacity summary runner.')
    Assert-Contains $brief 'capacity_summary' ($contract.Service + ' service brief must mention capacity_summary.')
}

Write-Host 'OK   loadtest capacity summary contracts checked for 9 services.'
