package monitoring

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"github.com/qsyy0921/IM/services/api-gateway/internal/infrastructure/ratelimit"
)

func renderPrometheus(snapshot Snapshot) string {
	var builder strings.Builder
	writePrometheusHeader(&builder, "nexusim_api_gateway_metrics_generated_at_unix_milliseconds", "Gauge", "Unix time when the api-gateway metrics snapshot was generated in milliseconds.")
	writePrometheusSample(&builder, "nexusim_api_gateway_metrics_generated_at_unix_milliseconds", nil, strconv.FormatInt(snapshot.GeneratedAtMS, 10))

	writePrometheusHeader(&builder, "nexusim_api_gateway_build_info", "Gauge", "Static api-gateway service information.")
	writePrometheusSample(&builder, "nexusim_api_gateway_build_info", map[string]string{"service": snapshot.Service}, "1")

	if snapshot.GRPC != nil {
		writeGRPCPrometheus(&builder, *snapshot.GRPC)
	}
	if snapshot.AuthJWKs != nil {
		writeAuthJWKPrometheus(&builder, *snapshot.AuthJWKs)
	}
	if snapshot.RateLimit != nil {
		writeRateLimitPrometheus(&builder, *snapshot.RateLimit)
	}
	if snapshot.Runtime != nil {
		writeRuntimePrometheus(&builder, *snapshot.Runtime)
	}
	if snapshot.Trace != nil {
		writeTracePrometheus(&builder, *snapshot.Trace)
	}
	return builder.String()
}

func writeGRPCPrometheus(builder *strings.Builder, snapshot GRPCSnapshot) {
	writePrometheusHeader(builder, "nexusim_api_gateway_grpc_requests_total", "Counter", "Total api-gateway gRPC requests by method and status code.")
	writePrometheusHeader(builder, "nexusim_api_gateway_grpc_errors_total", "Counter", "Total api-gateway non-OK gRPC requests by method.")
	writePrometheusHeader(builder, "nexusim_api_gateway_grpc_latency_avg_milliseconds", "Gauge", "Average api-gateway gRPC request latency by method.")
	writePrometheusHeader(builder, "nexusim_api_gateway_grpc_latency_max_milliseconds", "Gauge", "Maximum api-gateway gRPC request latency by method.")
	writePrometheusHeader(builder, "nexusim_api_gateway_grpc_exposure_requests_total", "Counter", "Total api-gateway gRPC requests by exposure class.")
	writePrometheusSample(builder, "nexusim_api_gateway_grpc_exposure_requests_total", map[string]string{"exposure": "facade"}, strconv.FormatInt(snapshot.FacadeRequests, 10))
	writePrometheusSample(builder, "nexusim_api_gateway_grpc_exposure_requests_total", map[string]string{"exposure": "legacy_descriptor"}, strconv.FormatInt(snapshot.LegacyDescriptorRequests, 10))
	writePrometheusSample(builder, "nexusim_api_gateway_grpc_exposure_requests_total", map[string]string{"exposure": "other"}, strconv.FormatInt(snapshot.OtherRequests, 10))
	writePrometheusHeader(builder, "nexusim_api_gateway_grpc_legacy_descriptor_last_seen_unix_milliseconds", "Gauge", "Unix time of the last observed api-gateway legacy descriptor request in milliseconds.")
	writePrometheusSample(builder, "nexusim_api_gateway_grpc_legacy_descriptor_last_seen_unix_milliseconds", nil, strconv.FormatInt(snapshot.LegacyDescriptorLastSeenMS, 10))

	methods := append([]GRPCMethodSnapshot(nil), snapshot.Methods...)
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Method < methods[j].Method
	})
	for _, method := range methods {
		codeNames := make([]string, 0, len(method.Codes))
		for code := range method.Codes {
			codeNames = append(codeNames, code)
		}
		sort.Strings(codeNames)
		for _, code := range codeNames {
			writePrometheusSample(builder, "nexusim_api_gateway_grpc_requests_total", map[string]string{
				"method": method.Method,
				"code":   code,
			}, strconv.FormatInt(method.Codes[code], 10))
		}
		writePrometheusSample(builder, "nexusim_api_gateway_grpc_errors_total", map[string]string{
			"method": method.Method,
		}, strconv.FormatInt(method.ErrorCount, 10))
		writePrometheusSample(builder, "nexusim_api_gateway_grpc_latency_avg_milliseconds", map[string]string{
			"method": method.Method,
		}, strconv.FormatInt(method.LatencyAvgMS, 10))
		writePrometheusSample(builder, "nexusim_api_gateway_grpc_latency_max_milliseconds", map[string]string{
			"method": method.Method,
		}, strconv.FormatInt(method.LatencyMaxMS, 10))
	}
}

