param(
    [string]$ManifestPath = "docs/runbook/distributed-smoke-evidence.json",
    [switch]$RequireFiles,
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

function Get-JsonArrayCount {
    param(
        $Object,
        [string]$Name
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name] -or $null -eq $Object.$Name) {
        return 0
    }
    return @($Object.$Name).Count
}

function Resolve-RepoPath {
    param(
        [string]$PathValue
    )

    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return [System.IO.Path]::GetFullPath($PathValue)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $repoRoot $PathValue))
}

function Validate-PushGatewaySummary {
    param(
        [string]$Path,
        [string]$ExpectedScenario = ""
    )

    Assert-Condition (Test-Path -LiteralPath $Path -PathType Leaf) "pushgateway summary does not exist: $Path"
    $push = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    Assert-Condition ([bool]$push.success) "pushgateway summary success must be true: $Path"
    if ($ExpectedScenario.Trim().Length -gt 0) {
        Assert-Condition ((Get-JsonPropertyString -Object $push -Name "scenario") -eq $ExpectedScenario.Trim()) "pushgateway summary scenario mismatch: $Path"
    }
    Assert-Condition ([int64]$push.delivery_outbox_published -gt 0) "pushgateway summary delivery_outbox_published must be positive: $Path"
    Assert-Condition ([int64]$push.delivery_outbox_pending -eq 0) "pushgateway summary delivery_outbox_pending must be 0: $Path"
    Assert-Condition ([int64]$push.delivery_outbox_dlq -eq 0) "pushgateway summary delivery_outbox_dlq must be 0: $Path"
    Assert-Condition ([int64]$push.pull_inbox.item_count -gt 0) "pushgateway summary pull_inbox.item_count must be positive: $Path"

    $ackOp = Get-JsonPropertyString -Object $push.delivery_ack_ok -Name "op"
    if ($ackOp.Length -eq 0 -and (Get-JsonArrayCount -Object $push -Name "device_notifications") -gt 0) {
        $ackOp = Get-JsonPropertyString -Object @($push.device_notifications)[0].delivery_ack_ok -Name "op"
    }
    Assert-Condition ($ackOp -eq "delivery.ack.ok") "pushgateway summary delivery_ack_ok.op must be delivery.ack.ok: $Path"
}

function Validate-KafkaFailover {
    param(
        [string]$Path
    )

    $summary = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    $beforeLeader = Get-JsonPropertyString -Object $summary -Name "before_leader_broker_id"
    $afterLeader = Get-JsonPropertyString -Object $summary -Name "after_leader_broker_id"
    Assert-Condition ($beforeLeader.Length -gt 0) "kafka failover before_leader_broker_id is required."
    Assert-Condition ($afterLeader.Length -gt 0) "kafka failover after_leader_broker_id is required."
    Assert-Condition ($beforeLeader -ne $afterLeader) "kafka failover after leader must differ from before leader."
    Validate-PushGatewaySummary -Path (Resolve-RepoPath (Get-JsonPropertyString -Object $summary -Name "before_summary"))
    Validate-PushGatewaySummary -Path (Resolve-RepoPath (Get-JsonPropertyString -Object $summary -Name "after_summary"))
}

function Validate-KafkaISRFlapping {
    param(
        [string]$Path
    )

    $summary = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    Assert-Condition ([int]$summary.flap_cycles -ge 1) "kafka isr flapping flap_cycles must be positive."
    Assert-Condition ((Get-JsonArrayCount -Object $summary -Name "cycles") -eq [int]$summary.flap_cycles) "kafka isr flapping cycles count must match flap_cycles."
    foreach ($cycle in @($summary.cycles)) {
        Assert-Condition ([bool]$cycle.degraded_probe.accepted) "kafka isr flapping degraded probe must be accepted."
        Assert-Condition ([bool]$cycle.restored_probe.accepted) "kafka isr flapping restored probe must be accepted."
        foreach ($state in @($cycle.degraded_topic_state)) {
            Assert-Condition ([int]$state.replica_count -eq 3) "kafka isr flapping degraded replica_count must be 3."
            Assert-Condition ([int]$state.isr_count -eq 2) "kafka isr flapping degraded isr_count must be 2."
        }
        foreach ($state in @($cycle.restored_topic_state)) {
            Assert-Condition ([int]$state.replica_count -eq 3) "kafka isr flapping restored replica_count must be 3."
            Assert-Condition ([int]$state.isr_count -eq 3) "kafka isr flapping restored isr_count must be 3."
        }
    }
}

