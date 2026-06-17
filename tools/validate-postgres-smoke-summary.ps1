param(
    [Parameter(Mandatory = $true)]
    [string]$SummaryPath,
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

function Test-JsonPropertyExists {
    param(
        $Object,
        [string]$Name
    )

    return ($null -ne $Object -and $null -ne $Object.PSObject.Properties[$Name])
}

function Validate-PushGatewaySummary {
    param(
        [string]$Path,
        [string]$FieldName
    )

    Assert-Condition (Test-Path -LiteralPath $Path -PathType Leaf) "Referenced $FieldName does not exist: $Path"
    $push = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json

    Assert-Condition ([bool]$push.success) "$FieldName success must be true."
    Assert-Condition ((Get-JsonPropertyString -Object $push -Name "scenario").Length -gt 0) "$FieldName scenario is required."
    Assert-Condition ([int64]$push.delivery_outbox_published -gt 0) "$FieldName delivery_outbox_published must be positive."
    Assert-Condition ([int64]$push.delivery_outbox_pending -eq 0) "$FieldName delivery_outbox_pending must be 0."
    Assert-Condition ([int64]$push.delivery_outbox_dlq -eq 0) "$FieldName delivery_outbox_dlq must be 0."
    Assert-Condition ([int64]$push.pull_inbox.item_count -gt 0) "$FieldName pull_inbox.item_count must be positive."
    Assert-Condition ([int64]$push.pull_inbox.max_seq -ge [int64]$push.send_message.conversation_seq) "$FieldName pull_inbox.max_seq must cover send_message.conversation_seq."

    $ackOp = Get-JsonPropertyString -Object $push.delivery_ack_ok -Name "op"
    if ($ackOp.Length -eq 0 -and (Get-JsonArrayCount -Object $push -Name "device_notifications") -gt 0) {
        $ackOp = Get-JsonPropertyString -Object @($push.device_notifications)[0].delivery_ack_ok -Name "op"
    }
    Assert-Condition ($ackOp -eq "delivery.ack.ok") "$FieldName delivery_ack_ok.op must be delivery.ack.ok."

    return [pscustomobject]@{
        Path = $Path
        Scenario = Get-JsonPropertyString -Object $push -Name "scenario"
    }
}

$resolvedSummaryPath = [System.IO.Path]::GetFullPath($SummaryPath)
Assert-Condition (Test-Path -LiteralPath $resolvedSummaryPath -PathType Leaf) "SummaryPath does not exist: $resolvedSummaryPath"

$summaryDir = Split-Path -Parent $resolvedSummaryPath
$summary = Get-Content -LiteralPath $resolvedSummaryPath -Raw | ConvertFrom-Json

$runName = Get-JsonPropertyString -Object $summary -Name "run_name"
$gitCommit = Get-JsonPropertyString -Object $summary -Name "git_commit"
$pgDsn = Get-JsonPropertyString -Object $summary -Name "pg_dsn"
$postgresExecContainer = Get-JsonPropertyString -Object $summary -Name "postgres_exec_container"
$beforePrimary = Get-JsonPropertyString -Object $summary -Name "before_primary"
$completedAt = Get-JsonPropertyString -Object $summary -Name "completed_at"

Assert-Condition ($runName.Length -gt 0) "postgres smoke summary run_name is required."
Assert-Condition ($gitCommit.Length -gt 0) "postgres smoke summary git_commit is required."
Assert-Condition ($pgDsn.Length -gt 0) "postgres smoke summary pg_dsn is required."
Assert-Condition ($postgresExecContainer.Length -gt 0) "postgres smoke summary postgres_exec_container is required."
Assert-Condition ($beforePrimary.Length -gt 0) "postgres smoke summary before_primary is required."
Assert-Condition ($completedAt.Length -gt 0) "postgres smoke summary completed_at is required."
if ($RequireCleanGit) {
    Assert-Condition (-not [bool]$summary.git_dirty) "postgres smoke summary git_dirty must be false when RequireCleanGit is set."
}

$scenario = ""
$validatedPushSummaries = @()
if ((Test-JsonPropertyExists -Object $summary -Name "after_primary") -or (Test-JsonPropertyExists -Object $summary -Name "stopped_container")) {
    $scenario = "postgres-failover"

    $stoppedContainer = Get-JsonPropertyString -Object $summary -Name "stopped_container"
    $afterPrimary = Get-JsonPropertyString -Object $summary -Name "after_primary"
    Assert-Condition ($stoppedContainer.Length -gt 0) "postgres failover summary stopped_container is required."
    Assert-Condition ($afterPrimary.Length -gt 0) "postgres failover summary after_primary is required."
    Assert-Condition ($afterPrimary -ne $beforePrimary) "postgres failover summary after_primary must differ from before_primary."

    $beforeSummaryPath = Resolve-ReferencedPath -BaseDir $summaryDir -PathValue (Get-JsonPropertyString -Object $summary -Name "before_summary") -FieldName "before_summary"
    $afterSummaryPath = Resolve-ReferencedPath -BaseDir $summaryDir -PathValue (Get-JsonPropertyString -Object $summary -Name "after_summary") -FieldName "after_summary"
    $validatedPushSummaries += Validate-PushGatewaySummary -Path $beforeSummaryPath -FieldName "before_summary"
    $validatedPushSummaries += Validate-PushGatewaySummary -Path $afterSummaryPath -FieldName "after_summary"
}
elseif ((Test-JsonPropertyExists -Object $summary -Name "stopped_standby_containers") -or (Test-JsonPropertyExists -Object $summary -Name "write_probe_with_only_primary")) {
    $scenario = "postgres-quorum-observation"

    Assert-Condition ((Get-JsonArrayCount -Object $summary -Name "before_pool_nodes") -gt 0) "postgres quorum observation before_pool_nodes is required."
    Assert-Condition ((Get-JsonArrayCount -Object $summary -Name "stopped_standby_containers") -ge 2) "postgres quorum observation must stop at least two standby containers."
    Assert-Condition ((Get-JsonArrayCount -Object $summary -Name "after_standby_stop_pool_nodes") -gt 0) "postgres quorum observation after_standby_stop_pool_nodes is required."
    Assert-Condition ((Get-JsonArrayCount -Object $summary -Name "after_restore_pool_nodes") -gt 0) "postgres quorum observation after_restore_pool_nodes is required."
    Assert-Condition (Test-JsonPropertyExists -Object $summary -Name "write_probe_with_only_primary") "postgres quorum observation write_probe_with_only_primary is required."

    $beforeSummaryPath = Resolve-ReferencedPath -BaseDir $summaryDir -PathValue (Get-JsonPropertyString -Object $summary -Name "before_summary") -FieldName "before_summary"
    $afterRestoreSummaryPath = Resolve-ReferencedPath -BaseDir $summaryDir -PathValue (Get-JsonPropertyString -Object $summary -Name "after_restore_summary") -FieldName "after_restore_summary"
    $validatedPushSummaries += Validate-PushGatewaySummary -Path $beforeSummaryPath -FieldName "before_summary"
    $validatedPushSummaries += Validate-PushGatewaySummary -Path $afterRestoreSummaryPath -FieldName "after_restore_summary"

    if ([bool]$summary.write_probe_with_only_primary) {
        $onlyPrimarySummaryPath = Resolve-ReferencedPath -BaseDir $summaryDir -PathValue (Get-JsonPropertyString -Object $summary -Name "only_primary_summary") -FieldName "only_primary_summary"
        $validatedPushSummaries += Validate-PushGatewaySummary -Path $onlyPrimarySummaryPath -FieldName "only_primary_summary"
    }
}
else {
    throw "Unable to infer PostgreSQL smoke summary scenario."
}

if ($ExpectedScenario.Trim().Length -gt 0) {
    Assert-Condition ($scenario -eq $ExpectedScenario.Trim()) "postgres smoke summary scenario $scenario did not match expected $ExpectedScenario."
}

$validation = [pscustomobject]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    summary_path = $resolvedSummaryPath
    run_name = $runName
    scenario = $scenario
    pg_dsn = $pgDsn
    postgres_exec_container = $postgresExecContainer
    before_primary = $beforePrimary
    after_primary = if ($scenario -eq "postgres-failover") { Get-JsonPropertyString -Object $summary -Name "after_primary" } else { "" }
    validated_smoke_summaries = @($validatedPushSummaries).Count
    valid = $true
    scope = "local PostgreSQL smoke summary validation; not a production PostgreSQL HA or quorum SLO"
}

if ($OutputPath.Trim().Length -gt 0) {
    $resolvedOutputPath = [System.IO.Path]::GetFullPath($OutputPath)
    $outputDir = Split-Path -Parent $resolvedOutputPath
    if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    $validation | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
    Write-Host "OK   postgres smoke summary validation written: $resolvedOutputPath"
}
else {
    $validation | ConvertTo-Json -Depth 5
}