func writeAuthJWKPrometheus(builder *strings.Builder, snapshot gatewayauth.JWKStats) {
	writePrometheusHeader(builder, "nexusim_api_gateway_auth_jwks_remote_url_configured", "Gauge", "Whether api-gateway JWT authentication has a remote JWKS URL configured.")
	writePrometheusSample(builder, "nexusim_api_gateway_auth_jwks_remote_url_configured", nil, prometheusBool(snapshot.RemoteURLConfigured))
	writePrometheusHeader(builder, "nexusim_api_gateway_auth_jwks_cached_keys", "Gauge", "Number of api-gateway cached JWT verification public keys.")
	writePrometheusSample(builder, "nexusim_api_gateway_auth_jwks_cached_keys", nil, strconv.Itoa(snapshot.CachedKeyCount))
	writePrometheusHeader(builder, "nexusim_api_gateway_auth_jwks_refresh_failures_total", "Counter", "Total api-gateway JWKS refresh failures.")
	writePrometheusSample(builder, "nexusim_api_gateway_auth_jwks_refresh_failures_total", nil, strconv.FormatInt(snapshot.RefreshFailures, 10))
	writePrometheusHeader(builder, "nexusim_api_gateway_auth_jwks_last_refresh_success_unix_milliseconds", "Gauge", "Unix time of the last successful api-gateway JWKS refresh in milliseconds.")
	writePrometheusSample(builder, "nexusim_api_gateway_auth_jwks_last_refresh_success_unix_milliseconds", nil, strconv.FormatInt(snapshot.LastRefreshSuccess, 10))
	writePrometheusHeader(builder, "nexusim_api_gateway_auth_jwks_last_refresh_failure_unix_milliseconds", "Gauge", "Unix time of the last failed api-gateway JWKS refresh in milliseconds.")
	writePrometheusSample(builder, "nexusim_api_gateway_auth_jwks_last_refresh_failure_unix_milliseconds", nil, strconv.FormatInt(snapshot.LastRefreshFailure, 10))
}

func writeRateLimitPrometheus(builder *strings.Builder, snapshot ratelimit.Snapshot) {
	labels := map[string]string{
		"backend":            snapshot.Backend,
		"key_scope":          snapshot.KeyScope,
		"tenant_plan_source": snapshot.TenantPlanSource,
	}
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_enabled", "Gauge", "Whether api-gateway rate limiting is enabled.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_enabled", labels, prometheusBool(snapshot.Enabled))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_accepted_total", "Counter", "Total api-gateway requests accepted by the rate limiter.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_accepted_total", labels, strconv.FormatInt(snapshot.TotalAccepted, 10))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_limited_total", "Counter", "Total api-gateway requests rejected by the rate limiter.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_limited_total", labels, strconv.FormatInt(snapshot.TotalLimited, 10))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_redis_errors_total", "Counter", "Total api-gateway Redis rate-limit backend errors.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_redis_errors_total", labels, strconv.FormatInt(snapshot.RedisErrors, 10))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_identity_errors_total", "Counter", "Total api-gateway tenant identity resolution errors in rate limiting.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_identity_errors_total", labels, strconv.FormatInt(snapshot.IdentityErrors, 10))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tenant_plan_count", "Gauge", "Number of configured api-gateway tenant rate-limit plan overrides.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tenant_plan_count", labels, strconv.Itoa(snapshot.TenantPlans))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tenant_plan_reload_total", "Counter", "Total successful api-gateway tenant rate-limit plan reloads.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tenant_plan_reload_total", labels, strconv.FormatInt(snapshot.TenantReloads, 10))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tenant_plan_reload_errors_total", "Counter", "Total failed api-gateway tenant rate-limit plan reloads.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tenant_plan_reload_errors_total", labels, strconv.FormatInt(snapshot.TenantErrors, 10))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tenant_plan_require_checksum", "Gauge", "Whether api-gateway requires tenant rate-limit plan snapshots to carry a valid checksum.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tenant_plan_require_checksum", labels, prometheusBool(snapshot.TenantPlanRequireChecksum))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tenant_plan_url_bearer_token_configured", "Gauge", "Whether api-gateway tenant rate-limit URL source has a bearer token configured.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tenant_plan_url_bearer_token_configured", labels, prometheusBool(snapshot.TenantPlanURLBearerSet))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tenant_plan_url_require_https", "Gauge", "Whether api-gateway tenant rate-limit URL source requires HTTPS.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tenant_plan_url_require_https", labels, prometheusBool(snapshot.TenantPlanURLRequireHTTPS))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tenant_plan_url_tls_configured", "Gauge", "Whether api-gateway tenant rate-limit URL source has custom TLS configuration.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tenant_plan_url_tls_configured", labels, prometheusBool(snapshot.TenantPlanURLTLSConfigured))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tenant_plan_url_client_cert_configured", "Gauge", "Whether api-gateway tenant rate-limit URL source has a client certificate configured.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tenant_plan_url_client_cert_configured", labels, prometheusBool(snapshot.TenantPlanURLClientCertSet))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tenant_plan_generated_at_unix_milliseconds", "Gauge", "Unix time when the applied api-gateway tenant rate-limit plan snapshot was generated.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tenant_plan_generated_at_unix_milliseconds", labels, strconv.FormatInt(snapshot.TenantPlanGeneratedAt, 10))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tenant_plan_max_age_milliseconds", "Gauge", "Configured maximum acceptable age for the applied api-gateway tenant rate-limit plan snapshot; 0 means disabled.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tenant_plan_max_age_milliseconds", labels, strconv.FormatInt(snapshot.TenantPlanMaxAgeMS, 10))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tenant_plan_age_milliseconds", "Gauge", "Current age of the applied api-gateway tenant rate-limit plan snapshot.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tenant_plan_age_milliseconds", labels, strconv.FormatInt(snapshot.TenantPlanAgeMS, 10))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tenant_plan_stale", "Gauge", "Whether the applied api-gateway tenant rate-limit plan snapshot is older than its configured max age.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tenant_plan_stale", labels, prometheusBool(snapshot.TenantPlanStale))
	writePrometheusHeader(builder, "nexusim_api_gateway_rate_limit_tracked_keys", "Gauge", "Number of api-gateway local rate-limit keys currently tracked.")
	writePrometheusSample(builder, "nexusim_api_gateway_rate_limit_tracked_keys", labels, strconv.Itoa(snapshot.TrackedKeys))
}

