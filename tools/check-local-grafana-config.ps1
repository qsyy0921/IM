$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$servicesRoot = Join-Path $repoRoot "services"
$composePath = Join-Path $repoRoot "deploy\local\docker-compose.grafana.yml"
$datasourcePath = Join-Path $repoRoot "deploy\local\grafana-datasources.yml"
$providerPath = Join-Path $repoRoot "deploy\local\grafana-dashboards.yml"
$dashboardRoot = Join-Path $repoRoot "deploy\local\grafana\dashboards"
$apiGatewayDashboardPath = Join-Path $repoRoot "deploy\local\grafana\dashboards\api-gateway-observability.json"
$identityDashboardPath = Join-Path $repoRoot "deploy\local\grafana\dashboards\identity-service-observability.json"
$messageDashboardPath = Join-Path $repoRoot "deploy\local\grafana\dashboards\message-service-observability.json"
$conversationDashboardPath = Join-Path $repoRoot "deploy\local\grafana\dashboards\conversation-service-observability.json"
$deliveryDashboardPath = Join-Path $repoRoot "deploy\local\grafana\dashboards\delivery-service-observability.json"
$pushGatewayDashboardPath = Join-Path $repoRoot "deploy\local\grafana\dashboards\push-gateway-observability.json"
$receiptDashboardPath = Join-Path $repoRoot "deploy\local\grafana\dashboards\receipt-service-observability.json"
$contactsDashboardPath = Join-Path $repoRoot "deploy\local\grafana\dashboards\contacts-service-observability.json"
$policyDashboardPath = Join-Path $repoRoot "deploy\local\grafana\dashboards\policy-service-observability.json"

foreach ($path in @($composePath, $datasourcePath, $providerPath, $apiGatewayDashboardPath, $identityDashboardPath, $messageDashboardPath, $conversationDashboardPath, $deliveryDashboardPath, $pushGatewayDashboardPath, $receiptDashboardPath, $contactsDashboardPath, $policyDashboardPath)) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing local Grafana config file: $path"
    }
}

$implementedServices = @(Get-ChildItem -LiteralPath $servicesRoot -Directory | Sort-Object Name | Select-Object -ExpandProperty Name)
$dashboardServices = @(Get-ChildItem -LiteralPath $dashboardRoot -Filter "*-observability.json" -File |
    ForEach-Object { $_.BaseName -replace "-observability$", "" } |
    Sort-Object)
$serviceDiff = Compare-Object -ReferenceObject $implementedServices -DifferenceObject $dashboardServices
if ($serviceDiff) {
    $diffText = ($serviceDiff | ForEach-Object { "$($_.SideIndicator) $($_.InputObject)" }) -join ", "
    throw "Grafana dashboard coverage mismatch with services directory: $diffText"
}

$compose = Get-Content -LiteralPath $composePath -Raw
$datasource = Get-Content -LiteralPath $datasourcePath -Raw
$provider = Get-Content -LiteralPath $providerPath -Raw

if ($compose -notmatch "13000:3000") {
    throw "Grafana compose must expose host port 13000 to avoid existing local service ports."
}
if ($datasource -notmatch "http://host\.docker\.internal:19090") {
    throw "Grafana datasource must point at the local Prometheus host port through host.docker.internal:19090."
}
if ($provider -notmatch "/var/lib/grafana/dashboards") {
    throw "Grafana dashboard provider must load dashboards from /var/lib/grafana/dashboards."
}

$forbiddenLabels = @("tenant_id", "user_id", "device_id", "session_id", "request_id", "trace_id", "gateway-token")

function Test-Dashboard {
    param(
        [string]$Path,
        [string]$Name,
        [string]$ExpectedUid,
        [int]$MinimumPanels,
        [string[]]$RequiredMetrics
    )

    $raw = Get-Content -LiteralPath $Path -Raw
    try {
        $dashboard = $raw | ConvertFrom-Json
    } catch {
        throw "Grafana $Name dashboard is not valid JSON: $($_.Exception.Message)"
    }

    if ($dashboard.uid -ne $ExpectedUid) {
        throw "Grafana $Name dashboard uid mismatch."
    }
    if (-not $dashboard.panels -or $dashboard.panels.Count -lt $MinimumPanels) {
        throw "Grafana $Name dashboard should include core observability panels."
    }

    foreach ($metric in $RequiredMetrics) {
        if ($raw -notmatch [regex]::Escape($metric)) {
            throw "Grafana $Name dashboard missing metric: $metric"
        }
    }

    foreach ($label in $forbiddenLabels) {
        if ($raw -match [regex]::Escape($label)) {
            throw "Grafana $Name dashboard must not reference sensitive or high-cardinality field: $label"
        }
    }
}

