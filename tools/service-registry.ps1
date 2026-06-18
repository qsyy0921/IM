$ErrorActionPreference = "Stop"

function Get-NexusIMRepoRoot {
    if ($PSScriptRoot) {
        return (Split-Path -Parent $PSScriptRoot)
    }
    return (Resolve-Path ".").Path
}

function Get-NexusIMServiceRegistry {
    param(
        [string]$RepoRoot = (Get-NexusIMRepoRoot)
    )

    $registryPath = Join-Path $RepoRoot "docs\runbook\service-registry.json"
    if (-not (Test-Path -LiteralPath $registryPath -PathType Leaf)) {
        throw "Missing service registry: $registryPath"
    }

    try {
        return (Get-Content -LiteralPath $registryPath -Raw | ConvertFrom-Json)
    }
    catch {
        throw "Invalid service registry JSON: $($_.Exception.Message)"
    }
}

function Get-NexusIMRegistryServices {
    param(
        [string]$RepoRoot = (Get-NexusIMRepoRoot),
        [string[]]$Stages = @(),
        [switch]$Active
    )

    $registry = Get-NexusIMServiceRegistry -RepoRoot $RepoRoot
    $services = @($registry.services)
    if ($Active) {
        $activeStages = @($registry.active_stages | ForEach-Object { [string]$_ })
        $services = @($services | Where-Object { [string]$_.stage -in $activeStages })
    }
    elseif ($Stages.Count -gt 0) {
        $services = @($services | Where-Object { [string]$_.stage -in $Stages })
    }

    return @($services | Sort-Object name)
}

function Get-NexusIMRegistryServiceNames {
    param(
        [string]$RepoRoot = (Get-NexusIMRepoRoot),
        [string[]]$Stages = @(),
        [switch]$Active
    )

    return @(Get-NexusIMRegistryServices -RepoRoot $RepoRoot -Stages $Stages -Active:$Active |
        ForEach-Object { [string]$_.name } |
        Sort-Object)
}

function Get-NexusIMRegistryWorkerRoles {
    param(
        [string]$RepoRoot = (Get-NexusIMRepoRoot)
    )

    $registry = Get-NexusIMServiceRegistry -RepoRoot $RepoRoot
    return @($registry.worker_roles | Sort-Object name)
}
