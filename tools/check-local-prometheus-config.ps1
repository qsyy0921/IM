$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$servicesRoot = Join-Path $repoRoot "services"
$composePath = Join-Path $repoRoot "deploy\local\docker-compose.prometheus.yml"
$configPath = Join-Path $repoRoot "deploy\local\prometheus.yml"
$apiGatewayRulesPath = Join-Path $repoRoot "deploy\local\prometheus-api-gateway-alerts.yml"
$identityRulesPath = Join-Path $repoRoot "deploy\local\prometheus-identity-service-alerts.yml"
$messageRulesPath = Join-Path $repoRoot "deploy\local\prometheus-message-service-alerts.yml"
$conversationRulesPath = Join-Path $repoRoot "deploy\local\prometheus-conversation-service-alerts.yml"
$deliveryRulesPath = Join-Path $repoRoot "deploy\local\prometheus-delivery-service-alerts.yml"
$pushGatewayRulesPath = Join-Path $repoRoot "deploy\local\prometheus-push-gateway-alerts.yml"
$receiptRulesPath = Join-Path $repoRoot "deploy\local\prometheus-receipt-service-alerts.yml"
$contactsRulesPath = Join-Path $repoRoot "deploy\local\prometheus-contacts-service-alerts.yml"
$policyRulesPath = Join-Path $repoRoot "deploy\local\prometheus-policy-service-alerts.yml"

$prometheusServices = @(
    @{ Name = "api-gateway"; DebugPort = 11904; RuleFile = "prometheus-api-gateway-alerts.yml" },
    @{ Name = "identity-service"; DebugPort = 11905; RuleFile = "prometheus-identity-service-alerts.yml" },
    @{ Name = "message-service"; DebugPort = 11910; RuleFile = "prometheus-message-service-alerts.yml" },
    @{ Name = "conversation-service"; DebugPort = 11911; RuleFile = "prometheus-conversation-service-alerts.yml" },
    @{ Name = "delivery-service"; DebugPort = 11912; RuleFile = "prometheus-delivery-service-alerts.yml" },
    @{ Name = "push-gateway"; DebugPort = 11913; RuleFile = "prometheus-push-gateway-alerts.yml" },
    @{ Name = "receipt-service"; DebugPort = 11914; RuleFile = "prometheus-receipt-service-alerts.yml" },
    @{ Name = "contacts-service"; DebugPort = 11915; RuleFile = "prometheus-contacts-service-alerts.yml" },
    @{ Name = "policy-service"; DebugPort = 11916; RuleFile = "prometheus-policy-service-alerts.yml" }
)

$implementedServices = @(Get-ChildItem -LiteralPath $servicesRoot -Directory | Sort-Object Name | Select-Object -ExpandProperty Name)
$configuredServices = @($prometheusServices | ForEach-Object { [string]$_.Name } | Sort-Object)
$serviceDiff = Compare-Object -ReferenceObject $implementedServices -DifferenceObject $configuredServices
if ($serviceDiff) {
    $diffText = ($serviceDiff | ForEach-Object { "$($_.SideIndicator) $($_.InputObject)" }) -join ", "
    throw "Prometheus service coverage mismatch with services directory: $diffText"
}

$requiredConfigFiles = @(
    $composePath,
    $configPath,
    $apiGatewayRulesPath,
    $identityRulesPath,
    $messageRulesPath,
    $conversationRulesPath,
    $deliveryRulesPath,
    $pushGatewayRulesPath,
    $receiptRulesPath,
    $contactsRulesPath,
    $policyRulesPath
)