$apiGatewayRequiredMetrics = @(
    "nexusim_api_gateway_grpc_requests_total",
    "nexusim_api_gateway_grpc_errors_total",
    "nexusim_api_gateway_grpc_exposure_requests_total",
    "nexusim_api_gateway_grpc_legacy_descriptor_last_seen_unix_milliseconds",
    "nexusim_api_gateway_grpc_latency_avg_milliseconds",
    "nexusim_api_gateway_rate_limit_limited_total",
    "nexusim_api_gateway_rate_limit_redis_errors_total",
    "nexusim_api_gateway_rate_limit_identity_errors_total",
    "nexusim_api_gateway_rate_limit_tenant_plan_reload_errors_total",
    "redis_mode",
    "nexusim_api_gateway_auth_jwks_refresh_failures_total",
    "nexusim_api_gateway_legacy_descriptors_registered",
    "nexusim_api_gateway_otel_traces_enabled"
)

$identityRequiredMetrics = @(
    "nexusim_identity_grpc_requests_total",
    "nexusim_identity_grpc_errors_total",
    "nexusim_identity_grpc_latency_avg_milliseconds",
    "nexusim_identity_password_login_locked",
    "nexusim_identity_mfa_login_locked",
    "nexusim_identity_mfa_recovery_locked",
    "nexusim_identity_challenge_delivery_requests_total",
    "nexusim_identity_challenge_delivery_outbox",
    "nexusim_identity_challenge_delivery_worker_errors_total",
    "nexusim_identity_outbox_relay_errors_total",
    "nexusim_identity_otel_traces_enabled"
)

$messageRequiredMetrics = @(
    "nexusim_message_latency_samples_total",
    "nexusim_message_latency_p95_milliseconds",
    "nexusim_message_value_avg",
    "nexusim_message_pg_pool_conns",
    "nexusim_message_outbox_relay_errors_total",
    "nexusim_message_outbox_relay_consecutive_errors",
    "nexusim_message_otel_traces_enabled"
)

$conversationRequiredMetrics = @(
    "nexusim_conversation_grpc_method_requests_total",
    "nexusim_conversation_grpc_method_errors_total",
    "nexusim_conversation_grpc_latency_avg_milliseconds",
    "nexusim_conversation_conversations",
    "nexusim_conversation_members",
    "nexusim_conversation_member_changes",
    "nexusim_conversation_member_change_worker_consecutive_errors",
    "nexusim_conversation_pg_pool_conns",
    "nexusim_conversation_otel_traces_enabled"
)

$deliveryRequiredMetrics = @(
    "nexusim_delivery_grpc_method_requests_total",
    "nexusim_delivery_grpc_method_errors_total",
    "nexusim_delivery_grpc_latency_avg_milliseconds",
    "nexusim_delivery_read_model",
    "nexusim_delivery_membership_projection",
    "nexusim_delivery_outbox",
    "nexusim_delivery_projection_failures",
    "nexusim_delivery_projection_failures_by_class",
    "nexusim_delivery_timeline_worker_consecutive_errors",
    "nexusim_delivery_outbox_relay_consecutive_errors",
    "nexusim_delivery_pg_pool_conns",
    "nexusim_delivery_otel_traces_enabled"
)

$pushGatewayRequiredMetrics = @(
    "nexusim_push_gateway_sessions",
    "nexusim_push_gateway_session_events_total",
    "nexusim_push_gateway_resume_buffer",
    "nexusim_push_gateway_resume_buffer_events_total",
    "nexusim_push_gateway_redis_route_events_total",
    "nexusim_push_gateway_redis_resume_events_total",
    "nexusim_push_gateway_redis_subscriber_worker_consecutive_errors",
    "nexusim_push_gateway_consumer_worker_errors_total",
    "nexusim_push_gateway_consumer_worker_consecutive_errors",
    "nexusim_push_gateway_auth_jwks_cached_keys",
    "nexusim_push_gateway_auth_jwks_refresh_failures_total",
    "nexusim_push_gateway_otel_traces_enabled"
)

