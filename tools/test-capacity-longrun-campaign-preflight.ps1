param(
    [Parameter(Mandatory = $true)]
    [string]$PlanPath,
    [string]$OutputPath = "",
    [string]$PGDSN = $env:NEXUSIM_PG_DSN,
    [string]$KafkaBrokers = "localhost:9092",
    [int]$TimeoutMilliseconds = 1500,
    [switch]$SkipNetworkChecks,
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

function Add-Endpoint {
    param(
        [System.Collections.Generic.List[object]]$List,
        [string]$Name,
        [string]$Kind,
        [string]$Target,
        [string]$Reason
    )

    $targetText = $Target.Trim()
    if ($targetText.Length -eq 0) {
        return
    }

    $hostName = ""
    $port = 0
    if ($Kind -eq "url") {
        $uri = [System.Uri]$targetText
        $hostName = $uri.Host
        $port = $uri.Port
    }
    else {
        $parts = $targetText.Split(":")
        Assert-Condition ($parts.Count -eq 2) "Endpoint $Name must be host:port: $targetText"
        $hostName = $parts[0]
        $port = [int]$parts[1]
    }

    Assert-Condition ($hostName.Trim().Length -gt 0) "Endpoint $Name host is required."
    Assert-Condition ($port -gt 0 -and $port -le 65535) "Endpoint $Name port is invalid."

    foreach ($existing in @($List)) {
        if ($existing.host -eq $hostName -and [int]$existing.port -eq $port -and $existing.name -eq $Name) {
            return
        }
    }

    $List.Add([pscustomobject]@{
        name = $Name
        kind = $Kind
        target = $targetText
        host = $hostName
        port = $port
        reason = $Reason
    })
}

function Add-PGEndpoint {
    param(
        [System.Collections.Generic.List[object]]$List,
        [string]$Value
    )

    $text = $Value.Trim()
    if ($text.Length -eq 0) {
        return
    }

    if ($text -match "^postgres(ql)?://") {
        $uri = [System.Uri]$text
        $port = if ($uri.Port -gt 0) { $uri.Port } else { 5432 }
        $List.Add([pscustomobject]@{
            name = "postgres"
            kind = "tcp"
            target = "$($uri.Host):$port"
            host = $uri.Host
            port = $port
            reason = "PGDSN"
        })
        return
    }

    if ($text -match "host=([^;\s]+).*port=([0-9]+)") {
        $List.Add([pscustomobject]@{
            name = "postgres"
            kind = "tcp"
            target = "$($Matches[1]):$($Matches[2])"
            host = $Matches[1]
            port = [int]$Matches[2]
            reason = "PGDSN"
        })
    }
}

function Add-KafkaEndpoints {
    param(
        [System.Collections.Generic.List[object]]$List,
        [string]$Value
    )

    foreach ($part in ($Value -split ",")) {
        $target = $part.Trim()
        if ($target.Length -eq 0) {
            continue
        }
        Add-Endpoint -List $List -Name "kafka" -Kind "tcp" -Target $target -Reason "KafkaBrokers"
    }
}

function Test-TcpEndpoint {
    param(
        [string]$HostName,
        [int]$Port,
        [int]$Timeout
    )

    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $task = $client.ConnectAsync($HostName, $Port)
        if (-not $task.Wait($Timeout)) {
            return [pscustomobject]@{ ok = $false; error = "timeout" }
        }
        if ($task.IsFaulted) {
            return [pscustomobject]@{ ok = $false; error = $task.Exception.GetBaseException().Message }
        }
        return [pscustomobject]@{ ok = $true; error = "" }
    }
    catch {
        return [pscustomobject]@{ ok = $false; error = $_.Exception.Message }
    }
    finally {
        $client.Close()
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$resolvedPlanPath = Resolve-RepoPath $PlanPath
Assert-Condition (Test-Path -LiteralPath $resolvedPlanPath -PathType Leaf) "PlanPath does not exist: $resolvedPlanPath"
Assert-Condition ($TimeoutMilliseconds -gt 0) "TimeoutMilliseconds must be greater than zero."

$plan = Get-Content -LiteralPath $resolvedPlanPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$plan.schema_version -eq 1) "capacity long-run campaign plan schema_version must be 1."
Assert-Condition ((Get-JsonPropertyString -Object $plan -Name "scope") -match "not a production SLO") "capacity long-run campaign plan must state non-SLO boundary."
Assert-Condition ([int]$plan.duration_seconds -ge 1800) "capacity long-run campaign duration must be at least 30m."

$campaignName = Get-JsonPropertyString -Object $plan -Name "campaign_name"
$outputRoot = Get-JsonPropertyString -Object $plan -Name "output_root"
$runDirectory = Get-JsonPropertyString -Object $plan -Name "run_directory"
$services = Convert-ToStringArray -Value $plan.services
Assert-Condition ($campaignName.Length -gt 0) "capacity long-run campaign plan campaign_name is required."
Assert-Condition ($outputRoot.Length -gt 0) "capacity long-run campaign plan output_root is required."
Assert-Condition ($runDirectory.Length -gt 0) "capacity long-run campaign plan run_directory is required."
Assert-Condition ($services.Count -gt 0) "capacity long-run campaign plan services are required."

$outputRootFullPath = [System.IO.Path]::GetFullPath($outputRoot)
$runDirectoryFullPath = [System.IO.Path]::GetFullPath($runDirectory)
Assert-ExternalOutputRoot -Value $outputRootFullPath -RepositoryRoot $repoRoot -Name "Plan output_root"
Assert-Condition (Test-PathInsideDirectory -Path $resolvedPlanPath -Directory $outputRootFullPath) "PlanPath must stay under plan output_root."
Assert-Condition (Test-PathInsideDirectory -Path $runDirectoryFullPath -Directory $outputRootFullPath) "Plan run_directory must stay under output_root."

if ($OutputPath.Trim().Length -eq 0) {
    $resolvedOutputPath = Join-Path $runDirectoryFullPath "capacity-longrun-campaign-preflight.json"
}
else {
    $resolvedOutputPath = Resolve-RepoPath $OutputPath
}
Assert-Condition (Test-PathInsideDirectory -Path $resolvedOutputPath -Directory $outputRootFullPath) "OutputPath must stay under plan output_root."

$endpoints = New-Object System.Collections.Generic.List[object]
foreach ($service in $services) {
    switch ($service) {
        "api-gateway" {
            Add-Endpoint -List $endpoints -Name "api-gateway" -Kind "tcp" -Target $ApiGatewayTarget -Reason $service
        }
        "identity-service" {
            Add-Endpoint -List $endpoints -Name "identity-service" -Kind "tcp" -Target $IdentityTarget -Reason $service
            Add-PGEndpoint -List $endpoints -Value $PGDSN
        }
        "message-service" {
            Add-Endpoint -List $endpoints -Name "message-service" -Kind "tcp" -Target $MessageTarget -Reason $service
            Add-PGEndpoint -List $endpoints -Value $PGDSN
        }
        "conversation-service" {
            Add-Endpoint -List $endpoints -Name "conversation-service" -Kind "tcp" -Target $ConversationTarget -Reason $service
            Add-PGEndpoint -List $endpoints -Value $PGDSN
        }
        "delivery-service" {
            Add-Endpoint -List $endpoints -Name "delivery-service" -Kind "tcp" -Target $DeliveryTarget -Reason $service
            Add-PGEndpoint -List $endpoints -Value $PGDSN
        }
        "push-gateway" {
            Add-Endpoint -List $endpoints -Name "conversation-service" -Kind "tcp" -Target $ConversationTarget -Reason $service
            Add-Endpoint -List $endpoints -Name "message-service" -Kind "tcp" -Target $MessageTarget -Reason $service
            Add-Endpoint -List $endpoints -Name "delivery-service" -Kind "tcp" -Target $DeliveryTarget -Reason $service
            Add-Endpoint -List $endpoints -Name "push-gateway" -Kind "url" -Target $PushURL -Reason $service
            Add-PGEndpoint -List $endpoints -Value $PGDSN
        }
        "receipt-service" {
            Add-Endpoint -List $endpoints -Name "conversation-service" -Kind "tcp" -Target $ConversationTarget -Reason $service
            Add-Endpoint -List $endpoints -Name "message-service" -Kind "tcp" -Target $MessageTarget -Reason $service
            Add-Endpoint -List $endpoints -Name "delivery-service" -Kind "tcp" -Target $DeliveryTarget -Reason $service
            Add-Endpoint -List $endpoints -Name "receipt-service" -Kind "tcp" -Target $ReceiptTarget -Reason $service
            Add-PGEndpoint -List $endpoints -Value $PGDSN
        }
        "contacts-service" {
            Add-Endpoint -List $endpoints -Name "contacts-service" -Kind "tcp" -Target $ContactsTarget -Reason $service
            Add-KafkaEndpoints -List $endpoints -Value $KafkaBrokers
            Add-PGEndpoint -List $endpoints -Value $PGDSN
        }
        "policy-service" {
            Add-Endpoint -List $endpoints -Name "policy-service" -Kind "tcp" -Target $PolicyTarget -Reason $service
        }
        default {
            throw "Unknown service in capacity long-run campaign preflight: $service"
        }
    }
}

$results = New-Object System.Collections.Generic.List[object]
foreach ($endpoint in @($endpoints.ToArray())) {
    $status = if ($SkipNetworkChecks) {
        [pscustomobject]@{ ok = $true; error = "skipped" }
    }
    else {
        Test-TcpEndpoint -HostName $endpoint.host -Port ([int]$endpoint.port) -Timeout $TimeoutMilliseconds
    }
    $results.Add([pscustomobject]@{
        name = $endpoint.name
        kind = $endpoint.kind
        target = $endpoint.target
        host = $endpoint.host
        port = [int]$endpoint.port
        reason = $endpoint.reason
        ok = [bool]$status.ok
        error = [string]$status.error
    })
}

$resultRows = @($results.ToArray())
$failed = @($resultRows | Where-Object { -not $_.ok })
$summary = [ordered]@{
    schema_version = 1
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "NexusIM long-run capacity campaign preflight; TCP readiness only, not a production SLO or capacity proof"
    plan_path = $resolvedPlanPath
    campaign_name = $campaignName
    status = if ($failed.Count -eq 0) { "passed" } else { "failed" }
    skip_network_checks = [bool]$SkipNetworkChecks
    timeout_milliseconds = $TimeoutMilliseconds
    service_count = $services.Count
    endpoint_count = $resultRows.Count
    failed_endpoint_count = $failed.Count
    services = @($services)
    endpoints = @($resultRows)
}

$outputDir = Split-Path -Parent $resolvedOutputPath
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$summary | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
Write-Host "OK   capacity long-run campaign preflight written: $resolvedOutputPath"

if ($failed.Count -gt 0) {
    foreach ($row in $failed) {
        Write-Host "FAIL $($row.name) $($row.target): $($row.error)" -ForegroundColor Red
    }
    exit 1
}