func writeRuntimePrometheus(builder *strings.Builder, snapshot RuntimeSnapshot) {
	writePrometheusHeader(builder, "nexusim_api_gateway_legacy_descriptors_registered", "Gauge", "Whether api-gateway legacy descriptors are registered.")
	writePrometheusSample(builder, "nexusim_api_gateway_legacy_descriptors_registered", nil, prometheusBool(snapshot.RegisterLegacyDescriptors))
	writePrometheusHeader(builder, "nexusim_api_gateway_legacy_descriptors_allowed_until_unix_milliseconds", "Gauge", "Configured expiration time for api-gateway legacy descriptor opt-in in milliseconds; 0 means unset.")
	writePrometheusSample(builder, "nexusim_api_gateway_legacy_descriptors_allowed_until_unix_milliseconds", nil, strconv.FormatInt(snapshot.LegacyDescriptorsAllowedUntilMS, 10))
}

func writeTracePrometheus(builder *strings.Builder, snapshot TraceSnapshot) {
	labels := map[string]string{"exporter": snapshot.Exporter}
	writePrometheusHeader(builder, "nexusim_api_gateway_otel_traces_enabled", "Gauge", "Whether api-gateway OpenTelemetry tracing is enabled.")
	writePrometheusSample(builder, "nexusim_api_gateway_otel_traces_enabled", labels, prometheusBool(snapshot.Enabled))
	writePrometheusHeader(builder, "nexusim_api_gateway_otel_traces_otlp_endpoint_configured", "Gauge", "Whether api-gateway OpenTelemetry OTLP endpoint is configured.")
	writePrometheusSample(builder, "nexusim_api_gateway_otel_traces_otlp_endpoint_configured", labels, prometheusBool(snapshot.OTLPEndpointSet))
	writePrometheusHeader(builder, "nexusim_api_gateway_otel_traces_otlp_insecure", "Gauge", "Whether api-gateway OpenTelemetry OTLP transport is configured as insecure.")
	writePrometheusSample(builder, "nexusim_api_gateway_otel_traces_otlp_insecure", labels, prometheusBool(snapshot.OTLPInsecure))
	writePrometheusHeader(builder, "nexusim_api_gateway_otel_traces_sampling_ratio", "Gauge", "api-gateway OpenTelemetry trace sampling ratio.")
	writePrometheusSample(builder, "nexusim_api_gateway_otel_traces_sampling_ratio", labels, strconv.FormatFloat(snapshot.SamplingRatio, 'f', -1, 64))
}

func writePrometheusHeader(builder *strings.Builder, name string, typ string, help string) {
	fmt.Fprintf(builder, "# HELP %s %s\n", name, help)
	fmt.Fprintf(builder, "# TYPE %s %s\n", name, strings.ToLower(typ))
}

func writePrometheusSample(builder *strings.Builder, name string, labels map[string]string, value string) {
	builder.WriteString(name)
	if len(labels) > 0 {
		builder.WriteString("{")
		keys := make([]string, 0, len(labels))
		for key := range labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for index, key := range keys {
			if index > 0 {
				builder.WriteString(",")
			}
			builder.WriteString(key)
			builder.WriteString("=\"")
			builder.WriteString(prometheusEscapeLabelValue(labels[key]))
			builder.WriteString("\"")
		}
		builder.WriteString("}")
	}
	builder.WriteString(" ")
	builder.WriteString(value)
	builder.WriteString("\n")
}

func prometheusEscapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
}

func prometheusBool(value bool) string {
	if value {
		return "1"
	}
	return "0"
}
