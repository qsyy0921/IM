param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$GatePolicyPath = "docs/runbook/ai-eval/gate-policy.local.json",
    [string[]]$OptionalAdapter = @(),
    [switch]$IncludeAllOptionalServiceStackAdapters,
    [string]$PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$MemoryTarget = "127.0.0.1:10580",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$RetrievalTarget = "127.0.0.1:10590",
    [string]$RAGTarget = "127.0.0.1:10610",
    [string]$SummaryTarget = "127.0.0.1:10620",
    [string]$AgentTarget = "127.0.0.1:10630",
    [string]$ActionExecutorTarget = "127.0.0.1:10660",
    [string]$WorkflowTarget = "127.0.0.1:10750",
    [string]$Python = "python",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$OutputPath = "",
    [string]$TenantID = "nexusim-local",
    [string]$UserID = "ai-eval-smoke",
    [string]$DeviceID = "ai-eval-smoke-device",
    [string]$RequestTimeout = "30s",
    [switch]$ExpectBusinessActionExecuted,
    [switch]$NoApplyMigration
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

function Resolve-RepoPath {
    param([string]$PathValue)

    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return [System.IO.Path]::GetFullPath($PathValue)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $repoRoot $PathValue))
}

function Get-JsonPropertyValue {
    param(
        $Object,
        [string]$Name,
        $DefaultValue = $null
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        return $DefaultValue
    }
    return $Object.$Name
}

function Get-JsonPropertyString {
    param(
        $Object,
        [string]$Name,
        [string]$DefaultValue = ""
    )

    $value = Get-JsonPropertyValue -Object $Object -Name $Name -DefaultValue $DefaultValue
    if ($null -eq $value) {
        return ""
    }
    return ([string]$value).Trim()
}

function Get-JsonPropertyBool {
    param(
        $Object,
        [string]$Name,
        [bool]$DefaultValue = $false
    )

    $value = Get-JsonPropertyValue -Object $Object -Name $Name -DefaultValue $DefaultValue
    if ($value -is [bool]) {
        return $value
    }
    if ($null -eq $value) {
        return $DefaultValue
    }
    return [System.Convert]::ToBoolean($value)
}

function ConvertTo-NameList {
    param([string[]]$Names)

    $result = New-Object System.Collections.Generic.List[string]
    foreach ($name in @($Names)) {
        foreach ($part in ([string]$name).Split(",")) {
            $trimmed = $part.Trim()
            if ($trimmed.Length -gt 0) {
                $result.Add($trimmed)
            }
        }
    }
    return $result.ToArray()
}

