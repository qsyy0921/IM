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

function New-Step {
    param(
        [string]$Service,
        [string]$Runner,
        [string[]]$RunnerArgs
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
        command = "go"
        args = @($argumentList.ToArray())
        command_line = "go " + (Format-CommandLine -ArgumentList @($argumentList.ToArray()))
        result_dir = ""
        exit_code = $null
        status = "planned"
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
            $step = New-Step -Service $Service -Runner "demo" -RunnerArgs @($runnerArgsList.ToArray())
        }
        "identity-service" {
            $runnerArgsList.Add("--target")
            $runnerArgsList.Add($IdentityTarget)
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            $runnerArgsList.Add("--cleanup")
            Add-ArgIfValue -ArgumentList $runnerArgsList -Name "--pg-dsn" -Value $PGDSN
            $step = New-Step -Service $Service -Runner "identity" -RunnerArgs @($runnerArgsList.ToArray())
        }
        "message-service" {
            $runnerArgsList.Add("--target")
            $runnerArgsList.Add($MessageTarget)
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            $runnerArgsList.Add("--vus")
            $runnerArgsList.Add([string]$VUs)
            $runnerArgsList.Add("--duration")
            $runnerArgsList.Add($Duration)
            $runnerArgsList.Add("--conversation-count")
            $runnerArgsList.Add([string]$ConversationCount)
            Add-ArgIfValue -ArgumentList $runnerArgsList -Name "--pg-dsn" -Value $PGDSN
            $step = New-Step -Service $Service -Runner "sendmessage" -RunnerArgs @($runnerArgsList.ToArray())
        }
        "conversation-service" {
            $runnerArgsList.Add("--target")
            $runnerArgsList.Add($ConversationTarget)
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            $runnerArgsList.Add("--vus")
            $runnerArgsList.Add([string]$VUs)
            $runnerArgsList.Add("--duration")
            $runnerArgsList.Add($Duration)
            Add-ArgIfValue -ArgumentList $runnerArgsList -Name "--pg-dsn" -Value $PGDSN
            $step = New-Step -Service $Service -Runner "memberchange" -RunnerArgs @($runnerArgsList.ToArray())
        }
        "delivery-service" {
            $runnerArgsList.Add("--target")
            $runnerArgsList.Add($DeliveryTarget)
            $runnerArgsList.Add("--result-dir")
            $runnerArgsList.Add($resultDir)
            Add-ArgIfValue -ArgumentList $runnerArgsList -Name "--pg-dsn" -Value $PGDSN
            $step = New-Step -Service $Service -Runner "delivery" -RunnerArgs @($runnerArgsList.ToArray())
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
            $step = New-Step -Service $Service -Runner "pushgateway" -RunnerArgs @($runnerArgsList.ToArray())
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
            $step = New-Step -Service $Service -Runner "receipt" -RunnerArgs @($runnerArgsList.ToArray())
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
            $step = New-Step -Service $Service -Runner "contacts" -RunnerArgs @($runnerArgsList.ToArray())
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
        status = $Status
        service_count = $Steps.Count
        services = @($Steps | ForEach-Object { $_.service })
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
    $markdown += "- Status: $Status"
    $markdown += "- Scope: $Scope"
    $markdown += "- Suite root: $($summary.suite_root)"
    $markdown += ""
    $markdown += "| Service | Runner | Status | Command |"
    $markdown += "| --- | --- | --- | --- |"
    foreach ($step in $Steps) {
        $markdown += "| $($step.service) | $($step.runner) | $($step.status) | `$($step.command_line)` |"
    }
    $markdown += ""
    $markdown += "This suite only coordinates local loadtest runners and writes raw outputs under H drive by default. It is not a production SLO, HA proof, or sizing claim."

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
elseif ($suiteStatus -ne "failed") {
    $suiteStatus = "passed"
}

$paths = Write-SuiteSummary -SuiteRoot $suiteRoot -Steps @($steps.ToArray()) -DryRunValue ([bool]$DryRun) -Status $suiteStatus

if (-not $DryRun) {
    $baselinePath = Join-Path $suiteRoot "capacity-baseline-summary.json"
    $baselineMarkdownPath = Join-Path $suiteRoot "capacity-baseline-summary.md"
    & (Join-Path $PSScriptRoot "summarize-loadtest-capacity-baselines.ps1") `
        -ResultRoot $suiteRoot `
        -OutputPath $baselinePath `
        -MarkdownPath $baselineMarkdownPath `
        -ExpectedServices $canonicalServices `
        -RequireAllServices
}

Write-Host "OK   capacity baseline suite summary written: $($paths.summary_path)"
Write-Host "OK   capacity baseline suite report written: $($paths.markdown_path)"

if ($suiteStatus -eq "failed") {
    exit 1
}
