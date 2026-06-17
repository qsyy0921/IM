function Resolve-PushGatewayRedisFaultSetup {
    param(
        [string]$Scenario,
        [string]$RouteBackend,
        [string]$RedisMode,
        [string]$RedisFaultCommand,
        [string]$RedisRestoreCommand,
        [string]$ResultDir
    )

    if ($Scenario -eq "redis-fault" -and -not $RedisFaultCommand) {
        $RedisFaultCommand = "docker stop nexusim-redis | Out-Null"
    }
    if ($Scenario -eq "redis-fault" -and -not $RedisRestoreCommand) {
        $RedisRestoreCommand = "docker start nexusim-redis | Out-Null"
    }
    if ($Scenario -eq "redis-cluster-node-stop") {
        if ($RouteBackend -ne "redis") {
            throw "redis-cluster-node-stop requires -RouteBackend redis"
        }
        if ($RedisMode -ne "cluster") {
            throw "redis-cluster-node-stop requires -RedisMode cluster"
        }
        if (-not $RedisFaultCommand) {
            throw "redis-cluster-node-stop requires -RedisFaultCommand"
        }
        if (-not $RedisRestoreCommand) {
            throw "redis-cluster-node-stop requires -RedisRestoreCommand"
        }
    }
    if ($Scenario -eq "redis-cluster-failover") {
        if ($RouteBackend -ne "redis") {
            throw "redis-cluster-failover requires -RouteBackend redis"
        }
        if ($RedisMode -ne "cluster") {
            throw "redis-cluster-failover requires -RedisMode cluster"
        }
        if (-not $RedisFaultCommand) {
            throw "redis-cluster-failover requires -RedisFaultCommand"
        }
    }
    $runnerRequestTimeout = "3s"
    if ($Scenario -eq "redis-sentinel-failover") {
        $runnerRequestTimeout = "60s"
    }
    if ($Scenario -eq "redis-sentinel-master-stop") {
        $runnerRequestTimeout = "90s"
    }
    if ($Scenario -eq "redis-sentinel-quorum-loss") {
        $runnerRequestTimeout = "90s"
    }
    if ($Scenario -eq "redis-sentinel-network-partition") {
        $runnerRequestTimeout = "90s"
    }
    if ($Scenario -eq "redis-cluster-node-stop") {
        $runnerRequestTimeout = "30s"
    }
    if ($Scenario -eq "redis-cluster-failover") {
        $runnerRequestTimeout = "90s"
    }

    if ($Scenario -eq "redis-sentinel-failover") {
        if ($RouteBackend -ne "redis") {
            throw "redis-sentinel-failover requires -RouteBackend redis"
        }
        if ($RedisMode -ne "sentinel") {
            throw "redis-sentinel-failover requires -RedisMode sentinel"
        }
        if (-not $RedisFaultCommand) {
            $failoverScript = Join-Path $ResultDir "redis-sentinel-failover.ps1"
            @'
$ErrorActionPreference = "Stop"
$before = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster)
if ($before.Count -lt 2) {
    throw "Sentinel did not return a current master before failover."
}
$beforeHost = $before[0].Trim()
$beforePort = $before[1].Trim()
$beforeAddr = "${beforeHost}:${beforePort}"
Write-Output "sentinel_master_before=$beforeAddr"
docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL failover mymaster | Out-Null
$deadline = (Get-Date).AddSeconds(45)
$afterAddr = ""
$afterHost = ""
$afterPort = ""
do {
    Start-Sleep -Seconds 1
    $after = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster)
    if ($after.Count -ge 2) {
        $afterHost = $after[0].Trim()
        $afterPort = $after[1].Trim()
        $afterAddr = "${afterHost}:${afterPort}"
    }
} while (($afterAddr -eq "" -or $afterAddr -eq $beforeAddr) -and (Get-Date) -lt $deadline)
if ($afterAddr -eq "" -or $afterAddr -eq $beforeAddr) {
    throw "Sentinel failover did not change master before timeout; before=$beforeAddr after=$afterAddr"
}
$ping = @(docker exec nexusim-redis-sentinel-1 redis-cli -h $afterHost -p $afterPort ping)
if ($ping.Count -lt 1 -or $ping[0].Trim() -ne "PONG") {
    throw "New Sentinel master did not respond to PING: $($ping -join ',')"
}
$role = @(docker exec nexusim-redis-sentinel-1 redis-cli -h $afterHost -p $afterPort role)
if ($role.Count -lt 1 -or $role[0].Trim() -ne "master") {
    throw "New Sentinel master role is not master: $($role -join ',')"
}
Write-Output "sentinel_master_after=$afterAddr"
'@ | Set-Content -LiteralPath $failoverScript -Encoding UTF8
            $RedisFaultCommand = "& '$failoverScript'"
        }
    }
    if ($Scenario -eq "redis-sentinel-quorum-loss") {
        if ($RouteBackend -ne "redis") {
            throw "redis-sentinel-quorum-loss requires -RouteBackend redis"
        }
        if ($RedisMode -ne "sentinel") {
            throw "redis-sentinel-quorum-loss requires -RedisMode sentinel"
        }
        if (-not $RedisFaultCommand) {
            $faultScript = Join-Path $ResultDir "redis-sentinel-quorum-loss.ps1"
            @'
$ErrorActionPreference = "Stop"
$portToContainer = @{
    "6380" = "nexusim-redis-ha-master"
    "6381" = "nexusim-redis-ha-replica-1"
    "6382" = "nexusim-redis-ha-replica-2"
}
$before = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster)
if ($before.Count -lt 2) {
    throw "Sentinel did not return a current master before quorum-loss fault."
}
$beforeHost = $before[0].Trim()
$beforePort = $before[1].Trim()
$beforeAddr = "${beforeHost}:${beforePort}"
if (-not $portToContainer.ContainsKey($beforePort)) {
    throw "No local Redis container mapping for Sentinel master port $beforePort"
}
$masterContainer = $portToContainer[$beforePort]
$stoppedSentinels = @("nexusim-redis-sentinel-2", "nexusim-redis-sentinel-3")
$allStopped = @($masterContainer) + $stoppedSentinels
Set-Content -LiteralPath (Join-Path $PSScriptRoot "redis-sentinel-quorum-loss-stopped.txt") -Value ($allStopped -join "`n") -Encoding ASCII
Write-Output "sentinel_master_before=$beforeAddr"
Write-Output "stopped_master_container=$masterContainer"
Write-Output "stopped_sentinels=$($stoppedSentinels -join ',')"
foreach ($container in $stoppedSentinels) {
    docker stop $container | Out-Null
}
docker stop $masterContainer | Out-Null
$post = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster 2>$null)
if ($post.Count -ge 2) {
    Write-Output ("sentinel_master_after_fault=" + $post[0].Trim() + ":" + $post[1].Trim())
} else {
    Write-Output "sentinel_master_after_fault=unavailable"
}
'@ | Set-Content -LiteralPath $faultScript -Encoding UTF8
            $RedisFaultCommand = "& '$faultScript'"
        }
        if (-not $RedisRestoreCommand) {
            $restoreScript = Join-Path $ResultDir "redis-sentinel-quorum-restore.ps1"
            @'
$ErrorActionPreference = "Stop"
$stoppedFile = Join-Path $PSScriptRoot "redis-sentinel-quorum-loss-stopped.txt"
if (-not (Test-Path -LiteralPath $stoppedFile)) {
    Write-Output "sentinel_restore=skipped_no_stopped_file"
    return
}
$containers = @(
    Get-Content -LiteralPath $stoppedFile |
        ForEach-Object { $_.Trim() } |
        Where-Object { $_ -ne "" }
)
if ($containers.Count -eq 0) {
    Write-Output "sentinel_restore=skipped_empty_stopped_list"
    return
}
foreach ($container in $containers) {
    docker start $container | Out-Null
}
$deadline = (Get-Date).AddSeconds(90)
$ready = $false
do {
    Start-Sleep -Seconds 2
    $sentinelState = docker inspect -f "{{.State.Health.Status}}" nexusim-redis-sentinel-1 2>$null
    $sentinelMaster = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster 2>$null)
    $ready = $sentinelState -eq "healthy" -and $sentinelMaster.Count -ge 2
} while (-not $ready -and (Get-Date) -lt $deadline)
if (-not $ready) {
    throw "Redis Sentinel quorum restore did not recover before timeout."
}
$masterHost = $sentinelMaster[0].Trim()
$masterPort = $sentinelMaster[1].Trim()
$ping = @(docker exec nexusim-redis-sentinel-1 redis-cli -h $masterHost -p $masterPort ping)
if ($ping.Count -lt 1 -or $ping[0].Trim() -ne "PONG") {
    throw "Recovered Sentinel master did not respond to PING: $($ping -join ',')"
}
Write-Output "sentinel_restored_containers=$($containers -join ',')"
Write-Output "sentinel_master_after_restore=${masterHost}:${masterPort}"
'@ | Set-Content -LiteralPath $restoreScript -Encoding UTF8
            $RedisRestoreCommand = "& '$restoreScript'"
        }
    }
    if ($Scenario -eq "redis-sentinel-network-partition") {
        if ($RouteBackend -ne "redis") {
            throw "redis-sentinel-network-partition requires -RouteBackend redis"
        }
        if ($RedisMode -ne "sentinel") {
            throw "redis-sentinel-network-partition requires -RedisMode sentinel"
        }
        if (-not $RedisFaultCommand) {
            $faultScript = Join-Path $ResultDir "redis-sentinel-network-partition.ps1"
            @'
$ErrorActionPreference = "Stop"
$portToContainer = @{
    "6380" = "nexusim-redis-ha-master"
    "6381" = "nexusim-redis-ha-replica-1"
    "6382" = "nexusim-redis-ha-replica-2"
}
$before = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster)
if ($before.Count -lt 2) {
    throw "Sentinel did not return a current master before network partition fault."
}
$beforeHost = $before[0].Trim()
$beforePort = $before[1].Trim()
$beforeAddr = "${beforeHost}:${beforePort}"
if (-not $portToContainer.ContainsKey($beforePort)) {
    throw "No local Redis container mapping for Sentinel master port $beforePort"
}
$masterContainer = $portToContainer[$beforePort]
$networks = @(
    docker inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' $masterContainer 2>$null |
        ForEach-Object { $_.Trim() } |
        Where-Object { $_ -ne "" }
)
if ($networks.Count -eq 0) {
    throw "Redis master container $masterContainer has no Docker network to partition."
}
$network = $networks[0]
$stateFile = Join-Path $PSScriptRoot "redis-sentinel-network-partition-state.txt"
Set-Content -LiteralPath $stateFile -Value @($masterContainer, $network) -Encoding ASCII
Write-Output "sentinel_master_before=$beforeAddr"
Write-Output "partitioned_container=$masterContainer"
Write-Output "partitioned_network=$network"
docker network disconnect $network $masterContainer | Out-Null
Start-Sleep -Seconds 8
$post = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster 2>$null)
if ($post.Count -ge 2) {
    Write-Output ("sentinel_master_after_partition=" + $post[0].Trim() + ":" + $post[1].Trim())
} else {
    Write-Output "sentinel_master_after_partition=unavailable"
}
'@ | Set-Content -LiteralPath $faultScript -Encoding UTF8
            $RedisFaultCommand = "& '$faultScript'"
        }
        if (-not $RedisRestoreCommand) {
            $restoreScript = Join-Path $ResultDir "redis-sentinel-network-restore.ps1"
            @'
$ErrorActionPreference = "Stop"
$stateFile = Join-Path $PSScriptRoot "redis-sentinel-network-partition-state.txt"
if (-not (Test-Path -LiteralPath $stateFile)) {
    Write-Output "sentinel_network_restore=skipped_no_state_file"
    return
}
$state = @(
    Get-Content -LiteralPath $stateFile |
        ForEach-Object { $_.Trim() } |
        Where-Object { $_ -ne "" }
)
if ($state.Count -lt 2) {
    Write-Output "sentinel_network_restore=skipped_invalid_state"
    return
}
$container = $state[0]
$network = $state[1]
$containerToAlias = @{
    "nexusim-redis-ha-master" = "redis-ha-master"
    "nexusim-redis-ha-replica-1" = "redis-ha-replica-1"
    "nexusim-redis-ha-replica-2" = "redis-ha-replica-2"
}
$attachedNetworks = @(
    docker inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' $container 2>$null |
        ForEach-Object { $_.Trim() } |
        Where-Object { $_ -ne "" }
)
if ($attachedNetworks -notcontains $network) {
    if ($containerToAlias.ContainsKey($container)) {
        $alias = $containerToAlias[$container]
        docker network connect --alias $alias $network $container | Out-Null
    } else {
        docker network connect $network $container | Out-Null
    }
}
$deadline = (Get-Date).AddSeconds(90)
$ready = $false
do {
    Start-Sleep -Seconds 2
    $sentinelState = docker inspect -f "{{.State.Health.Status}}" nexusim-redis-sentinel-1 2>$null
    $sentinelMaster = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster 2>$null)
    $ready = $sentinelState -eq "healthy" -and $sentinelMaster.Count -ge 2
} while (-not $ready -and (Get-Date) -lt $deadline)
if (-not $ready) {
    throw "Redis Sentinel network partition restore did not recover before timeout."
}
$masterHost = $sentinelMaster[0].Trim()
$masterPort = $sentinelMaster[1].Trim()
$ping = @(docker exec nexusim-redis-sentinel-1 redis-cli -h $masterHost -p $masterPort ping)
if ($ping.Count -lt 1 -or $ping[0].Trim() -ne "PONG") {
    throw "Recovered Sentinel master did not respond to PING: $($ping -join ',')"
}
Write-Output "sentinel_network_restored_container=$container"
Write-Output "sentinel_network_restored_network=$network"
Write-Output "sentinel_master_after_network_restore=${masterHost}:${masterPort}"
'@ | Set-Content -LiteralPath $restoreScript -Encoding UTF8
            $RedisRestoreCommand = "& '$restoreScript'"
        }
    }
    if ($Scenario -eq "redis-sentinel-master-stop") {
        if ($RouteBackend -ne "redis") {
            throw "redis-sentinel-master-stop requires -RouteBackend redis"
        }
        if ($RedisMode -ne "sentinel") {
            throw "redis-sentinel-master-stop requires -RedisMode sentinel"
        }
        if (-not $RedisFaultCommand) {
            $failoverScript = Join-Path $ResultDir "redis-sentinel-stop-master.ps1"
            @'
$ErrorActionPreference = "Stop"
$portToContainer = @{
    "6380" = "nexusim-redis-ha-master"
    "6381" = "nexusim-redis-ha-replica-1"
    "6382" = "nexusim-redis-ha-replica-2"
}
$before = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster)
if ($before.Count -lt 2) {
    throw "Sentinel did not return a current master before stop."
}
$beforeHost = $before[0].Trim()
$beforePort = $before[1].Trim()
$beforeAddr = "${beforeHost}:${beforePort}"
if (-not $portToContainer.ContainsKey($beforePort)) {
    throw "No local Redis container mapping for Sentinel master port $beforePort"
}
$container = $portToContainer[$beforePort]
Set-Content -LiteralPath (Join-Path $PSScriptRoot "redis-sentinel-stopped-container.txt") -Value $container -Encoding ASCII
Write-Output "sentinel_master_before=$beforeAddr"
Write-Output "stopped_container=$container"
docker stop $container | Out-Null
$deadline = (Get-Date).AddSeconds(75)
$afterAddr = ""
$afterHost = ""
$afterPort = ""
do {
    Start-Sleep -Seconds 1
    $after = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster)
    if ($after.Count -ge 2) {
        $afterHost = $after[0].Trim()
        $afterPort = $after[1].Trim()
        $afterAddr = "${afterHost}:${afterPort}"
    }
} while (($afterAddr -eq "" -or $afterAddr -eq $beforeAddr) -and (Get-Date) -lt $deadline)
if ($afterAddr -eq "" -or $afterAddr -eq $beforeAddr) {
    throw "Sentinel did not promote a different master after stopping $container; before=$beforeAddr after=$afterAddr"
}
$ping = @(docker exec nexusim-redis-sentinel-1 redis-cli -h $afterHost -p $afterPort ping)
if ($ping.Count -lt 1 -or $ping[0].Trim() -ne "PONG") {
    throw "Promoted Sentinel master did not respond to PING: $($ping -join ',')"
}
$role = @(docker exec nexusim-redis-sentinel-1 redis-cli -h $afterHost -p $afterPort role)
if ($role.Count -lt 1 -or $role[0].Trim() -ne "master") {
    throw "Promoted Sentinel master role is not master: $($role -join ',')"
}
Write-Output "sentinel_master_after=$afterAddr"
'@ | Set-Content -LiteralPath $failoverScript -Encoding UTF8
            $RedisFaultCommand = "& '$failoverScript'"
        }
        if (-not $RedisRestoreCommand) {
            $restoreScript = Join-Path $ResultDir "redis-sentinel-restore-stopped.ps1"
            @'
$ErrorActionPreference = "Stop"
$stoppedFile = Join-Path $PSScriptRoot "redis-sentinel-stopped-container.txt"
if (-not (Test-Path -LiteralPath $stoppedFile)) {
    Write-Output "sentinel_restore=skipped_no_stopped_container_file"
    return
}
$container = (Get-Content -LiteralPath $stoppedFile -Raw).Trim()
if ($container -eq "") {
    Write-Output "sentinel_restore=skipped_empty_container"
    return
}
docker start $container | Out-Null
$deadline = (Get-Date).AddSeconds(60)
do {
    Start-Sleep -Seconds 1
    $state = docker inspect -f "{{.State.Health.Status}}" $container 2>$null
} while ($state -ne "healthy" -and (Get-Date) -lt $deadline)
Write-Output "sentinel_restored_container=$container"
Write-Output "sentinel_restored_health=$state"
'@ | Set-Content -LiteralPath $restoreScript -Encoding UTF8
            $RedisRestoreCommand = "& '$restoreScript'"
        }
    }

    return [pscustomobject]@{
        RedisFaultCommand = $RedisFaultCommand
        RedisRestoreCommand = $RedisRestoreCommand
        RunnerRequestTimeout = $runnerRequestTimeout
    }
}
