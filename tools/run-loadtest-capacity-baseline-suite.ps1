param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = ("capacity-baseline-" + (Get-Date).ToString("yyyyMMdd-HHmmss")),
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
    [switch]$DryRun,
    [switch]$ContinueOnError,
    [switch]$IncludeSeededRunners,
    [switch]$IncludeStackRunners,
    [string]$PGDSN = $env:NEXUSIM_PG_DSN,
    [string]$KafkaBrokers = "localhost:9092",
    [int]$VUs = 2,
    [string]$Duration = "10s",
    [int]$ConversationCount = 10,
    [string]$ApiGatewayTarget = "127.0.0.1:12000",
    [string]$ConversationTarget = "127.0.0.1:10496",
    [string]$MessageTarget = "127.0.0.1:10495",
    [string]$DeliveryTarget = "127.0.0.1:10497",
    [string]$ReceiptTarget = "127.0.0.1:10499",
    [string]$PushURL = "ws://127.0.0.1:10498",
    [string]$ContactsTarget = "127.0.0.1:10500",
    [string]$IdentityTarget = "127.0.0.1:10600",
    [string]$PolicyTarget = "127.0.0.1:10800",
    [string]$Scope = "local 9-service capacity baseline suite; not a production SLO or sizing claim"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot

function Convert-ToServiceSet {
    param([string[]]$Values)

    $set = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($value in @($Values)) {
        foreach ($part in (([string]$value) -split "[,;]")) {
            $text = $part.Trim()
            if ($text.Length -gt 0) {
                [void]$set.Add($text)
            }
        }
    }
    return $set
}

function Add-ArgIfValue {
    param(
        [System.Collections.Generic.List[string]]$ArgumentList,
        [string]$Name,
        [string]$Value
    )

    if ([string]::IsNullOrWhiteSpace($Value)) {
        return
    }
    $ArgumentList.Add($Name)
    $ArgumentList.Add($Value)
}