function Invoke-GateAdapter {
    param(
        [string]$AdapterName,
        [string]$ScriptPath,
        [string]$AdapterRunName,
        [string]$SummaryPath
    )

    switch ($AdapterName) {
        "memory-service" {
            & $ScriptPath `
                -CasePath $resolvedCasePath `
                -PGDSN $PGDSN `
                -MemoryTarget $MemoryTarget `
                -KafkaBrokers $KafkaBrokers `
                -ResultRoot $ResultRoot `
                -RunName $AdapterRunName `
                -OutputPath $SummaryPath `
                -RequestTimeout $RequestTimeout
        }
        "rag-service" {
            & $ScriptPath `
                -CasePath $resolvedCasePath `
                -PGDSN $PGDSN `
                -RAGTarget $RAGTarget `
                -ResultRoot $ResultRoot `
                -RunName $AdapterRunName `
                -OutputPath $SummaryPath `
                -RequestTimeout $RequestTimeout
        }
        "retrieval-gateway" {
            & $ScriptPath `
                -CasePath $resolvedCasePath `
                -PGDSN $PGDSN `
                -RetrievalTarget $RetrievalTarget `
                -ResultRoot $ResultRoot `
                -RunName $AdapterRunName `
                -OutputPath $SummaryPath `
                -RequestTimeout $RequestTimeout
        }
        "retrieval-gateway-negative" {
            & $ScriptPath `
                -CasePath $resolvedCasePath `
                -PGDSN $PGDSN `
                -RetrievalTarget $RetrievalTarget `
                -ResultRoot $ResultRoot `
                -RunName $AdapterRunName `
                -OutputPath $SummaryPath `
                -RequestTimeout $RequestTimeout
        }
        "summary-service" {
            & $ScriptPath `
                -CasePath $resolvedCasePath `
                -PGDSN $PGDSN `
                -SummaryTarget $SummaryTarget `
                -ResultRoot $ResultRoot `
                -RunName $AdapterRunName `
                -OutputPath $SummaryPath `
                -RequestTimeout $RequestTimeout
        }
        "agent-action-executor" {
            & $ScriptPath `
                -CasePath $resolvedCasePath `
                -PGDSN $PGDSN `
                -AgentTarget $AgentTarget `
                -ActionExecutorTarget $ActionExecutorTarget `
                -ResultRoot $ResultRoot `
                -RunName $AdapterRunName `
                -OutputPath $SummaryPath `
                -RequestTimeout $RequestTimeout
        }
        "rag-agent-demo" {
            $adapterArgs = @(
                "-CasePath", $resolvedCasePath,
                "-PGDSN", $PGDSN,
                "-RAGTarget", $RAGTarget,
                "-AgentTarget", $AgentTarget,
                "-ActionExecutorTarget", $ActionExecutorTarget,
                "-WorkflowTarget", $WorkflowTarget,
                "-ResultRoot", $ResultRoot,
                "-RunName", $AdapterRunName,
                "-OutputPath", $SummaryPath,
                "-RequestTimeout", $RequestTimeout
            )
            if ($ExpectBusinessActionExecuted) {
                $adapterArgs += "-ExpectBusinessActionExecuted"
            }
            & $ScriptPath @adapterArgs
        }
        "python-ai-worker" {
            & $ScriptPath `
                -Python $Python `
                -ResultRoot $ResultRoot `
                -RunName $AdapterRunName `
                -OutputPath $SummaryPath
        }
        "python-memory-extraction-candidate" {
            & $ScriptPath `
                -Python $Python `
                -ResultRoot $ResultRoot `
                -RunName $AdapterRunName `
                -OutputPath $SummaryPath
        }
        "agent-python-worker-provider" {
            & $ScriptPath `
                -CasePath $resolvedCasePath `
                -Python $Python `
                -ResultRoot $ResultRoot `
                -RunName $AdapterRunName `
                -OutputPath $SummaryPath
        }
        default {
            & $ScriptPath `
                -CasePath $resolvedCasePath `
                -ResultRoot $ResultRoot `
                -RunName $AdapterRunName `
                -OutputPath $SummaryPath
        }
    }
}

function Invoke-EvalRecorder {
    param(
        [string]$SummaryPath,
        [string]$RecordOutputPath
    )

    $applyMigrationValue = "true"
    if ($NoApplyMigration) {
        $applyMigrationValue = "false"
    }

    $goArgs = @(
        "run", "./services/ai-eval-service/cmd/ai-eval-record-smoke",
        "-summary", $SummaryPath,
        "-pg-dsn", $PGDSN,
        "-tenant-id", $TenantID,
        "-user-id", $UserID,
        "-device-id", $DeviceID,
        "-output", $RecordOutputPath,
        "-timeout", $RequestTimeout,
        "-apply-migration=$applyMigrationValue"
    )

    Push-Location $repoRoot
    try {
        & go @goArgs
        if ($LASTEXITCODE -ne 0) {
            throw "ai-eval RecordEvalRun recorder failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "ai-eval-regression-gate-smoke-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
Assert-ExternalOutputRoot -Value $resultDir -RepositoryRoot $repoRoot -Name "ResultDir"
if (-not (Test-Path -LiteralPath $resultDir)) {
    New-Item -ItemType Directory -Force -Path $resultDir | Out-Null
}

$resolvedCasePath = Resolve-RepoPath $CasePath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"

$resolvedGatePolicyPath = Resolve-RepoPath $GatePolicyPath
Assert-Condition (Test-Path -LiteralPath $resolvedGatePolicyPath -PathType Leaf) "GatePolicyPath does not exist: $resolvedGatePolicyPath"
$gatePolicy = Get-Content -LiteralPath $resolvedGatePolicyPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$gatePolicy.schema_version -eq 1) "unsupported gate policy schema_version"
Assert-Condition (@($gatePolicy.adapters).Count -gt 0) "gate policy has no adapters"

$requiredAdapters = @($gatePolicy.adapters | Where-Object { Get-JsonPropertyBool -Object $_ -Name "required" })
Assert-Condition ($requiredAdapters.Count -gt 0) "gate policy has no required adapters"

$optionalAdapters = @($gatePolicy.optional_service_stack_adapters)
$selectedOptionalAdapters = @()
$selectedOptionalNames = ConvertTo-NameList -Names $OptionalAdapter
if ($IncludeAllOptionalServiceStackAdapters) {
    $selectedOptionalAdapters = $optionalAdapters
} elseif ($selectedOptionalNames.Count -gt 0) {
    foreach ($selectedName in $selectedOptionalNames) {
        $match = @($optionalAdapters | Where-Object { (Get-JsonPropertyString -Object $_ -Name "name") -eq $selectedName })
        Assert-Condition ($match.Count -eq 1) "optional adapter is not declared in gate policy: $selectedName"
        $selectedOptionalAdapters += $match[0]
    }
}

$adapters = @($requiredAdapters + $selectedOptionalAdapters)

$adapterResults = New-Object System.Collections.Generic.List[object]
$totalCases = 0
$totalPassed = 0
$totalFailed = 0
$totalSkipped = 0

foreach ($adapter in $adapters) {
    $adapterName = Get-JsonPropertyString -Object $adapter -Name "name"
    $scriptName = Get-JsonPropertyString -Object $adapter -Name "script"
    $runSuffix = Get-JsonPropertyString -Object $adapter -Name "run_suffix" -DefaultValue $adapterName
    $summaryFile = Get-JsonPropertyString -Object $adapter -Name "summary_file" -DefaultValue "$adapterName-summary.json"
    Assert-Condition ($adapterName.Length -gt 0) "gate policy adapter name is required"
    Assert-Condition ($scriptName.Length -gt 0) "gate policy script is required for $adapterName"

    $adapterRunName = "$RunName-$runSuffix"
    $summaryPath = Join-Path $resultDir $summaryFile
    $recordPath = Join-Path $resultDir "$adapterName-record-summary.json"
    $scriptPath = Join-Path $PSScriptRoot $scriptName
    Assert-Condition (Test-Path -LiteralPath $scriptPath -PathType Leaf) "adapter script does not exist for $adapterName`: $scriptPath"

    Invoke-GateAdapter `
        -AdapterName $adapterName `
        -ScriptPath $scriptPath `
        -AdapterRunName $adapterRunName `
        -SummaryPath $summaryPath

    Assert-Condition (Test-Path -LiteralPath $summaryPath -PathType Leaf) "adapter summary missing: $summaryPath"
    Invoke-EvalRecorder -SummaryPath $summaryPath -RecordOutputPath $recordPath
    Assert-Condition (Test-Path -LiteralPath $recordPath -PathType Leaf) "record summary missing: $recordPath"

    $record = Get-Content -LiteralPath $recordPath -Raw | ConvertFrom-Json
    Assert-Condition ($record.status -eq "passed") "record smoke did not pass for $adapterName"
    Assert-Condition ($record.eval_run_status -eq "PASSED") "eval run status was not PASSED for $adapterName"
    if (Get-JsonPropertyBool -Object $gatePolicy.policy -Name "require_get_run" -DefaultValue $true) {
        Assert-Condition ([bool]$record.get_run_matched) "GetEvalRun did not match for $adapterName"
    }
    if (Get-JsonPropertyBool -Object $gatePolicy.policy -Name "require_list_run" -DefaultValue $true) {
        Assert-Condition ([bool]$record.list_contains_run) "ListEvalRuns did not contain run for $adapterName"
    }

    $totalCases += [int]$record.case_count
    $totalPassed += [int]$record.passed_count
    $totalFailed += [int]$record.failed_count
    $totalSkipped += [int]$record.skipped_count

    $adapterResults.Add([pscustomobject]@{
        name = $adapterName
        run_id = $record.run_id
        suite_id = $record.suite_id
        stage = $record.stage
        eval_run_status = $record.eval_run_status
        case_count = [int]$record.case_count
        passed_count = [int]$record.passed_count
        failed_count = [int]$record.failed_count
        skipped_count = [int]$record.skipped_count
        summary_ref = $record.summary_ref
        record_summary_path = $recordPath
    })
}

$gateStatus = "passed"
$maxFailedCount = [int](Get-JsonPropertyValue -Object $gatePolicy.policy -Name "max_failed_count" -DefaultValue 0)
$minCaseCount = [int](Get-JsonPropertyValue -Object $gatePolicy.policy -Name "min_case_count" -DefaultValue 1)
$requiredAdapterNames = @($gatePolicy.policy.required_adapters | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_.Length -gt 0 })
foreach ($requiredName in $requiredAdapterNames) {
    $found = $false
    foreach ($adapterResult in $adapterResults.ToArray()) {
        if ($adapterResult.name -eq $requiredName) {
            $found = $true
        }
    }
    if (-not $found) {
        throw "required adapter did not run: $requiredName"
    }
}
if ($totalFailed -gt $maxFailedCount -or $totalCases -lt $minCaseCount) {
    $gateStatus = "failed"
}

$gateSummary = [pscustomobject]@{
    schema_version = 1
    status = $gateStatus
    scope = "first-stage AI eval multi-adapter regression gate; required local adapters plus explicit optional adapters only"
    gate_id = Get-JsonPropertyString -Object $gatePolicy -Name "gate_id"
    gate_policy_ref = $resolvedGatePolicyPath
    run_name = $RunName
    result_dir = $resultDir
    adapter_count = $adapterResults.Count
    selected_optional_adapters = @($selectedOptionalAdapters | ForEach-Object { Get-JsonPropertyString -Object $_ -Name "name" })
    case_count = $totalCases
    passed_count = $totalPassed
    failed_count = $totalFailed
    skipped_count = $totalSkipped
    adapters = $adapterResults
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "ai-eval-regression-gate-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$gateSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8

if ($gateStatus -ne "passed") {
    throw "AI eval regression gate failed; summary: $resolvedOutputPath"
}

Write-Host "OK   ai-eval regression gate smoke completed: $resolvedOutputPath"