function Validate-KafkaProducerFault {
    param(
        [string]$Path
    )

    $summary = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    Assert-Condition ([int]$summary.producer_attempted -gt 0) "kafka producer fault producer_attempted must be positive."
    Assert-Condition ([int]$summary.producer_acked -eq [int]$summary.producer_attempted) "kafka producer fault producer_acked must match attempted."
    Assert-Condition ([int]$summary.producer_failed -eq 0) "kafka producer fault producer_failed must be 0."
    Assert-Condition ([int]$summary.consumed_unique -eq [int]$summary.producer_acked) "kafka producer fault consumed_unique must match acked."
    Assert-Condition ([int]$summary.missing_acked_count -eq 0) "kafka producer fault missing_acked_count must be 0."
    Assert-Condition ([int]$summary.unacked_observed_count -eq 0) "kafka producer fault unacked_observed_count must be 0."
}

function Validate-KafkaConsumerChurn {
    param(
        [string]$Path
    )

    $summary = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    Assert-Condition ([int]$summary.churn_cycles -ge 1) "kafka consumer churn churn_cycles must be positive."
    Assert-Condition ((Get-JsonArrayCount -Object $summary -Name "transitions") -ge ([int]$summary.churn_cycles * 4)) "kafka consumer churn transitions must cover stop/start actions."
    foreach ($transition in @($summary.transitions)) {
        Assert-Condition ((Get-JsonPropertyString -Object $transition.snapshot -Name "state") -eq "Stable") "kafka consumer churn snapshot state must be Stable."
        Assert-Condition ([int]$transition.snapshot.member_count -eq [int]$transition.expected_members) "kafka consumer churn member_count must match expected_members."
        Assert-Condition ([int]$transition.probe.acked -eq [int]$transition.probe.attempted) "kafka consumer churn probe acked must match attempted."
        Assert-Condition ([int]$transition.probe.failed -eq 0) "kafka consumer churn probe failed must be 0."
        Assert-Condition ((Get-JsonPropertyString -Object $transition.post_probe_snapshot -Name "state") -eq "Stable") "kafka consumer churn post_probe_snapshot state must be Stable."
        Assert-Condition ([int]$transition.post_probe_snapshot.total_lag -eq 0) "kafka consumer churn post_probe_snapshot total_lag must be 0."
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$resolvedManifestPath = Resolve-RepoPath $ManifestPath
Assert-Condition (Test-Path -LiteralPath $resolvedManifestPath -PathType Leaf) "ManifestPath does not exist: $resolvedManifestPath"

$manifest = Get-Content -LiteralPath $resolvedManifestPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$manifest.schema_version -eq 1) "distributed smoke evidence schema_version must be 1."
Assert-Condition ((Get-JsonPropertyString -Object $manifest -Name "scope").Length -gt 0) "distributed smoke evidence scope is required."
Assert-Condition ((Get-JsonArrayCount -Object $manifest -Name "entries") -gt 0) "distributed smoke evidence entries are required."

$knownKinds = @(
    "pushgateway-full",
    "redis-smoke",
    "postgres-smoke",
    "kafka-failover",
    "kafka-isr-flapping",
    "kafka-producer-fault",
    "kafka-consumer-churn"
)
$seenNames = @{}
$validatedFiles = 0

foreach ($entry in @($manifest.entries)) {
    $name = Get-JsonPropertyString -Object $entry -Name "name"
    $kind = Get-JsonPropertyString -Object $entry -Name "kind"
    $summaryPath = Get-JsonPropertyString -Object $entry -Name "summary_path"

    Assert-Condition ($name.Length -gt 0) "distributed smoke evidence entry name is required."
    Assert-Condition (-not $seenNames.ContainsKey($name)) "duplicate distributed smoke evidence entry name: $name"
    $seenNames[$name] = $true
    Assert-Condition ($kind -in $knownKinds) "distributed smoke evidence entry $name has unknown kind: $kind"
    Assert-Condition ($summaryPath.Length -gt 0) "distributed smoke evidence entry $name summary_path is required."

    if (-not $RequireFiles) {
        continue
    }

    $resolvedSummaryPath = Resolve-RepoPath $summaryPath
    Assert-Condition (Test-Path -LiteralPath $resolvedSummaryPath -PathType Leaf) "distributed smoke evidence entry $name summary_path does not exist: $resolvedSummaryPath"

    $entrySummary = Get-Content -LiteralPath $resolvedSummaryPath -Raw | ConvertFrom-Json
    if ([bool]$entry.require_clean_git) {
        Assert-Condition (-not [bool]$entrySummary.git_dirty) "distributed smoke evidence entry $name requires clean git summary."
    }

    switch ($kind) {
        "pushgateway-full" {
            Validate-PushGatewaySummary -Path $resolvedSummaryPath -ExpectedScenario (Get-JsonPropertyString -Object $entry -Name "expected_scenario")
        }
        "redis-smoke" {
            $invocationArgs = @{
                SummaryPath = $resolvedSummaryPath
                ExpectedRedisMode = (Get-JsonPropertyString -Object $entry -Name "expected_redis_mode")
                ExpectedScenario = (Get-JsonPropertyString -Object $entry -Name "expected_scenario")
            }
            if ([bool]$entry.require_clean_git) {
                $invocationArgs.RequireCleanGit = $true
            }
            & (Join-Path $PSScriptRoot "validate-redis-smoke-summary.ps1") @invocationArgs | Out-Null
        }
        "postgres-smoke" {
            $invocationArgs = @{
                SummaryPath = $resolvedSummaryPath
                ExpectedScenario = (Get-JsonPropertyString -Object $entry -Name "expected_scenario")
            }
            if ([bool]$entry.require_clean_git) {
                $invocationArgs.RequireCleanGit = $true
            }
            & (Join-Path $PSScriptRoot "validate-postgres-smoke-summary.ps1") @invocationArgs | Out-Null
        }
        "kafka-failover" {
            Validate-KafkaFailover -Path $resolvedSummaryPath
        }
        "kafka-isr-flapping" {
            Validate-KafkaISRFlapping -Path $resolvedSummaryPath
        }
        "kafka-producer-fault" {
            Validate-KafkaProducerFault -Path $resolvedSummaryPath
        }
        "kafka-consumer-churn" {
            Validate-KafkaConsumerChurn -Path $resolvedSummaryPath
        }
    }
    $validatedFiles++
}

$validation = [pscustomobject]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    manifest_path = $resolvedManifestPath
    entry_count = @($manifest.entries).Count
    files_required = [bool]$RequireFiles
    validated_files = $validatedFiles
    valid = $true
    scope = "local distributed smoke evidence manifest validation; not a production HA or SLO claim"
}

if ($OutputPath.Trim().Length -gt 0) {
    $resolvedOutputPath = Resolve-RepoPath $OutputPath
    $outputDir = Split-Path -Parent $resolvedOutputPath
    if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    $validation | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
    Write-Host "OK   distributed smoke evidence validation written: $resolvedOutputPath"
}
else {
    $validation | ConvertTo-Json -Depth 5
}