foreach ($path in $requiredConfigFiles) {
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
$deliveryRules = Get-Content -LiteralPath $deliveryRulesPath -Raw
$pushGatewayRules = Get-Content -LiteralPath $pushGatewayRulesPath -Raw
$receiptRules = Get-Content -LiteralPath $receiptRulesPath -Raw
$contactsRules = Get-Content -LiteralPath $contactsRulesPath -Raw
$policyRules = Get-Content -LiteralPath $policyRulesPath -Raw

if ($compose -notmatch "19090:9090") {
    throw "Prometheus compose must expose host port 19090 to avoid existing local service ports."
}
if ($config -notmatch "metrics_path:\s*/metrics") {
    throw "Prometheus config must scrape local /metrics endpoints."
}
if ($config -notmatch "(?ms)^rule_files:\s*\r?\n\s*-\s*/etc/prometheus/rules/\*\.yml") {
    throw "Prometheus config must load mounted local alert rule files."
}

foreach ($service in $prometheusServices) {
    $name = [string]$service.Name
    $ruleFile = [string]$service.RuleFile
    $debugPort = [int]$service.DebugPort
    $escapedName = [regex]::Escape($name)
    $escapedRuleFile = [regex]::Escape($ruleFile)

    if ($compose -notmatch $escapedRuleFile) {
        throw "Prometheus compose must mount $name alert rules."
    }
    if ($config -notmatch "job_name:\s*nexusim-$escapedName") {
        throw "Prometheus config must define a scrape job for $name."
    }
    if ($config -notmatch "host\.docker\.internal:$debugPort") {
        throw "Prometheus config must target the local $name debug endpoint through host.docker.internal:$debugPort."
    }
    if ($config -notmatch "service:\s*$escapedName") {
        throw "Prometheus config must label the $name scrape target."
    }
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

$requiredDeliveryAlerts = @(
    "NexusIMDeliveryGrpcErrors",
    "NexusIMDeliveryMetricsQueryError",
    "NexusIMDeliveryOutboxDLQ",
    "NexusIMDeliveryOutboxPendingReady",
    "NexusIMDeliveryProjectionFailures",
    "NexusIMDeliveryTimelineWorkerErrors",
    "NexusIMDeliveryTimelineWorkerConsecutiveErrors",
    "NexusIMDeliveryOutboxRelayErrors",
    "NexusIMDeliveryOutboxRelayConsecutiveErrors",
    "NexusIMDeliveryPGPoolCanceledAcquire",
    "NexusIMDeliveryOtlpEndpointMissing"
)

foreach ($alert in $requiredDeliveryAlerts) {
    if ($deliveryRules -notmatch [regex]::Escape($alert)) {
        throw "Prometheus delivery-service rules missing alert: $alert"
    }
}

$requiredPushGatewayAlerts = @(
    "NexusIMPushGatewaySlowSessionEvictions",
    "NexusIMPushGatewayRedisRouteErrors",
    "NexusIMPushGatewayRedisRouteNoSubscriber",
    "NexusIMPushGatewayRedisSubscriberWorkerErrors",
    "NexusIMPushGatewayRedisSubscriberWorkerConsecutiveErrors",
    "NexusIMPushGatewayConsumerWorkerErrors",
    "NexusIMPushGatewayConsumerWorkerConsecutiveErrors",
    "NexusIMPushGatewayJwksRefreshFailures",
    "NexusIMPushGatewayOtlpEndpointMissing"
)

foreach ($alert in $requiredPushGatewayAlerts) {
    if ($pushGatewayRules -notmatch [regex]::Escape($alert)) {
        throw "Prometheus push-gateway rules missing alert: $alert"
    }
}

$requiredReceiptAlerts = @(
    "NexusIMReceiptGrpcErrors",
    "NexusIMReceiptMetricsQueryError",
    "NexusIMReceiptOutboxDLQ",
    "NexusIMReceiptOutboxReadyPending",
    "NexusIMReceiptDeliveryProjectionWorkerErrors",
    "NexusIMReceiptDeliveryProjectionWorkerConsecutiveErrors",
    "NexusIMReceiptOutboxRelayErrors",
    "NexusIMReceiptOutboxRelayConsecutiveErrors",
    "NexusIMReceiptPGPoolCanceledAcquire",
    "NexusIMReceiptOtlpEndpointMissing"
)

foreach ($alert in $requiredReceiptAlerts) {
    if ($receiptRules -notmatch [regex]::Escape($alert)) {
        throw "Prometheus receipt-service rules missing alert: $alert"
    }
}

$requiredContactsAlerts = @(
    "NexusIMContactsGrpcErrors",
    "NexusIMContactsMetricsQueryError",
    "NexusIMContactsOutboxDLQ",
    "NexusIMContactsOutboxReadyPending",
    "NexusIMContactsOutboxRelayErrors",
    "NexusIMContactsOutboxRelayConsecutiveErrors",
    "NexusIMContactsPGPoolCanceledAcquire",
    "NexusIMContactsPendingRequests",
    "NexusIMContactsOtlpEndpointMissing"
)

foreach ($alert in $requiredContactsAlerts) {
    if ($contactsRules -notmatch [regex]::Escape($alert)) {
        throw "Prometheus contacts-service rules missing alert: $alert"
    }
}

$requiredPolicyAlerts = @(
    "NexusIMPolicyGrpcErrors",
    "NexusIMPolicyDecisionErrors",
    "NexusIMPolicyRuleStoreQueryError",
    "NexusIMPolicyProjectionQueryError",
    "NexusIMPolicyAuditOutboxDLQ",
    "NexusIMPolicyAuditOutboxPending",
    "NexusIMPolicyProjectionWorkerErrors",
    "NexusIMPolicyProjectionWorkerConsecutiveErrors",
    "NexusIMPolicyOutboxRelayErrors",
    "NexusIMPolicyOutboxRelayConsecutiveErrors",
    "NexusIMPolicyPGPoolCanceledAcquire",
    "NexusIMPolicyOtlpEndpointMissing"
)

foreach ($alert in $requiredPolicyAlerts) {
    if ($policyRules -notmatch [regex]::Escape($alert)) {
        throw "Prometheus policy-service rules missing alert: $alert"
    }
}

Write-Host "OK   local Prometheus config"
