$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot "service-registry.ps1")

$servicesRoot = Join-Path $repoRoot "services"
$serviceComposePath = Join-Path $repoRoot "deploy\local\docker-compose.services.yml"
$workerComposePath = Join-Path $repoRoot "deploy\local\docker-compose.service-workers.yml"

if (-not (Test-Path -LiteralPath $serviceComposePath)) {
    throw "Missing local service compose file: $serviceComposePath"
}
if (-not (Test-Path -LiteralPath $workerComposePath)) {
    throw "Missing local service worker compose file: $workerComposePath"
}

$compose = Get-Content -LiteralPath $serviceComposePath -Raw
$workerCompose = Get-Content -LiteralPath $workerComposePath -Raw
$implementedServices = Get-NexusIMRegistryServiceNames -RepoRoot $repoRoot -Active
$actualServiceDirs = @(Get-ChildItem -LiteralPath $servicesRoot -Directory |
    Sort-Object Name |
    Select-Object -ExpandProperty Name)
$serviceDirDiff = Compare-Object -ReferenceObject $implementedServices -DifferenceObject $actualServiceDirs
if ($serviceDirDiff) {
    $diffText = ($serviceDirDiff | ForEach-Object { "$($_.SideIndicator) $($_.InputObject)" }) -join ", "
    throw "Service registry active services mismatch with services directory: $diffText"
}

foreach ($service in $implementedServices) {
    if ($compose -notmatch "image:\s+nexusim/$([regex]::Escape($service)):local") {
        throw "Local service compose must use nexusim/$service`:local."
    }
    if ($compose -notmatch "container_name:\s+nexusim-$([regex]::Escape($service))-") {
        throw "Local service compose must name a container for $service."
    }
}

$expectedProcesses = @(Get-NexusIMRegistryServices -RepoRoot $repoRoot -Active |
    Where-Object { [string]$_.local_process -ne "" } |
    ForEach-Object { [string]$_.local_process } |
    Sort-Object)

foreach ($process in $expectedProcesses) {
    if ($compose -notmatch "(?m)^\s{2}$([regex]::Escape($process)):\s*$") {
        throw "Local service compose missing service process: $process"
    }
}

$requiredEnvironment = @(
    "NEXUSIM_API_GATEWAY_AUTH_MODE: mock",
    "NEXUSIM_PUSH_AUTH_MODE: mock",
    "NEXUSIM_PUSH_ROUTE_BACKEND: redis",
    "NEXUSIM_MESSAGE_AUTH_MODE: body",
    "NEXUSIM_CONVERSATION_AUTH_MODE: body",
    "NEXUSIM_DELIVERY_AUTH_MODE: body",
    "NEXUSIM_RECEIPT_AUTH_MODE: body",
    "NEXUSIM_CONTACTS_AUTH_MODE: body",
    "NEXUSIM_IDENTITY_ADMIN_AUTH_MODE: body",
    "NEXUSIM_CONVERSATION_RPC_TIMEOUT: 500ms",
    "NEXUSIM_POLICY_RPC_TIMEOUT: 2s"
)

foreach ($entry in $requiredEnvironment) {
    if ($compose -notmatch [regex]::Escape($entry)) {
        throw "Local service compose missing required environment entry: $entry"
    }
}

$expectedWorkerProcesses = @(Get-NexusIMRegistryWorkerRoles -RepoRoot $repoRoot |
    ForEach-Object { [string]$_.name } |
    Sort-Object)

foreach ($process in $expectedWorkerProcesses) {
    if ($workerCompose -notmatch "(?m)^\s{2}$([regex]::Escape($process)):\s*$") {
        throw "Local service worker compose missing process: $process"
    }
}

$requiredWorkerEnvironment = @(Get-NexusIMRegistryWorkerRoles -RepoRoot $repoRoot |
    ForEach-Object { "$([string]$_.env): $([string]$_.value)" })
$requiredWorkerEnvironment += "NEXUSIM_KAFKA_BROKERS: kafka:29092"

foreach ($entry in $requiredWorkerEnvironment) {
    if ($workerCompose -notmatch [regex]::Escape($entry)) {
        throw "Local service worker compose missing required environment entry: $entry"
    }
}

if ($compose -notmatch "172\.30\.80\.0/24") {
    throw "Local service compose must use the private 172.30.80.0/24 Docker network."
}
if ($compose -notmatch "host\.docker\.internal:5432" -or $compose -notmatch "host\.docker\.internal:6379") {
    throw "Local service compose must use host.docker.internal for local infrastructure access."
}
if ($compose -match "depends_on:\s*\r?\n\s*(postgres|redis|kafka):") {
    throw "Local service compose must not depend on the base compose services or mutate the base compose network."
}
if ($compose -match "NEXUSIM_.*(TOKEN|SECRET|PASSWORD).*:.*(sk-|bearer|token=|password=)") {
    throw "Local service compose appears to contain a high-risk secret literal."
}
if ($workerCompose -match "NEXUSIM_.*(TOKEN|SECRET|PASSWORD).*:.*(sk-|bearer|token=|password=)") {
    throw "Local service worker compose appears to contain a high-risk secret literal."
}
if ($workerCompose -notmatch "name:\s+nexusim-local_default") {
    throw "Local service worker compose must attach workers to the base infrastructure network for Kafka internal listener access."
}

& docker compose -f $serviceComposePath config --quiet
if ($LASTEXITCODE -ne 0) {
    throw "docker compose config validation failed for local service compose."
}

& docker compose -f $serviceComposePath -f $workerComposePath config --quiet
if ($LASTEXITCODE -ne 0) {
    throw "docker compose config validation failed for local service worker compose overlay."
}

Write-Host "OK   local service compose covers $($implementedServices.Count) services and $($expectedWorkerProcesses.Count) worker roles."
