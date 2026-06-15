param(
    [Parameter(Mandatory = $true)]
    [string]$SnapshotDir,
    [string]$OutputPath = "",
    [string]$MarkdownPath = ""
)

$ErrorActionPreference = "Stop"

$snapshotPath = [System.IO.Path]::GetFullPath($SnapshotDir)
if (-not (Test-Path -LiteralPath $snapshotPath -PathType Container)) {
    throw "Snapshot directory does not exist: $snapshotPath"
}

if ($OutputPath.Trim().Length -eq 0) {
    $OutputPath = Join-Path $snapshotPath "resource-summary.json"
}
if ($MarkdownPath.Trim().Length -eq 0) {
    $MarkdownPath = Join-Path $snapshotPath "resource-summary.md"
}

$runSummaryPath = Join-Path $snapshotPath "run-summary.json"
$endpointSummaryPath = Join-Path $snapshotPath "endpoint-summary.json"
$dockerStatsPath = Join-Path $snapshotPath "docker-stats.jsonl"

foreach ($path in @($runSummaryPath, $endpointSummaryPath, $dockerStatsPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Required snapshot file is missing: $path"
    }
}

function Read-JsonFile {
    param([string]$Path)

    return Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
}

function Convert-ToArray {
    param([object]$Value)

    if ($null -eq $Value) {
        return @()
    }
    if ($Value -is [System.Array]) {
        return @($Value)
    }
    return @($Value)
}

function Convert-Percent {
    param([object]$Value)

    $text = ([string]$Value).Trim()
    if ($text.EndsWith("%")) {
        $text = $text.Substring(0, $text.Length - 1)
    }
    if ($text.Length -eq 0) {
        return 0.0
    }

    $number = 0.0
    $culture = [System.Globalization.CultureInfo]::InvariantCulture
    $styles = [System.Globalization.NumberStyles]::Float
    if (-not [double]::TryParse($text, $styles, $culture, [ref]$number)) {
        throw "Cannot parse percent value: $Value"
    }
    return $number
}

function Get-ContainerRole {
    param(
        [string]$Name,
        [string[]]$ServiceContainers,
        [string[]]$BaseContainers
    )

    if ($ServiceContainers -contains $Name) {
        return "service"
    }
    if ($BaseContainers -contains $Name) {
        return "base"
    }
    return "extra"
}

function Format-Number {
    param([double]$Value)

    return $Value.ToString("0.##", [System.Globalization.CultureInfo]::InvariantCulture)
}

$runSummary = Read-JsonFile -Path $runSummaryPath
$endpoints = Convert-ToArray -Value (Read-JsonFile -Path $endpointSummaryPath)

$serviceContainers = @($runSummary.service_containers | ForEach-Object { [string]$_ })
$baseContainers = @($runSummary.base_containers | ForEach-Object { [string]$_ })
$expectedContainers = @($serviceContainers + $baseContainers)

if ($expectedContainers.Count -eq 0) {
    throw "run-summary.json does not list expected containers."
}

$endpointRows = @(
foreach ($endpoint in $endpoints) {
    $healthz = [bool]$endpoint.healthz
    $readyz = [bool]$endpoint.readyz
    [pscustomobject]@{
        service = [string]$endpoint.service
        healthz = $healthz
        readyz = $readyz
        status = if ($healthz -and $readyz) { "healthy" } elseif ($healthz) { "unready" } else { "unhealthy" }
        url = [string]$endpoint.url
    }
}
)

if ($endpointRows.Count -ne [int]$runSummary.service_count) {
    throw "Endpoint summary count $($endpointRows.Count) does not match service_count $($runSummary.service_count)."
}

$unhealthyEndpoints = @($endpointRows | Where-Object { $_.status -ne "healthy" })
if ($unhealthyEndpoints.Count -gt 0) {
    $names = ($unhealthyEndpoints | ForEach-Object { "$($_.service):$($_.status)" }) -join ", "
    throw "Endpoint summary contains unhealthy services: $names"
}

$stats = @()
foreach ($line in Get-Content -LiteralPath $dockerStatsPath) {
    if ($line.Trim().Length -eq 0) {
        continue
    }
    $stats += ($line | ConvertFrom-Json)
}

