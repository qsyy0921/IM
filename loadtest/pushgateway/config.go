package main

import (
	"flag"
	"strings"
	"time"
)

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.conversationTarget, "conversation-target", "127.0.0.1:11596", "conversation-service gRPC target")
	flag.StringVar(&cfg.messageTarget, "message-target", "127.0.0.1:11595", "message-service gRPC target")
	flag.StringVar(&cfg.deliveryTarget, "delivery-target", "127.0.0.1:11597", "delivery-service gRPC target")
	flag.StringVar(&cfg.identityTarget, "identity-target", "", "optional identity-service gRPC target used to issue push gateway HMAC tokens")
	registerTLSFlags("conversation-tls", "NEXUSIM_CONVERSATION_TLS", "conversation-service", &cfg.conversationTLS)
	registerTLSFlags("message-tls", "NEXUSIM_MESSAGE_TLS", "message-service", &cfg.messageTLS)
	registerTLSFlags("delivery-tls", "NEXUSIM_DELIVERY_TLS", "delivery-service", &cfg.deliveryTLS)
	registerTLSFlags("identity-tls", "NEXUSIM_IDENTITY_TLS", "identity-service", &cfg.identityTLS)
	registerTLSFlags("push-tls", "NEXUSIM_PUSH_WS_TLS", "push-gateway WebSocket", &cfg.pushTLS)
	flag.StringVar(&cfg.pushURL, "push-url", "ws://127.0.0.1:11598", "push-gateway WebSocket URL")
	flag.StringVar(&cfg.reconnectPushURL, "reconnect-push-url", "", "optional WebSocket URL used for reconnect/resume scenarios")
	flag.StringVar(&cfg.resultDir, "result-dir", "H:/NexusIM/loadtest-results/push-gateway-smoke", "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "wait timeout for async chain")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-push-smoke", "tenant id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "conv-push-smoke", "conversation id")
	flag.StringVar(&cfg.ownerUserID, "owner-user-id", "owner-1", "owner/sender user id")
	flag.StringVar(&cfg.receiverUserID, "receiver-user-id", "push-user-1", "online receiver user id")
	flag.StringVar(&cfg.receiverDeviceID, "receiver-device-id", "push-device-1", "online receiver device id")
	var receiverDeviceIDs string
	flag.StringVar(&receiverDeviceIDs, "receiver-device-ids", "", "comma separated online receiver device ids; overrides receiver-device-id when set")
	flag.StringVar(&cfg.scenario, "scenario", "full", "scenario: full, message-change-notify, resume-replay, redis-resume-negative, cross-instance-resume, slow-client, redis-fault, redis-cluster-node-stop, redis-cluster-failover, redis-sentinel-failover, redis-sentinel-master-stop, redis-sentinel-quorum-loss, redis-sentinel-network-partition, or identity-revoke")
	flag.IntVar(&cfg.messageCount, "message-count", 1, "number of messages sent in the full online notify scenario")
	flag.IntVar(&cfg.slowMessageCount, "slow-message-count", 128, "number of messages sent while slow client does not read")
	flag.StringVar(&cfg.messageChangeAction, "message-change-action", "edit", "message-change-notify action: edit, revoke, or delete")
	flag.StringVar(&cfg.pushMetricsURL, "push-metrics-url", "", "push-gateway debug metrics URL")
	flag.StringVar(&cfg.reconnectMetricsURL, "reconnect-push-metrics-url", "", "optional debug metrics URL for reconnect/resume gateway")
	flag.StringVar(&cfg.consumerMetricsURL, "consumer-push-metrics-url", "", "optional debug metrics URL for delivery-consumer gateway")
	flag.StringVar(&cfg.routeBackend, "route-backend", "", "push route backend used by the smoke environment")
	flag.StringVar(&cfg.pushAuthMode, "push-auth-mode", "mock", "push-gateway auth mode used by the smoke environment: mock, hmac, or jwt")
	flag.StringVar(&cfg.pushAuthHMACSecret, "push-auth-hmac-secret", "", "HMAC secret used to sign push gateway smoke tokens when --push-auth-mode=hmac")
	flag.StringVar(&cfg.pushAuthHMACPreviousSecrets, "push-auth-hmac-previous-secrets", "", "comma separated previous HMAC secrets configured on push-gateway during rotation smoke; used only for summary evidence")
	flag.StringVar(&cfg.pushAuthTokenSigningSecret, "push-auth-token-signing-secret", "", "optional HMAC secret used only for signing smoke tokens; defaults to --push-auth-hmac-secret")
	flag.DurationVar(&cfg.pushAuthTokenTTL, "push-auth-token-ttl", 10*time.Minute, "TTL for generated push gateway HMAC smoke tokens")
	flag.StringVar(&cfg.identityGatewayTokenFormat, "identity-gateway-token-format", "legacy", "identity-service gateway token format used by smoke environment: legacy, jwt, or jwt-rs256")
	flag.StringVar(&cfg.identityTokenMethod, "identity-token-method", "issue_gateway_token", "identity-service token method: issue_gateway_token, login, or register_login")
	flag.StringVar(&cfg.identityLoginPassword, "identity-login-password", "push-smoke-password", "password used when --identity-token-method=login or register_login")
	flag.StringVar(&cfg.redisKeyPrefix, "redis-key-prefix", "", "Redis route key prefix used by the smoke environment")
	flag.StringVar(&cfg.pushWSGatewayID, "push-ws-gateway-id", "", "WebSocket gateway id used by cross-instance route smoke")
	flag.StringVar(&cfg.pushReconnectGatewayID, "push-reconnect-gateway-id", "", "reconnect WebSocket gateway id used by cross-instance resume smoke")
	flag.StringVar(&cfg.pushConsumerGatewayID, "push-consumer-gateway-id", "", "delivery consumer gateway id used by cross-instance route smoke")
	flag.StringVar(&cfg.identityRevokeScope, "identity-revoke-scope", "device", "identity-revoke target scope: device or session")
	flag.StringVar(&cfg.redisFaultCommand, "redis-fault-command", "", "optional command executed after WebSocket route registration and before SendMessage in redis-fault scenario")
	flag.StringVar(&cfg.redisRestoreCommand, "redis-restore-command", "", "optional command executed after PullInbox recovery and before reconnect/ACK in redis-fault scenario")
	flag.BoolVar(&cfg.verifiedAuthMetadata, "verified-auth-metadata", envBool(false, "NEXUSIM_PUSHGATEWAY_LOADTEST_VERIFIED_AUTH_METADATA", "NEXUSIM_CONVERSATION_LOADTEST_VERIFIED_AUTH_METADATA", "NEXUSIM_MESSAGE_LOADTEST_VERIFIED_AUTH_METADATA", "NEXUSIM_DELIVERY_LOADTEST_VERIFIED_AUTH_METADATA"), "send gateway verified identity through user-facing gRPC metadata")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete existing rows for tenant before running")
	flag.Parse()
	cfg.receiverDeviceIDs = parseDeviceIDs(receiverDeviceIDs, cfg.receiverDeviceID)
	cfg.receiverDeviceID = cfg.receiverDeviceIDs[0]
	cfg.scenario = strings.TrimSpace(cfg.scenario)
	if cfg.scenario == "" {
		cfg.scenario = "full"
	}
	cfg.messageChangeAction = strings.ToLower(strings.TrimSpace(cfg.messageChangeAction))
	if cfg.messageChangeAction == "" {
		cfg.messageChangeAction = "edit"
	}
	if cfg.messageCount <= 0 {
		cfg.messageCount = 1
	}
	cfg.identityRevokeScope = strings.ToLower(strings.TrimSpace(cfg.identityRevokeScope))
	if cfg.identityRevokeScope == "" {
		cfg.identityRevokeScope = "device"
	}
	if cfg.slowMessageCount <= 0 {
		cfg.slowMessageCount = 1
	}
	if cfg.pushMetricsURL == "" {
		cfg.pushMetricsURL = derivePushMetricsURL(cfg.pushURL)
	}
	if cfg.reconnectPushURL == "" {
		cfg.reconnectPushURL = cfg.pushURL
	}
	cfg.pushAuthMode = strings.ToLower(strings.TrimSpace(cfg.pushAuthMode))
	if cfg.pushAuthMode == "" {
		cfg.pushAuthMode = "mock"
	}
	normalizePushAuthConfig(&cfg)
	cfg.identityTokenMethod = normalizeIdentityTokenMethod(cfg.identityTokenMethod)
	if cfg.reconnectMetricsURL == "" && cfg.reconnectPushURL != cfg.pushURL {
		cfg.reconnectMetricsURL = derivePushMetricsURL(cfg.reconnectPushURL)
	}
	return cfg
}
