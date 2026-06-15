param(
    [int]$TimeoutSeconds = 90,
    [switch]$AllowImagePull,
    [switch]$KeepRunning,
    [switch]$IncludeAlertmanager,
    [switch]$RecordSummary,
    [string]$RunName = "",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$prometheusUpScript = Join-Path $PSScriptRoot "local-up-prometheus.ps1"
$grafanaUpScript = Join-Path $PSScriptRoot "local-up-grafana.ps1"
$alertmanagerUpScript = Join-Path $PSScriptRoot "local-up-alertmanager.ps1"
$prometheusCompose = Join-Path $repoRoot "deploy\local\docker-compose.prometheus.yml"
$grafanaCompose = Join-Path $repoRoot "deploy\local\docker-compose.grafana.yml"
$alertmanagerCompose = Join-Path $repoRoot "deploy\local\docker-compose.alertmanager.yml"
$summaryWriter = Join-Path $PSScriptRoot "write-observability-smoke-summary.ps1"

$expectedDashboardUids = @(
    "nexusim-api-gateway",
    "nexusim-contacts-service",
    "nexusim-conversation-service",
    "nexusim-delivery-service",
    "nexusim-identity-service",
    "nexusim-message-service",
    "nexusim-policy-service",
    "nexusim-push-gateway",
    "nexusim-receipt-service"
)

function Test-ContainerRunning {
    param([string]$ContainerName)

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $running = docker inspect -f "{{.State.Running}}" $ContainerName 2>$null
    $inspectExitCode = $LASTEXITCODE
    $ErrorActionPreference = $previousErrorActionPreference
    return ($inspectExitCode -eq 0 -and $running -eq "true")
}

function Invoke-Endpoint {
    param(
        [string]$Url,
        [hashtable]$Headers = @{}
    )

    return Invoke-RestMethod -Uri $Url -Headers $Headers -TimeoutSec 5
}

function Wait-HttpEndpoint {
    param(
        [string]$Name,
        [string]$Url,
        [hashtable]$Headers = @{}
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        try {
            [void](Invoke-Endpoint -Url $Url -Headers $Headers)
            return
        }
        catch {
            Start-Sleep -Seconds 2
        }
    } while ((Get-Date) -lt $deadline)

    throw "$Name did not become reachable before timeout: $Url"
}

function Get-BasicAuthHeader {
    $bytes = [System.Text.Encoding]::ASCII.GetBytes("admin:nexusim")
    return @{ Authorization = "Basic " + [Convert]::ToBase64String($bytes) }
}

function New-DefaultRunName {
    return "local-observability-smoke-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

Push-Location $repoRoot
try {
    $prometheusWasRunning = Test-ContainerRunning -ContainerName "nexusim-prometheus"
    $grafanaWasRunning = Test-ContainerRunning -ContainerName "nexusim-grafana"
    $alertmanagerWasRunning = Test-ContainerRunning -ContainerName "nexusim-alertmanager"

    $upArgs = @()
    if ($AllowImagePull) {
        $upArgs += "-AllowImagePull"
    }

    if ($IncludeAlertmanager) {
        & $alertmanagerUpScript @upArgs
        if ($LASTEXITCODE -ne 0) {
            throw "local-up-alertmanager.ps1 failed with exit code $LASTEXITCODE"
        }
        Wait-HttpEndpoint -Name "Alertmanager" -Url "http://127.0.0.1:19093/-/ready"
    }

    & $prometheusUpScript @upArgs
    if ($LASTEXITCODE -ne 0) {
        throw "local-up-prometheus.ps1 failed with exit code $LASTEXITCODE"
    }
    & $grafanaUpScript @upArgs
    if ($LASTEXITCODE -ne 0) {
        throw "local-up-grafana.ps1 failed with exit code $LASTEXITCODE"
    }

    Wait-HttpEndpoint -Name "Prometheus" -Url "http://127.0.0.1:19090/-/ready"
    $rules = Invoke-Endpoint -Url "http://127.0.0.1:19090/api/v1/rules"
    if ($rules.status -ne "success" -or -not $rules.data.groups -or $rules.data.groups.Count -lt 9) {
        throw "Prometheus did not load expected local rule groups."
    }
    $activeAlertmanagerUrls = @()
    if ($IncludeAlertmanager) {
        $alertmanagers = Invoke-Endpoint -Url "http://127.0.0.1:19090/api/v1/alertmanagers"
        if ($alertmanagers.status -ne "success" -or -not $alertmanagers.data.activeAlertmanagers) {
            throw "Prometheus did not discover the local Alertmanager target."
        }
        $activeAlertmanagerUrls = @($alertmanagers.data.activeAlertmanagers | ForEach-Object { [string]$_.url })
        if (-not (@($activeAlertmanagerUrls | Where-Object { $_ -match "host\.docker\.internal:19093|127\.0\.0\.1:19093|localhost:19093" }).Count -gt 0)) {
            throw "Prometheus active Alertmanager target did not include local port 19093."
        }
    }

    $grafanaHeaders = Get-BasicAuthHeader
    Wait-HttpEndpoint -Name "Grafana" -Url "http://127.0.0.1:13000/api/health" -Headers $grafanaHeaders
    $search = @(Invoke-Endpoint -Url "http://127.0.0.1:13000/api/search?type=dash-db" -Headers $grafanaHeaders)
    $foundUids = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($item in $search) {
        if ($item.uid) {
            [void]$foundUids.Add([string]$item.uid)
        }
    }

    foreach ($uid in $expectedDashboardUids) {
        if (-not $foundUids.Contains($uid)) {
            throw "Grafana did not provision expected dashboard uid: $uid"
        }
    }

    if ($RecordSummary) {
        if (-not (Test-Path -LiteralPath $summaryWriter -PathType Leaf)) {
            throw "Missing observability summary writer: $summaryWriter"
        }
        if ($RunName.Trim().Length -eq 0) {
            $RunName = New-DefaultRunName
        }

        $summaryDir = Join-Path $ResultRoot $RunName
        $foundDashboardUids = @($expectedDashboardUids | Where-Object { $foundUids.Contains($_) })
        & $summaryWriter `
            -OutputDir $summaryDir `
            -RunName $RunName `
            -RuleGroupCount ([int]$rules.data.groups.Count) `
            -ExpectedDashboardUids $expectedDashboardUids `
            -FoundDashboardUids $foundDashboardUids `
            -AlertmanagerChecked ([bool]$IncludeAlertmanager) `
            -ActiveAlertmanagerUrls $activeAlertmanagerUrls
        if ($LASTEXITCODE -ne 0) {
            throw "write-observability-smoke-summary.ps1 failed with exit code $LASTEXITCODE"
        }
        Write-Host "observability_summary_dir=$summaryDir"
    }

    Write-Host "OK   local observability smoke passed: Prometheus rules and Grafana dashboards loaded."
}
finally {
    if (-not $KeepRunning) {
        try {
            if (-not $grafanaWasRunning) {
                & docker compose -f $grafanaCompose down
            }
            if (-not $prometheusWasRunning) {
                & docker compose -f $prometheusCompose down
            }
            if ($IncludeAlertmanager -and -not $alertmanagerWasRunning) {
                & docker compose -f $alertmanagerCompose down
            }
        }
        catch {
            Write-Warning "local observability smoke cleanup failed: $($_.Exception.Message)"
        }
    }
    Pop-Location
}