$statsByName = @{}
foreach ($stat in $stats) {
    $name = if ([string]$stat.Name) { [string]$stat.Name } else { [string]$stat.Container }
    if ($name.Trim().Length -eq 0) {
        throw "docker-stats.jsonl contains a row without Name or Container."
    }
    $statsByName[$name] = $stat
}

$missingContainers = @($expectedContainers | Where-Object { -not $statsByName.ContainsKey($_) })
if ($missingContainers.Count -gt 0) {
    throw "Docker stats missing expected containers: $($missingContainers -join ', ')"
}

$rows = foreach ($name in ($statsByName.Keys | Sort-Object)) {
    $stat = $statsByName[$name]
    $cpu = Convert-Percent -Value $stat.CPUPerc
    $mem = Convert-Percent -Value $stat.MemPerc
    [pscustomobject]@{
        name = $name
        role = Get-ContainerRole -Name $name -ServiceContainers $serviceContainers -BaseContainers $baseContainers
        cpu_percent = $cpu
        mem_percent = $mem
        mem_usage = [string]$stat.MemUsage
        net_io = [string]$stat.NetIO
        block_io = [string]$stat.BlockIO
        pids = [string]$stat.PIDs
    }
}

$roleRank = @{ service = 0; base = 1; extra = 2 }
$rows = @($rows | Sort-Object @{ Expression = { $roleRank[[string]$_.role] } }, name)

$serviceRows = @($rows | Where-Object { $_.role -eq "service" })
$baseRows = @($rows | Where-Object { $_.role -eq "base" })
$maxCpu = if ($rows.Count -gt 0) { ($rows | Measure-Object -Property cpu_percent -Maximum).Maximum } else { 0.0 }
$maxMem = if ($rows.Count -gt 0) { ($rows | Measure-Object -Property mem_percent -Maximum).Maximum } else { 0.0 }

$summary = [pscustomobject]@{
    run_name = [string]$runSummary.run_name
    created_at = [string]$runSummary.created_at
    summarized_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "single no-stream Docker stats snapshot after healthz/readyz pass; not a capacity benchmark"
    snapshot_dir = $snapshotPath
    service_count = [int]$runSummary.service_count
    totals = [pscustomobject]@{
        service_containers = $serviceRows.Count
        base_containers = $baseRows.Count
        all_containers = $rows.Count
    }
    endpoints = [pscustomobject]@{
        total = $endpointRows.Count
        healthy = @($endpointRows | Where-Object { $_.status -eq "healthy" }).Count
        unready = @($endpointRows | Where-Object { $_.status -eq "unready" }).Count
        unhealthy = @($endpointRows | Where-Object { $_.status -eq "unhealthy" }).Count
    }
    max_cpu_percent = $maxCpu
    max_mem_percent = $maxMem
    rows = $rows
}

$summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $OutputPath -Encoding UTF8

$markdown = @()
$markdown += "# Local Service Resource Snapshot"
$markdown += ""
$markdown += "- Run: $($summary.run_name)"
$markdown += "- Created at: $($summary.created_at)"
$markdown += "- Summarized at: $($summary.summarized_at)"
$markdown += "- Scope: $($summary.scope)"
$markdown += "- Endpoints: $($summary.endpoints.healthy)/$($summary.endpoints.total) healthy"
$markdown += "- Containers: $($summary.totals.service_containers) service, $($summary.totals.base_containers) base, $($summary.totals.all_containers) total"
$markdown += "- Max CPU: $(Format-Number -Value $summary.max_cpu_percent)%"
$markdown += "- Max memory: $(Format-Number -Value $summary.max_mem_percent)%"
$markdown += ""
$markdown += "| Role | Container | CPU % | Mem % | Mem usage | Net IO | Block IO | PIDs |"
$markdown += "| --- | --- | ---: | ---: | --- | --- | --- | ---: |"
foreach ($row in $rows) {
    $markdown += "| $($row.role) | $($row.name) | $(Format-Number -Value $row.cpu_percent) | $(Format-Number -Value $row.mem_percent) | $($row.mem_usage) | $($row.net_io) | $($row.block_io) | $($row.pids) |"
}
$markdown += ""
$markdown += "This summary is a health-state snapshot only. It is not a capacity benchmark or production SLO measurement."

$markdown | Set-Content -LiteralPath $MarkdownPath -Encoding UTF8

Write-Host "OK   resource summary written: $OutputPath"
Write-Host "OK   markdown summary written: $MarkdownPath"
