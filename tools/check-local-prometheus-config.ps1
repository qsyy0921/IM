$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot "service-registry.ps1")

$servicesRoot = Join-Path $repoRoot "services"
$composePath = Join-Path $repoRoot "deploy\local\docker-compose.prometheus.yml"
$configPath = Join-Path $repoRoot "deploy\local\prometheus.yml"

$prometheusServices = @(Get-NexusIMRegistryServices -RepoRoot $repoRoot -Active |
    Where-Object { [string]$_.prometheus_rule_file -ne "" -and [int]$_.debug_port -gt 0 } |
    ForEach-Object {
        @{
            Name = [string]$_.name
            DebugPort = [int]$_.debug_port
            RuleFile = [string]$_.prometheus_rule_file
        }
    })

$implementedServices = Get-NexusIMRegistryServiceNames -RepoRoot $repoRoot -Active
$actualServiceDirs = @(Get-ChildItem -LiteralPath $servicesRoot -Directory | Sort-Object Name | Select-Object -ExpandProperty Name)
$serviceDirDiff = Compare-Object -ReferenceObject $implementedServices -DifferenceObject $actualServiceDirs
if ($serviceDirDiff) {
    $diffText = ($serviceDirDiff | ForEach-Object { "$($_.SideIndicator) $($_.InputObject)" }) -join ", "
    throw "Service registry active services mismatch with services directory: $diffText"
}

$configuredServices = @($prometheusServices | ForEach-Object { [string]$_.Name } | Sort-Object)
$serviceDiff = Compare-Object -ReferenceObject $implementedServices -DifferenceObject $configuredServices
if ($serviceDiff) {
    $diffText = ($serviceDiff | ForEach-Object { "$($_.SideIndicator) $($_.InputObject)" }) -join ", "
    throw "Prometheus service coverage mismatch with services directory: $diffText"
}

$requiredConfigFiles = @($composePath, $configPath)
$requiredConfigFiles += @($prometheusServices | ForEach-Object { Join-Path $repoRoot "deploy\local\$([string]$_.RuleFile)" })

foreach ($path in $requiredConfigFiles) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing local Prometheus config file: $path"
    }
}

$compose = Get-Content -LiteralPath $composePath -Raw
$config = Get-Content -LiteralPath $configPath -Raw
$rulesByService = @{}
foreach ($service in $prometheusServices) {
    $rulesByService[[string]$service.Name] = Get-Content -LiteralPath (Join-Path $repoRoot "deploy\local\$([string]$service.RuleFile)") -Raw
}

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
    if ($rulesByService["api-gateway"] -notmatch [regex]::Escape($alert)) {
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
    if ($rulesByService["identity-service"] -notmatch [regex]::Escape($alert)) {
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
    if ($rulesByService["message-service"] -notmatch [regex]::Escape($alert)) {
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
    if ($rulesByService["conversation-service"] -notmatch [regex]::Escape($alert)) {
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
    if ($rulesByService["delivery-service"] -notmatch [regex]::Escape($alert)) {
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
    if ($rulesByService["push-gateway"] -notmatch [regex]::Escape($alert)) {
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
    if ($rulesByService["receipt-service"] -notmatch [regex]::Escape($alert)) {
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
    if ($rulesByService["contacts-service"] -notmatch [regex]::Escape($alert)) {
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
    if ($rulesByService["policy-service"] -notmatch [regex]::Escape($alert)) {
        throw "Prometheus policy-service rules missing alert: $alert"
    }
}

$requiredSearchAlerts = @(
    "NexusIMSearchServiceDown",
    "NexusIMSearchServiceInfoMissing"
)

foreach ($alert in $requiredSearchAlerts) {
    if ($rulesByService["search-service"] -notmatch [regex]::Escape($alert)) {
        throw "Prometheus search-service rules missing alert: $alert"
    }
}

$requiredMemoryAlerts = @(
    "NexusIMMemoryServiceDown",
    "NexusIMMemoryServiceInfoMissing"
)

foreach ($alert in $requiredMemoryAlerts) {
    if ($rulesByService["memory-service"] -notmatch [regex]::Escape($alert)) {
        throw "Prometheus memory-service rules missing alert: $alert"
    }
}

$requiredRAGAlerts = @(
    "NexusIMRAGServiceDown",
    "NexusIMRAGServiceInfoMissing"
)

foreach ($alert in $requiredRAGAlerts) {
    if ($rulesByService["rag-service"] -notmatch [regex]::Escape($alert)) {
        throw "Prometheus rag-service rules missing alert: $alert"
    }
}

$requiredSummaryAlerts = @(
    "NexusIMSummaryServiceDown",
    "NexusIMSummaryServiceInfoMissing"
)

foreach ($alert in $requiredSummaryAlerts) {
    if ($rulesByService["summary-service"] -notmatch [regex]::Escape($alert)) {
        throw "Prometheus summary-service rules missing alert: $alert"
    }
}

$requiredAgentAlerts = @(
    "NexusIMAgentServiceDown",
    "NexusIMAgentServiceInfoMissing"
)

foreach ($alert in $requiredAgentAlerts) {
    if ($rulesByService["agent-service"] -notmatch [regex]::Escape($alert)) {
        throw "Prometheus agent-service rules missing alert: $alert"
    }
}

$requiredSkillRegistryAlerts = @(
    "NexusIMSkillRegistryDown",
    "NexusIMSkillRegistryInfoMissing"
)

foreach ($alert in $requiredSkillRegistryAlerts) {
    if ($rulesByService["skill-registry"] -notmatch [regex]::Escape($alert)) {
        throw "Prometheus skill-registry rules missing alert: $alert"
    }
}

$requiredMCPGatewayAlerts = @(
    "NexusIMMCPGatewayDown",
    "NexusIMMCPGatewayInfoMissing"
)

foreach ($alert in $requiredMCPGatewayAlerts) {
    if ($rulesByService["mcp-gateway"] -notmatch [regex]::Escape($alert)) {
        throw "Prometheus mcp-gateway rules missing alert: $alert"
    }
}

$requiredActionExecutorAlerts = @(
    "NexusIMActionExecutorDown",
    "NexusIMActionExecutorInfoMissing"
)

foreach ($alert in $requiredActionExecutorAlerts) {
    if ($rulesByService["action-executor"] -notmatch [regex]::Escape($alert)) {
        throw "Prometheus action-executor rules missing alert: $alert"
    }
}

$requiredAIEvalServiceAlerts = @(
    "NexusIMAIEvalServiceDown",
    "NexusIMAIEvalServiceInfoMissing"
)

foreach ($alert in $requiredAIEvalServiceAlerts) {
    if ($rulesByService["ai-eval-service"] -notmatch [regex]::Escape($alert)) {
        throw "Prometheus ai-eval-service rules missing alert: $alert"
    }
}

Write-Host "OK   local Prometheus config"