function Format-CommandLine {
    param([string[]]$ArgumentList)

    $parts = New-Object System.Collections.Generic.List[string]
    foreach ($arg in $ArgumentList) {
        if ($arg -match "[\s`"]") {
            $escaped = $arg.Replace("`"", "\`"")
            $parts.Add('"' + $escaped + '"')
        }
        else {
            $parts.Add($arg)
        }
    }
    return ($parts -join " ")
}

function Get-PropertyValue {
    param(
        [object]$Object,
        [string]$Name
    )

    if ($null -eq $Object) {
        return $null
    }

    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) {
        return $null
    }

    return $property.Value
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

function Test-CapacityResult {
    param([string]$ResultDir)

    $summaryFiles = @(Get-ChildItem -LiteralPath $ResultDir -Recurse -File -Filter "*summary.json" -ErrorAction SilentlyContinue | Sort-Object FullName)
    foreach ($file in $summaryFiles) {
        $json = Get-Content -LiteralPath $file.FullName -Raw | ConvertFrom-Json
        $capacity = Get-PropertyValue -Object $json -Name "capacity_summary"
        if ($null -eq $capacity) {
            continue
        }

        $success = Get-PropertyValue -Object $json -Name "success"
        if ($success -is [bool] -and -not $success) {
            return [pscustomobject]@{
                ok = $false
                summary_path = $file.FullName
                reason = "summary success=false"
            }
        }

        foreach ($field in @("success_count", "logical_success_count", "allowed_action_count")) {
            $value = Convert-ToDoubleOrNull (Get-PropertyValue -Object $capacity -Name $field)
            if ($null -ne $value) {
                if ($value -gt 0) {
                    return [pscustomobject]@{
                        ok = $true
                        summary_path = $file.FullName
                        reason = "$field=$value"
                    }
                }
                return [pscustomobject]@{
                    ok = $false
                    summary_path = $file.FullName
                    reason = "$field is zero"
                }
            }
        }

        foreach ($field in @(
            "accepted_rps",
            "logical_accepted_rps",
            "operations_per_second",
            "messages_per_second",
            "notify_frames_per_second",
            "decisions_per_second",
            "ops_per_second",
            "events_per_second",
            "acks_per_second",
            "items_per_second"
        )) {
            $value = Convert-ToDoubleOrNull (Get-PropertyValue -Object $capacity -Name $field)
            if ($null -ne $value -and $value -gt 0) {
                return [pscustomobject]@{
                    ok = $true
                    summary_path = $file.FullName
                    reason = "$field=$value"
                }
            }
        }

        return [pscustomobject]@{
            ok = $false
            summary_path = $file.FullName
            reason = "capacity_summary has no positive success or throughput metric"
        }
    }

    return [pscustomobject]@{
        ok = $false
        summary_path = ""
        reason = "no capacity_summary file found"
    }
}

function New-Step {
    param(
        [string]$Service,
        [string]$Runner,
        [string[]]$RunnerArgs,
        [string]$BaselineMode = "direct",
        [bool]$RequiresSeed = $false,
        [bool]$RequiresRuntimeStack = $false,
        [string]$SkipReason = ""
    )

    $argumentList = New-Object System.Collections.Generic.List[string]
    $argumentList.Add("run")
    $argumentList.Add("./loadtest/$Runner")
    foreach ($arg in @($RunnerArgs)) {
        $argumentList.Add([string]$arg)
    }

    return [pscustomobject]@{
        service = $Service
        runner = $Runner
        baseline_mode = $BaselineMode
        requires_seed = $RequiresSeed
        requires_runtime_stack = $RequiresRuntimeStack
        skip_reason = $SkipReason
        command = "go"
        args = @($argumentList.ToArray())
        command_line = "go " + (Format-CommandLine -ArgumentList @($argumentList.ToArray()))
        result_dir = ""
        exit_code = $null
        status = "planned"
        capacity_summary_path = ""
        capacity_check_reason = ""
        started_at = ""
        finished_at = ""
        output_log = ""
    }
}

function Build-Step {
    param(
        [string]$Service,
        [string]$SuiteRoot
    )

    $resultDir = Join-Path $SuiteRoot $Service
    $runnerArgsList = New-Object System.Collections.Generic.List[string]

    switch ($Service) {
        "api-gateway" {
            $runnerArgsList.Add("--gateway-facade")
            $runnerArgsList.Add("--gateway-auth-mode")
            $runnerArgsList.Add("mock")
            $runnerArgsList.Add("--conversation-target")
            $runnerArgsList.Add($ApiGatewayTarget)
            $runnerArgsList.Add("--message-target")
            $runnerArgsList.Add($ApiGatewayTarget)
            $runnerArgsList.Add("--delivery-target")
            $runnerArgsList.Add($ApiGatewayTarget)
            $runnerArgsList.Add("--receipt-target")
            $runnerArgsList.Add($ApiGatewayTarget)
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            Add-ArgIfValue -ArgumentList $runnerArgsList -Name "--pg-dsn" -Value $PGDSN
            $step = New-Step `
                -Service $Service `
                -Runner "demo" `
                -RunnerArgs @($runnerArgsList.ToArray()) `
                -BaselineMode "requires_stack" `
                -RequiresRuntimeStack $true `
                -SkipReason "api-gateway demo validates an end-to-end stack and requires relay/consumer roles"
        }
        "identity-service" {
            $runnerArgsList.Add("--target")
            $runnerArgsList.Add($IdentityTarget)
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            $runnerArgsList.Add("--cleanup")
            Add-ArgIfValue -ArgumentList $runnerArgsList -Name "--pg-dsn" -Value $PGDSN
            $step = New-Step `
                -Service $Service `
                -Runner "identity" `
                -RunnerArgs @($runnerArgsList.ToArray()) `
                -BaselineMode "requires_stack" `
                -RequiresRuntimeStack $true `
                -SkipReason "identity loadtest exercises challenge delivery and requires webhook fixture or delivery worker setup"
        }
        "message-service" {
            $runnerArgsList.Add("--target")
            $runnerArgsList.Add($MessageTarget)
            $runnerArgsList.Add("--tenant-id")
            $runnerArgsList.Add("tenant-capacity-message")
            $runnerArgsList.Add("--conversation-prefix")
            $runnerArgsList.Add("conv-capacity-message")
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            $runnerArgsList.Add("--vus")
            $runnerArgsList.Add([string]$VUs)
            $runnerArgsList.Add("--duration")
            $runnerArgsList.Add($Duration)
            $runnerArgsList.Add("--conversation-count")
            $runnerArgsList.Add([string]$ConversationCount)
            Add-ArgIfValue -ArgumentList $runnerArgsList -Name "--pg-dsn" -Value $PGDSN
            $step = New-Step `
                -Service $Service `
                -Runner "sendmessage" `
                -RunnerArgs @($runnerArgsList.ToArray()) `
                -BaselineMode "requires_seed" `
                -RequiresSeed $true `
                -SkipReason "message loadtest requires pre-seeded ACTIVE conversations and members"
        }
        "conversation-service" {
            $runnerArgsList.Add("--target")
            $runnerArgsList.Add($ConversationTarget)
            $runnerArgsList.Add("--tenant-id")
            $runnerArgsList.Add("tenant-capacity-conversation")
            $runnerArgsList.Add("--conversation-id")
            $runnerArgsList.Add("conv-capacity-memberchange")
            $runnerArgsList.Add("--operator-user-id")
            $runnerArgsList.Add("owner-1")
            $runnerArgsList.Add("--target-prefix")
            $runnerArgsList.Add("target-capacity-$RunName")
            $runnerArgsList.Add("--idempotency-prefix")
            $runnerArgsList.Add("capacity-$RunName")
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            $runnerArgsList.Add("--vus")
            $runnerArgsList.Add([string]$VUs)
            $runnerArgsList.Add("--duration")
            $runnerArgsList.Add($Duration)
            Add-ArgIfValue -ArgumentList $runnerArgsList -Name "--pg-dsn" -Value $PGDSN
            $step = New-Step `
                -Service $Service `
                -Runner "memberchange" `
                -RunnerArgs @($runnerArgsList.ToArray()) `
                -BaselineMode "requires_seed" `
                -RequiresSeed $true `
                -SkipReason "conversation memberchange loadtest requires a pre-seeded conversation with an ACTIVE owner"
        }
        "delivery-service" {
            $runnerArgsList.Add("--target")
            $runnerArgsList.Add($DeliveryTarget)
            $runnerArgsList.Add("--tenant-id")
            $runnerArgsList.Add("tenant-capacity-delivery")
            $runnerArgsList.Add("--conversation-id")
            $runnerArgsList.Add("conv-capacity-delivery")
            $runnerArgsList.Add("--user-id")
            $runnerArgsList.Add("delivery-user-1")
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            Add-ArgIfValue -ArgumentList $runnerArgsList -Name "--pg-dsn" -Value $PGDSN
            $step = New-Step `
                -Service $Service `
                -Runner "delivery" `
                -RunnerArgs @($runnerArgsList.ToArray()) `
                -BaselineMode "requires_seed" `
                -RequiresSeed $true `
                -SkipReason "delivery loadtest validates existing PullInbox state and requires seeded inbox data"
        }
        "push-gateway" {
            $runnerArgsList.Add("--conversation-target")
            $runnerArgsList.Add($ConversationTarget)
            $runnerArgsList.Add("--message-target")
            $runnerArgsList.Add($MessageTarget)
            $runnerArgsList.Add("--delivery-target")
            $runnerArgsList.Add($DeliveryTarget)
            $runnerArgsList.Add("--push-url")
            $runnerArgsList.Add($PushURL)
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            $runnerArgsList.Add("--scenario")
            $runnerArgsList.Add("full")
            Add-ArgIfValue -ArgumentList $runnerArgsList -Name "--pg-dsn" -Value $PGDSN
            $step = New-Step `
                -Service $Service `
                -Runner "pushgateway" `
                -RunnerArgs @($runnerArgsList.ToArray()) `
                -BaselineMode "requires_stack" `
                -RequiresRuntimeStack $true `
                -SkipReason "push-gateway full scenario requires delivery timeline/outbox relay and push delivery-consumer roles"
        }
        "receipt-service" {
            $runnerArgsList.Add("--conversation-target")
            $runnerArgsList.Add($ConversationTarget)
            $runnerArgsList.Add("--message-target")
            $runnerArgsList.Add($MessageTarget)
            $runnerArgsList.Add("--delivery-target")
            $runnerArgsList.Add($DeliveryTarget)
            $runnerArgsList.Add("--receipt-target")
            $runnerArgsList.Add($ReceiptTarget)
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            Add-ArgIfValue -ArgumentList $runnerArgsList -Name "--pg-dsn" -Value $PGDSN
            $step = New-Step `
                -Service $Service `
                -Runner "receipt" `
                -RunnerArgs @($runnerArgsList.ToArray()) `
                -BaselineMode "requires_stack" `
                -RequiresRuntimeStack $true `
                -SkipReason "receipt loadtest requires message/delivery/receipt relay and consumer roles"
        }
        "contacts-service" {
            $runnerArgsList.Add("--target")
            $runnerArgsList.Add($ContactsTarget)
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            $runnerArgsList.Add("--kafka-brokers")
            $runnerArgsList.Add($KafkaBrokers)
            $runnerArgsList.Add("--cleanup")
            Add-ArgIfValue -ArgumentList $runnerArgsList -Name "--pg-dsn" -Value $PGDSN
            $step = New-Step `
                -Service $Service `
                -Runner "contacts" `
                -RunnerArgs @($runnerArgsList.ToArray()) `
                -BaselineMode "requires_stack" `
                -RequiresRuntimeStack $true `
                -SkipReason "contacts loadtest validates Kafka contact events and requires contacts outbox-relay"
        }
        "policy-service" {
            $runnerArgsList.Add("--target")
            $runnerArgsList.Add($PolicyTarget)
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            $step = New-Step -Service $Service -Runner "policy" -RunnerArgs @($runnerArgsList.ToArray())
        }
        default {
            throw "Unknown service for capacity baseline suite: $Service"
        }
    }

    $step.result_dir = $resultDir
    $step.output_log = Join-Path $resultDir "capacity-baseline.out.log"
    return $step
}

function Write-SuiteSummary {
    param(
        [string]$SuiteRoot,
        [object[]]$Steps,
        [bool]$DryRunValue,
        [string]$Status
    )

    $summary = [pscustomobject]@{
        run_name = $RunName
        created_at = (Get-Date).ToUniversalTime().ToString("o")
        result_root = ([System.IO.Path]::GetFullPath($ResultRoot))
        suite_root = ([System.IO.Path]::GetFullPath($SuiteRoot))
        scope = $Scope
        dry_run = $DryRunValue
        include_seeded_runners = [bool]$IncludeSeededRunners
        include_stack_runners = [bool]$IncludeStackRunners
        status = $Status
        service_count = $Steps.Count
        runnable_service_count = @($Steps | Where-Object { $_.status -notlike "skipped_*" }).Count
        skipped_service_count = @($Steps | Where-Object { $_.status -like "skipped_*" }).Count
        services = @($Steps | ForEach-Object { $_.service })
        skipped_services = @($Steps | Where-Object { $_.status -like "skipped_*" } | ForEach-Object { $_.service })
        steps = @($Steps)
    }

    $summaryPath = Join-Path $SuiteRoot "capacity-baseline-suite-summary.json"
    $summary | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $summaryPath -Encoding UTF8

    $markdown = @()
    $markdown += "# Loadtest Capacity Baseline Suite"
    $markdown += ""
    $markdown += "- Run: $RunName"
    $markdown += "- Created at: $($summary.created_at)"
    $markdown += "- Dry run: $DryRunValue"
    $markdown += "- Include seeded runners: $([bool]$IncludeSeededRunners)"
    $markdown += "- Include stack runners: $([bool]$IncludeStackRunners)"
    $markdown += "- Status: $Status"
    $markdown += "- Scope: $Scope"
    $markdown += "- Suite root: $($summary.suite_root)"
    $markdown += "- Runnable services: $($summary.runnable_service_count)/$($summary.service_count)"
    if ($summary.skipped_service_count -gt 0) {
        $markdown += "- Skipped services: $($summary.skipped_services -join ', ')"
    }
    $markdown += ""
    $markdown += "| Service | Runner | Baseline mode | Status | Skip reason | Command |"
    $markdown += "| --- | --- | --- | --- | --- | --- |"
    $tick = [char]96
    foreach ($step in $Steps) {
        $commandText = [string]$step.command_line
        $markdown += "| $($step.service) | $($step.runner) | $($step.baseline_mode) | $($step.status) | $($step.skip_reason) | $tick$commandText$tick |"
    }
    $markdown += ""
    $markdown += "This suite only coordinates local loadtest runners and writes raw outputs under H drive by default. Seeded runners and stack runners are skipped unless explicitly enabled because they need pre-populated state or extra relay/consumer roles. This is not a production SLO, HA proof, or sizing claim."

    $markdownPath = Join-Path $SuiteRoot "capacity-baseline-suite-summary.md"
    $markdown | Set-Content -LiteralPath $markdownPath -Encoding UTF8

    return [pscustomobject]@{
        summary_path = $summaryPath
        markdown_path = $markdownPath
    }
}

$selected = Convert-ToServiceSet -Values $Services
if ($selected.Count -eq 0) {
    throw "At least one service is required."
}

$suiteRoot = Join-Path $ResultRoot $RunName
New-Item -ItemType Directory -Force -Path $suiteRoot | Out-Null

$canonicalServices = @(
    "api-gateway",
    "identity-service",
    "message-service",
    "conversation-service",
    "delivery-service",
    "push-gateway",
    "receipt-service",
    "contacts-service",
    "policy-service"
)

$steps = New-Object System.Collections.Generic.List[object]
foreach ($service in $canonicalServices) {
    if (-not $selected.Contains($service)) {
        continue
    }
    $steps.Add((Build-Step -Service $service -SuiteRoot $suiteRoot))
}

if ($steps.Count -eq 0) {
    throw "No known services selected."
}

if (-not $DryRun) {
    $goEnv = Join-Path $repoRoot "tools\go-env.ps1"
    if (Test-Path -LiteralPath $goEnv -PathType Leaf) {
        . $goEnv
    }
}

$suiteStatus = "planned"
foreach ($step in $steps) {
    New-Item -ItemType Directory -Force -Path $step.result_dir | Out-Null
    if ($step.requires_seed -and -not $IncludeSeededRunners) {
        $step.status = "skipped_seed_required"
        continue
    }
    if ($step.requires_runtime_stack -and -not $IncludeStackRunners) {
        $step.status = "skipped_stack_required"
        continue
    }
    if ($DryRun) {
        $step.status = "dry_run"
        continue
    }

    $step.status = "running"
    $step.started_at = (Get-Date).ToUniversalTime().ToString("o")
    Push-Location $repoRoot
    try {
        $output = & $step.command @($step.args) 2>&1
        $exitCode = $LASTEXITCODE
        $step.exit_code = $exitCode
        $output | Set-Content -LiteralPath $step.output_log -Encoding UTF8
        $step.finished_at = (Get-Date).ToUniversalTime().ToString("o")
        if ($exitCode -eq 0) {
            $capacityCheck = Test-CapacityResult -ResultDir $step.result_dir
            $step.capacity_summary_path = $capacityCheck.summary_path
            $step.capacity_check_reason = $capacityCheck.reason
            if (-not $capacityCheck.ok) {
                $step.exit_code = 1
                $step.status = "failed"
                $suiteStatus = "failed"
                Add-Content -LiteralPath $step.output_log -Encoding UTF8 -Value "capacity summary check failed: $($capacityCheck.reason)"
                if (-not $ContinueOnError) {
                    break
                }
                continue
            }
            $step.status = "passed"
        }
        else {
            $step.status = "failed"
            $suiteStatus = "failed"
            if (-not $ContinueOnError) {
                break
            }
        }
    }
    finally {
        Pop-Location
    }
}

if ($DryRun) {
    $suiteStatus = "dry_run"
}
elseif (@($steps | Where-Object { $_.status -notlike "skipped_*" }).Count -eq 0) {
    $suiteStatus = "skipped"
}
elseif ($suiteStatus -ne "failed") {
    $suiteStatus = "passed"
}

$paths = Write-SuiteSummary -SuiteRoot $suiteRoot -Steps @($steps.ToArray()) -DryRunValue ([bool]$DryRun) -Status $suiteStatus

if (-not $DryRun) {
    $expectedSummaryServices = @(
        $steps |
            Where-Object { $_.status -notlike "skipped_*" } |
            ForEach-Object { $_.service }
    )
    $baselinePath = Join-Path $suiteRoot "capacity-baseline-summary.json"
    $baselineMarkdownPath = Join-Path $suiteRoot "capacity-baseline-summary.md"
    if ($expectedSummaryServices.Count -gt 0 -and $suiteStatus -ne "failed") {
        & (Join-Path $PSScriptRoot "summarize-loadtest-capacity-baselines.ps1") `
            -ResultRoot $suiteRoot `
            -OutputPath $baselinePath `
            -MarkdownPath $baselineMarkdownPath `
            -ExpectedServices $expectedSummaryServices `
            -RequireAllServices
    }
}

Write-Host "OK   capacity baseline suite summary written: $($paths.summary_path)"
Write-Host "OK   capacity baseline suite report written: $($paths.markdown_path)"

if ($suiteStatus -eq "failed") {
    exit 1
}
