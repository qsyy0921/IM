param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string[]]$Actions = @("send"),
    [switch]$UsePolicyRules,
    [switch]$UseTenantPolicyRules,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "policy-message-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
$allowDir = Join-Path $resultDir "allow"
$denyDir = Join-Path $resultDir "deny"
$logDir = Join-Path $resultDir "logs"

New-Item -ItemType Directory -Force $allowDir | Out-Null
New-Item -ItemType Directory -Force $denyDir | Out-Null
New-Item -ItemType Directory -Force $logDir | Out-Null

. .\tools\go-env.ps1

if (-not $SkipBuild) {
    go build -o bin\policy-service.exe ./services/policy-service/cmd/policy-service
    go build -o bin\message-service.exe ./services/message-service/cmd/message-service
    go build -o bin\policy-message-loadtest.exe ./loadtest/policyintegration
}

function Apply-PostgresMigration {
    param(
        [string]$Path,
        [string]$Name
    )
    $resolved = Resolve-Path $Path
    $containerPath = "/tmp/$Name"
    docker cp $resolved "nexusim-postgres:$containerPath"
    docker exec nexusim-postgres psql `
        -U nexusim `
        -d nexusim `
        -v ON_ERROR_STOP=1 `
        -f $containerPath | Out-Null
}

function Get-FreeTcpPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return $listener.LocalEndpoint.Port
    } finally {
        $listener.Stop()
    }
}

function Wait-Tcp {
    param(
        [string]$HostName,
        [int]$Port,
        [int]$TimeoutSeconds = 20
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $client = [System.Net.Sockets.TcpClient]::new()
        try {
            $connect = $client.BeginConnect($HostName, $Port, $null, $null)
            if ($connect.AsyncWaitHandle.WaitOne(300)) {
                $client.EndConnect($connect)
                return
            }
        } catch {
        } finally {
            $client.Close()
        }
        Start-Sleep -Milliseconds 200
    }
    throw "Timed out waiting for ${HostName}:${Port}"
}

function Start-NexusProcess {
    param(
        [string]$Name,
        [string]$FilePath,
        [hashtable]$Env,
        [int]$Port = 0
    )
    foreach ($key in $Env.Keys) {
        [Environment]::SetEnvironmentVariable($key, [string]$Env[$key], "Process")
    }
    $out = Join-Path $logDir "$Name.out.log"
    $err = Join-Path $logDir "$Name.err.log"
    $proc = Start-Process -FilePath $FilePath `
        -WindowStyle Hidden `
        -PassThru `
        -RedirectStandardOutput $out `
        -RedirectStandardError $err
    if ($Port -gt 0) {
        Wait-Tcp -HostName "127.0.0.1" -Port $Port
    } else {
        Start-Sleep -Milliseconds 800
    }
    return $proc
}

function Stop-Processes {
    param([array]$Processes)
    foreach ($proc in $Processes) {
        if ($null -ne $proc -and -not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        }
    }
}

function Run-Scenario {
    param(
        [string]$Action,
        [string]$Scenario,
        [string]$ScenarioDir,
        [bool]$Allowed,
        [int64]$PermissionVersion,
        [string]$Classification,
        [string]$Reason
    )
    $policyPort = Get-FreeTcpPort
    $messagePort = Get-FreeTcpPort
    $policyAddr = "127.0.0.1:$policyPort"
    $messageAddr = "127.0.0.1:$messagePort"
    $processes = @()
    try {
        $usesRuleStore = $UsePolicyRules -or $UseTenantPolicyRules
        $policyStaticAllowed = if ($usesRuleStore) { -not $Allowed } else { $Allowed }
        $policyStaticClassification = if ($usesRuleStore) { "LOCAL_STATIC_SHOULD_NOT_APPEAR" } else { $Classification }
        $policyEnv = @{
            NEXUSIM_POLICY_SERVICE_MODE = "grpc"
            NEXUSIM_POLICY_GRPC_ADDR = $policyAddr
            NEXUSIM_POLICY_MESSAGE_ALLOWED = [string]$policyStaticAllowed
            NEXUSIM_POLICY_PERMISSION_VERSION = [string]$PermissionVersion
            NEXUSIM_POLICY_CLASSIFICATION = $policyStaticClassification
            NEXUSIM_POLICY_DENY_REASON = $Reason
        }
        if ($usesRuleStore) {
            $policyEnv["NEXUSIM_POLICY_RULES_ENABLED"] = "true"
            $policyEnv["NEXUSIM_PG_DSN"] = $PgDsn
        }
        $processes += Start-NexusProcess -Name "policy-$Scenario" -FilePath (Join-Path $repo "bin\policy-service.exe") -Port $policyPort -Env $policyEnv
        $processes += Start-NexusProcess -Name "message-$Scenario" -FilePath (Join-Path $repo "bin\message-service.exe") -Port $messagePort -Env @{
            NEXUSIM_MESSAGE_SERVICE_MODE = "grpc"
            NEXUSIM_GRPC_ADDR = $messageAddr
            NEXUSIM_PG_DSN = $PgDsn
            NEXUSIM_POLICY_SERVICE_ADDR = $policyAddr
            NEXUSIM_POLICY_RPC_TIMEOUT = "2s"
            NEXUSIM_MOCK_POLICY_ALLOWED = [string](-not $Allowed)
            NEXUSIM_MOCK_PERMISSION_VERSION = [string]$PermissionVersion
            NEXUSIM_MOCK_CLASSIFICATION = "LOCAL_STATIC_SHOULD_NOT_APPEAR"
        }
        $runner = Join-Path $repo "bin\policy-message-loadtest.exe"
        $tenant = "tenant-policy-message-$Scenario-" + (Get-Date -Format "yyyyMMddHHmmss")
        $args = @(
            "--target", $messageAddr,
            "--pg-dsn", $PgDsn,
            "--scenario", $Scenario,
            "--action", $Action,
            "--result-dir", $ScenarioDir,
            "--tenant-id", $tenant,
            "--conversation-id", "policy-message-$Action-$Scenario-conversation",
            "--client-msg-id", "policy-message-$Action-$Scenario-client-msg",
            "--expected-permission-version", $PermissionVersion,
            "--expected-classification", $Classification,
            "--cleanup"
        )
        if ($Reason -ne "") {
            $args += @("--expected-reason", $Reason)
        }
        if ($UsePolicyRules) {
            $args += "--seed-policy-rule"
        }
        if ($UseTenantPolicyRules) {
            $args += "--seed-tenant-policy-rule"
        }
        & $runner @args
        if ($LASTEXITCODE -ne 0) {
            throw "policy message smoke runner failed with exit code $LASTEXITCODE"
        }
        $summary = Get-Content -LiteralPath (Join-Path $ScenarioDir "policy-message-summary.json") -Raw | ConvertFrom-Json
        if (-not $summary.success) {
            throw "policy message smoke failed: $($summary.error)"
        }
        return $summary
    } finally {
        Stop-Processes $processes
    }
}

