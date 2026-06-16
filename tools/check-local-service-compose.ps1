$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
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
$implementedServices = @(Get-ChildItem -LiteralPath $servicesRoot -Directory |
    Sort-Object Name |
    Select-Object -ExpandProperty Name)

foreach ($service in $implementedServices) {
    if ($compose -notmatch "image:\s+nexusim/$([regex]::Escape($service)):local") {
        throw "Local service compose must use nexusim/$service`:local."
    }
    if ($compose -notmatch "container_name:\s+nexusim-$([regex]::Escape($service))-") {
        throw "Local service compose must name a container for $service."
    }
}

$expectedProcesses = @(
    "api-gateway-grpc",
    "identity-service-grpc",
    "message-service-grpc",
    "conversation-service-grpc",
    "delivery-service-grpc",
    "push-gateway-ws",
    "receipt-service-grpc",
    "contacts-service-grpc",
    "policy-service-grpc"
)

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
    "NEXUSIM_IDENTITY_ADMIN_AUTH_MODE: body"
)

foreach ($entry in $requiredEnvironment) {
    if ($compose -notmatch [regex]::Escape($entry)) {
        throw "Local service compose missing required environment entry: $entry"
    }
}

$expectedWorkerProcesses = @(
    "message-service-outbox-relay",
    "delivery-service-timeline-consumer",
    "delivery-service-outbox-relay",
    "push-gateway-delivery-consumer",
    "push-gateway-identity-consumer",
    "receipt-service-delivery-consumer",
    "receipt-service-outbox-relay",
    "contacts-service-outbox-relay",
    "identity-service-outbox-relay"
)

foreach ($process in $expectedWorkerProcesses) {
    if ($workerCompose -notmatch "(?m)^\s{2}$([regex]::Escape($process)):\s*$") {
        throw "Local service worker compose missing process: $process"
    }
}

$requiredWorkerEnvironment = @(
    "NEXUSIM_MESSAGE_SERVICE_MODE: outbox-relay",
    "NEXUSIM_DELIVERY_SERVICE_MODE: timeline-consumer",
    "NEXUSIM_DELIVERY_SERVICE_MODE: outbox-relay",
    "NEXUSIM_PUSH_GATEWAY_MODE: delivery-consumer",
    "NEXUSIM_PUSH_GATEWAY_MODE: identity-consumer",
    "NEXUSIM_RECEIPT_SERVICE_MODE: delivery-consumer",
    "NEXUSIM_RECEIPT_SERVICE_MODE: outbox-relay",
    "NEXUSIM_CONTACTS_SERVICE_MODE: outbox-relay",
    "NEXUSIM_IDENTITY_SERVICE_MODE: outbox-relay",
    "NEXUSIM_KAFKA_BROKERS: host.docker.internal:9092"
)

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

& docker compose -f $serviceComposePath config --quiet
if ($LASTEXITCODE -ne 0) {
    throw "docker compose config validation failed for local service compose."
}

& docker compose -f $serviceComposePath -f $workerComposePath config --quiet
if ($LASTEXITCODE -ne 0) {
    throw "docker compose config validation failed for local service worker compose overlay."
}

Write-Host "OK   local service compose covers $($implementedServices.Count) services and $($expectedWorkerProcesses.Count) worker roles."
