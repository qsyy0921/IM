$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$composePath = Join-Path $repoRoot "deploy\local\docker-compose.prometheus.yml"
$configPath = Join-Path $repoRoot "deploy\local\prometheus.yml"
$apiGatewayRulesPath = Join-Path $repoRoot "deploy\local\prometheus-api-gateway-alerts.yml"
$identityRulesPath = Join-Path $repoRoot "deploy\local\prometheus-identity-service-alerts.yml"
$messageRulesPath = Join-Path $repoRoot "deploy\local\prometheus-message-service-alerts.yml"
$conversationRulesPath = Join-Path $repoRoot "deploy\local\prometheus-conversation-service-alerts.yml"

foreach ($path in @($composePath, $configPath, $apiGatewayRulesPath, $identityRulesPath, $messageRulesPath, $conversationRulesPath)) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing local Prometheus config file: $path"
    }
}

$compose = Get-Content -LiteralPath $composePath -Raw
$config = Get-Content -LiteralPath $configPath -Raw
$apiGatewayRules = Get-Content -LiteralPath $apiGatewayRulesPath -Raw
$identityRules = Get-Content -LiteralPath $identityRulesPath -Raw
$messageRules = Get-Content -LiteralPath $messageRulesPath -Raw
$conversationRules = Get-Content -LiteralPath $conversationRulesPath -Raw

if ($compose -notmatch "19090:9090") {
    throw "Prometheus compose must expose host port 19090 to avoid existing local service ports."
}
if ($compose -notmatch "prometheus-api-gateway-alerts\.yml") {
    throw "Prometheus compose must mount api-gateway alert rules."
}
if ($compose -notmatch "prometheus-identity-service-alerts\.yml") {
    throw "Prometheus compose must mount identity-service alert rules."
}
if ($compose -notmatch "prometheus-message-service-alerts\.yml") {
    throw "Prometheus compose must mount message-service alert rules."
}
if ($compose -notmatch "prometheus-conversation-service-alerts\.yml") {
    throw "Prometheus compose must mount conversation-service alert rules."
}
if ($config -notmatch "metrics_path:\s*/metrics") {
    throw "Prometheus config must scrape local /metrics endpoints."
}
if ($config -notmatch "host\.docker\.internal:11904") {
    throw "Prometheus config must target the local api-gateway debug endpoint through host.docker.internal:11904."
}
if ($config -notmatch "host\.docker\.internal:11905") {
    throw "Prometheus config must target the local identity-service debug endpoint through host.docker.internal:11905."
}
if ($config -notmatch "host\.docker\.internal:11910") {
    throw "Prometheus config must target the local message-service debug endpoint through host.docker.internal:11910."
}
if ($config -notmatch "host\.docker\.internal:11911") {
    throw "Prometheus config must target the local conversation-service debug endpoint through host.docker.internal:11911."
}
if ($config -notmatch "service:\s*identity-service") {
    throw "Prometheus config must label the identity-service scrape target."
}
if ($config -notmatch "service:\s*message-service") {
    throw "Prometheus config must label the message-service scrape target."
}
if ($config -notmatch "service:\s*conversation-service") {
    throw "Prometheus config must label the conversation-service scrape target."
}

$requiredAPIGatewayAlerts = @(
    "NexusIMApiGatewayGrpcErrors",
    "NexusIMApiGatewayLegacyDescriptorTraffic",
    "NexusIMApiGatewayLegacyDescriptorStillRegistered",
    "NexusIMApiGatewayLegacyDescriptorOptInExpired",
    "NexusIMApiGatewayRateLimitRedisErrors",
    "NexusIMApiGatewayRateLimitIdentityErrors",
    "NexusIMApiGatewayTenantQuotaReloadErrors",
    "NexusIMApiGatewayTenantQuotaSnapshotStale",
    "NexusIMApiGatewayJwksRefreshFailures",
    "NexusIMApiGatewayOtlpEndpointMissing"
)

foreach ($alert in $requiredAPIGatewayAlerts) {
    if ($apiGatewayRules -notmatch [regex]::Escape($alert)) {
        throw "Prometheus api-gateway rules missing alert: $alert"
    }
}

$requiredIdentityAlerts = @(
    "NexusIMIdentityGrpcErrors",
    "NexusIMIdentityPasswordLoginLocked",
    "NexusIMIdentityMFALoginLocked",
    "NexusIMIdentityMFARecoveryLocked",
    "NexusIMIdentityChallengeDeliveryFailures",
    "NexusIMIdentityChallengeDeliveryOutboxDLQ",
    "NexusIMIdentityChallengeDeliveryOutboxPendingExpired",
    "NexusIMIdentityChallengeDeliveryWorkerErrors",
    "NexusIMIdentityOutboxRelayErrors",
    "NexusIMIdentityOtlpEndpointMissing"
)

foreach ($alert in $requiredIdentityAlerts) {
    if ($identityRules -notmatch [regex]::Escape($alert)) {
        throw "Prometheus identity-service rules missing alert: $alert"
    }
}

$requiredMessageAlerts = @(
    "NexusIMMessageSendLatencyHigh",
    "NexusIMMessageRepositoryPoolAcquireLatencyHigh",
    "NexusIMMessageKafkaPublishLatencyHigh",
    "NexusIMMessageOutboxRelayErrors",
    "NexusIMMessageOutboxRelayConsecutiveErrors",
    "NexusIMMessageOtlpEndpointMissing"
)

foreach ($alert in $requiredMessageAlerts) {
    if ($messageRules -notmatch [regex]::Escape($alert)) {
        throw "Prometheus message-service rules missing alert: $alert"
    }
}

$requiredConversationAlerts = @(
    "NexusIMConversationGrpcErrors",
    "NexusIMConversationMetricsQueryError",
    "NexusIMConversationMemberChangeFailedCompensated",
    "NexusIMConversationMemberChangeWorkerErrors",
    "NexusIMConversationMemberChangeWorkerConsecutiveErrors",
    "NexusIMConversationPGPoolCanceledAcquire",
    "NexusIMConversationOtlpEndpointMissing"
)

foreach ($alert in $requiredConversationAlerts) {
    if ($conversationRules -notmatch [regex]::Escape($alert)) {
        throw "Prometheus conversation-service rules missing alert: $alert"
    }
}

Write-Host "OK   local Prometheus config"