$receiptRequiredMetrics = @(
    "nexusim_receipt_grpc_method_requests_total",
    "nexusim_receipt_grpc_method_errors_total",
    "nexusim_receipt_grpc_latency_avg_milliseconds",
    "nexusim_receipt_projection",
    "nexusim_receipt_conversation_summary",
    "nexusim_receipt_kafka_checkpoints",
    "nexusim_receipt_outbox",
    "nexusim_receipt_outbox_age_milliseconds",
    "nexusim_receipt_delivery_projection_worker_errors_total",
    "nexusim_receipt_delivery_projection_worker_consecutive_errors",
    "nexusim_receipt_outbox_relay_errors_total",
    "nexusim_receipt_outbox_relay_consecutive_errors",
    "nexusim_receipt_pg_pool_conns",
    "nexusim_receipt_otel_traces_enabled"
)

$contactsRequiredMetrics = @(
    "nexusim_contacts_grpc_method_requests_total",
    "nexusim_contacts_grpc_method_errors_total",
    "nexusim_contacts_grpc_latency_avg_milliseconds",
    "nexusim_contacts_requests",
    "nexusim_contacts_request_status",
    "nexusim_contacts_edges",
    "nexusim_contacts_edge_status",
    "nexusim_contacts_command_idempotency_total",
    "nexusim_contacts_outbox",
    "nexusim_contacts_outbox_age_milliseconds",
    "nexusim_contacts_outbox_relay_errors_total",
    "nexusim_contacts_outbox_relay_consecutive_errors",
    "nexusim_contacts_pg_pool_conns",
    "nexusim_contacts_otel_traces_enabled"
)

$policyRequiredMetrics = @(
    "nexusim_policy_grpc_method_requests_total",
    "nexusim_policy_grpc_method_errors_total",
    "nexusim_policy_grpc_latency_avg_milliseconds",
    "nexusim_policy_decisions_total",
    "nexusim_policy_decision_action_total",
    "nexusim_policy_rules",
    "nexusim_policy_rule_actions",
    "nexusim_policy_role_rules",
    "nexusim_policy_contact_edges_projection",
    "nexusim_policy_conversation_members_projection",
    "nexusim_policy_kafka_checkpoints",
    "nexusim_policy_audit_outbox",
    "nexusim_policy_projection_worker_errors_total",
    "nexusim_policy_projection_worker_consecutive_errors",
    "nexusim_policy_outbox_relay_errors_total",
    "nexusim_policy_outbox_relay_consecutive_errors",
    "nexusim_policy_pg_pool_conns",
    "nexusim_policy_otel_traces_enabled"
)

Test-Dashboard -Path $apiGatewayDashboardPath -Name "api-gateway" -ExpectedUid "nexusim-api-gateway" -MinimumPanels 5 -RequiredMetrics $apiGatewayRequiredMetrics
Test-Dashboard -Path $identityDashboardPath -Name "identity-service" -ExpectedUid "nexusim-identity-service" -MinimumPanels 8 -RequiredMetrics $identityRequiredMetrics
Test-Dashboard -Path $messageDashboardPath -Name "message-service" -ExpectedUid "nexusim-message-service" -MinimumPanels 8 -RequiredMetrics $messageRequiredMetrics
Test-Dashboard -Path $conversationDashboardPath -Name "conversation-service" -ExpectedUid "nexusim-conversation-service" -MinimumPanels 8 -RequiredMetrics $conversationRequiredMetrics
Test-Dashboard -Path $deliveryDashboardPath -Name "delivery-service" -ExpectedUid "nexusim-delivery-service" -MinimumPanels 8 -RequiredMetrics $deliveryRequiredMetrics
Test-Dashboard -Path $pushGatewayDashboardPath -Name "push-gateway" -ExpectedUid "nexusim-push-gateway" -MinimumPanels 8 -RequiredMetrics $pushGatewayRequiredMetrics
Test-Dashboard -Path $receiptDashboardPath -Name "receipt-service" -ExpectedUid "nexusim-receipt-service" -MinimumPanels 8 -RequiredMetrics $receiptRequiredMetrics
Test-Dashboard -Path $contactsDashboardPath -Name "contacts-service" -ExpectedUid "nexusim-contacts-service" -MinimumPanels 8 -RequiredMetrics $contactsRequiredMetrics
Test-Dashboard -Path $policyDashboardPath -Name "policy-service" -ExpectedUid "nexusim-policy-service" -MinimumPanels 8 -RequiredMetrics $policyRequiredMetrics

Write-Host "OK   local Grafana config"