Get-ChildItem -Path "migrations\postgres\message" -Filter "*.sql" |
    Sort-Object Name |
    ForEach-Object {
        Apply-PostgresMigration -Path $_.FullName -Name $_.Name
    }

if ($UsePolicyRules -or $UseTenantPolicyRules) {
    Get-ChildItem -Path "migrations\postgres\policy" -Filter "*.sql" |
        Sort-Object Name |
        ForEach-Object {
            Apply-PostgresMigration -Path $_.FullName -Name $_.Name
        }
}

$scenarioSummaries = @()
foreach ($action in $Actions) {
    $normalizedAction = $action.Trim().ToLowerInvariant()
    if ($normalizedAction -eq "") {
        continue
    }
    $actionAllowDir = if ($Actions.Count -eq 1 -and $normalizedAction -eq "send") { $allowDir } else { Join-Path $resultDir "$normalizedAction-allow" }
    $actionDenyDir = if ($Actions.Count -eq 1 -and $normalizedAction -eq "send") { $denyDir } else { Join-Path $resultDir "$normalizedAction-deny" }
    New-Item -ItemType Directory -Force $actionAllowDir | Out-Null
    New-Item -ItemType Directory -Force $actionDenyDir | Out-Null

    $allowClassification = "POLICY_RPC_" + $normalizedAction.ToUpperInvariant() + "_ALLOWED"
    $denyClassification = "POLICY_RPC_" + $normalizedAction.ToUpperInvariant() + "_BLOCKED"
    $denyReason = "blocked by policy integration smoke $normalizedAction"

    $scenarioSummaries += Run-Scenario `
        -Action $normalizedAction `
        -Scenario "allow" `
        -ScenarioDir $actionAllowDir `
        -Allowed $true `
        -PermissionVersion 41 `
        -Classification $allowClassification `
        -Reason ""

    $scenarioSummaries += Run-Scenario `
        -Action $normalizedAction `
        -Scenario "deny" `
        -ScenarioDir $actionDenyDir `
        -Allowed $false `
        -PermissionVersion 42 `
        -Classification $denyClassification `
        -Reason $denyReason
}

$combinedFields = [ordered]@{
    success = $true
    result_dir = $resultDir
    policy_rules_enabled = [bool]$UsePolicyRules
    tenant_policy_rules_enabled = [bool]$UseTenantPolicyRules
    actions = $Actions
}
if ($scenarioSummaries.Count -eq 2 -and $scenarioSummaries[0].action -eq "send" -and $scenarioSummaries[1].action -eq "send") {
    $combinedFields["allow"] = $scenarioSummaries[0]
    $combinedFields["deny"] = $scenarioSummaries[1]
}
$combinedFields["scenarios"] = $scenarioSummaries
$combined = [pscustomobject]$combinedFields
$combined | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $resultDir "policy-message-smoke-summary.json") -Encoding UTF8

Write-Host "result_dir=$resultDir"
foreach ($summary in $scenarioSummaries) {
    $code = if ($summary.action -eq "send") { $summary.send_message.grpc_code } else { $summary.change_message.grpc_code }
    Write-Host "scenario=$($summary.action)/$($summary.scenario) grpc_code=$code"
}
