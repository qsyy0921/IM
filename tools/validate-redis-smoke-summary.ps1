param(
    [Parameter(Mandatory = $true)]
    [string]$SummaryPath,
    [string]$ExpectedRedisMode = "",
    [string]$ExpectedScenario = "",
    [switch]$RequireCleanGit,
    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Resolve-ReferencedPath {
    param(
        [string]$BaseDir,
        [string]$PathValue,
        [string]$FieldName
    )

    $trimmed = $PathValue.Trim()
    Assert-Condition ($trimmed.Length -gt 0) "$FieldName is required."
    if ([System.IO.Path]::IsPathRooted($trimmed)) {
        return [System.IO.Path]::GetFullPath($trimmed)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $BaseDir $trimmed))
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

$resolvedSummaryPath = [System.IO.Path]::GetFullPath($SummaryPath)
Assert-Condition (Test-Path -LiteralPath $resolvedSummaryPath -PathType Leaf) "SummaryPath does not exist: $resolvedSummaryPath"

$summaryDir = Split-Path -Parent $resolvedSummaryPath
$summary = Get-Content -LiteralPath $resolvedSummaryPath -Raw | ConvertFrom-Json

$runName = Get-JsonPropertyString -Object $summary -Name "run_name"
$redisMode = Get-JsonPropertyString -Object $summary -Name "redis_mode"
$gitCommit = Get-JsonPropertyString -Object $summary -Name "git_commit"
$completedAt = Get-JsonPropertyString -Object $summary -Name "completed_at"

Assert-Condition ($runName.Length -gt 0) "redis smoke summary run_name is required."
Assert-Condition ($gitCommit.Length -gt 0) "redis smoke summary git_commit is required."
Assert-Condition ($redisMode -in @("sentinel", "cluster")) "redis smoke summary redis_mode must be sentinel or cluster."
Assert-Condition ($completedAt.Length -gt 0) "redis smoke summary completed_at is required."
if ($ExpectedRedisMode.Trim().Length -gt 0) {
    Assert-Condition ($redisMode -eq $ExpectedRedisMode.Trim()) "redis smoke summary redis_mode $redisMode did not match expected $ExpectedRedisMode."
}
if ($RequireCleanGit) {
    Assert-Condition (-not [bool]$summary.git_dirty) "redis smoke summary git_dirty must be false when RequireCleanGit is set."
}

$pushgatewaySummaryPath = Resolve-ReferencedPath `
    -BaseDir $summaryDir `
    -PathValue (Get-JsonPropertyString -Object $summary -Name "pushgateway_summary") `
    -FieldName "pushgateway_summary"
Assert-Condition (Test-Path -LiteralPath $pushgatewaySummaryPath -PathType Leaf) "Referenced pushgateway_summary does not exist: $pushgatewaySummaryPath"

$push = Get-Content -LiteralPath $pushgatewaySummaryPath -Raw | ConvertFrom-Json
$scenario = Get-JsonPropertyString -Object $push -Name "scenario"
Assert-Condition ($scenario.Length -gt 0) "pushgateway summary scenario is required."
if ($ExpectedScenario.Trim().Length -gt 0) {
    Assert-Condition ($scenario -eq $ExpectedScenario.Trim()) "pushgateway summary scenario $scenario did not match expected $ExpectedScenario."
}

Assert-Condition ([bool]$push.success) "pushgateway summary success must be true."
Assert-Condition ((Get-JsonPropertyString -Object $push -Name "route_backend") -eq "redis") "pushgateway summary route_backend must be redis."
Assert-Condition ([int64]$push.delivery_outbox_published -gt 0) "pushgateway summary delivery_outbox_published must be positive."
Assert-Condition ([int64]$push.delivery_outbox_pending -eq 0) "pushgateway summary delivery_outbox_pending must be 0."
Assert-Condition ([int64]$push.delivery_outbox_dlq -eq 0) "pushgateway summary delivery_outbox_dlq must be 0."
Assert-Condition ([int64]$push.pull_inbox.item_count -gt 0) "pushgateway summary pull_inbox.item_count must be positive."

$faultObserved = $false
$capacityObserved = $false

if ($null -ne $push.redis_fault) {
    $faultObserved = $true
    Assert-Condition ([int64]$push.redis_fault.recovery_pull_inbox.item_count -gt 0) "redis_fault recovery_pull_inbox.item_count must be positive."
    Assert-Condition ([int64]$push.redis_fault.recovery_pull_inbox.max_seq -ge [int64]$push.send_message.conversation_seq) "redis_fault recovery_pull_inbox.max_seq must cover sent message seq."
    Assert-Condition ((Get-JsonPropertyString -Object $push.redis_fault.ack_ok -Name "op") -eq "delivery.ack.ok") "redis_fault ack_ok.op must be delivery.ack.ok."
    Assert-Condition ([int64]$push.redis_fault.delivery_outbox_total -gt 0) "redis_fault delivery_outbox_total must be positive."
    if ($scenario -eq "redis-cluster-failover") {
        Assert-Condition ([bool]$push.redis_fault.notify_received) "redis-cluster-failover must receive online notify after promoted master."
    }
    elseif ($scenario -match "quorum-loss|network-partition|node-stop|redis-fault") {
        Assert-Condition (-not [bool]$push.redis_fault.notify_received) "$scenario should prove PullInbox recovery after online notify miss."
    }
}

if ($null -ne $push.capacity_summary) {
    $capacityObserved = $true
    $expectedMessageCount = 0
    if ($null -ne $summary.PSObject.Properties["message_count"]) {
        $expectedMessageCount = [int]$summary.message_count
    }
    elseif ($null -ne $push.PSObject.Properties["message_count"]) {
        $expectedMessageCount = [int]$push.message_count
    }

    if ($expectedMessageCount -gt 0) {
        Assert-Condition ([int]$push.capacity_summary.message_count -eq $expectedMessageCount) "capacity_summary.message_count must match expected message count $expectedMessageCount."
        Assert-Condition ([int]$push.pull_inbox.item_count -ge $expectedMessageCount) "pull_inbox.item_count must cover expected message count $expectedMessageCount."
        Assert-Condition ([int]$push.capacity_summary.notify_frame_count -ge $expectedMessageCount) "capacity_summary.notify_frame_count must cover expected message count $expectedMessageCount."
    }
    Assert-Condition ([double]$push.capacity_summary.duration_ms -gt 0) "capacity_summary.duration_ms must be positive."
}

Assert-Condition ($faultObserved -or $capacityObserved) "redis smoke validation requires redis_fault or capacity_summary evidence."

$validation = [pscustomobject]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    summary_path = $resolvedSummaryPath
    pushgateway_summary_path = $pushgatewaySummaryPath
    run_name = $runName
    redis_mode = $redisMode
    scenario = $scenario
    fault_evidence = $faultObserved
    capacity_evidence = $capacityObserved
    valid = $true
    scope = "local Redis smoke summary validation; not a production Redis HA or capacity SLO"
}

if ($OutputPath.Trim().Length -gt 0) {
    $resolvedOutputPath = [System.IO.Path]::GetFullPath($OutputPath)
    $outputDir = Split-Path -Parent $resolvedOutputPath
    if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    $validation | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
    Write-Host "OK   redis smoke summary validation written: $resolvedOutputPath"
}
else {
    $validation | ConvertTo-Json -Depth 5
}
