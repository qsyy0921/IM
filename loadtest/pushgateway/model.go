package main

import (
	"time"

	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
)

const (
	opClientHello    = "client.hello"
	opClientPing     = "client.ping"
	opDeliveryAck    = "delivery.ack"
	opServerHello    = "server.hello"
	opServerPong     = "server.pong"
	opDeliveryNotify = "delivery.notify"
	opDeliveryAckOK  = "delivery.ack.ok"
	opResumeHint     = "server.resume_hint"
	opError          = "error"
)

const (
	metadataTenantID  = "x-nexusim-tenant-id"
	metadataUserID    = "x-nexusim-user-id"
	metadataDeviceID  = "x-nexusim-device-id"
	metadataSessionID = "x-nexusim-session-id"
	metadataTraceID   = "x-nexusim-trace-id"
	metadataRequestID = "x-nexusim-request-id"
)

type clientFrame struct {
	Op             string   `json:"op"`
	RequestID      string   `json:"request_id,omitempty"`
	DeviceID       string   `json:"device_id,omitempty"`
	ConversationID string   `json:"conversation_id,omitempty"`
	ReceivedSeq    int64    `json:"received_seq,omitempty"`
	ResumeToken    string   `json:"resume_token,omitempty"`
	LastReceived   []cursor `json:"last_received,omitempty"`
}

type serverFrame struct {
	Op              string `json:"op"`
	RequestID       string `json:"request_id,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	ResumeToken     string `json:"resume_token,omitempty"`
	EventID         string `json:"event_id,omitempty"`
	ConversationID  string `json:"conversation_id,omitempty"`
	ConversationSeq int64  `json:"conversation_seq,omitempty"`
	SourceEventID   string `json:"source_event_id,omitempty"`
	SourceEventType string `json:"source_event_type,omitempty"`
	MessageID       string `json:"message_id,omitempty"`
	PullRequired    bool   `json:"pull_required,omitempty"`
	LastReceivedSeq int64  `json:"last_received_seq,omitempty"`
	Code            string `json:"code,omitempty"`
	Message         string `json:"message,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Retryable       bool   `json:"retryable"`
}

type config struct {
	conversationTarget                 string
	messageTarget                      string
	deliveryTarget                     string
	identityTarget                     string
	conversationTLS                    grpctls.Config
	messageTLS                         grpctls.Config
	deliveryTLS                        grpctls.Config
	identityTLS                        grpctls.Config
	pushTLS                            grpctls.Config
	pushURL                            string
	reconnectPushURL                   string
	resultDir                          string
	pgDSN                              string
	requestTimeout                     time.Duration
	waitTimeout                        time.Duration
	pollInterval                       time.Duration
	tenantID                           string
	conversationID                     string
	ownerUserID                        string
	receiverUserID                     string
	receiverDeviceID                   string
	receiverDeviceIDs                  []string
	scenario                           string
	slowMessageCount                   int
	pushMetricsURL                     string
	reconnectMetricsURL                string
	consumerMetricsURL                 string
	routeBackend                       string
	pushAuthMode                       string
	pushAuthHMACSecret                 string
	pushAuthHMACPreviousSecrets        string
	pushAuthTokenSigningSecret         string
	pushAuthTokenSigningSecretExplicit bool
	pushAuthTokenTTL                   time.Duration
	identityGatewayTokenFormat         string
	identityTokenMethod                string
	identityLoginPassword              string
	redisKeyPrefix                     string
	pushWSGatewayID                    string
	pushReconnectGatewayID             string
	pushConsumerGatewayID              string
	identityRevokeScope                string
	messageChangeAction                string
	redisFaultCommand                  string
	redisRestoreCommand                string
	verifiedAuthMetadata               bool
	cleanup                            bool
}

type summary struct {
	Commit                                  string                      `json:"commit"`
	CommitFull                              string                      `json:"commit_full"`
	GitDirty                                bool                        `json:"git_dirty"`
	GitStatusShort                          string                      `json:"git_status_short,omitempty"`
	ConversationTarget                      string                      `json:"conversation_target"`
	MessageTarget                           string                      `json:"message_target"`
	DeliveryTarget                          string                      `json:"delivery_target"`
	IdentityTarget                          string                      `json:"identity_target,omitempty"`
	ConversationTLSEnabled                  bool                        `json:"conversation_tls_enabled"`
	MessageTLSEnabled                       bool                        `json:"message_tls_enabled"`
	DeliveryTLSEnabled                      bool                        `json:"delivery_tls_enabled"`
	IdentityTLSEnabled                      bool                        `json:"identity_tls_enabled"`
	PushTLSEnabled                          bool                        `json:"push_tls_enabled"`
	VerifiedAuthMetadata                    bool                        `json:"verified_auth_metadata"`
	PushURL                                 string                      `json:"push_url"`
	ReconnectPushURL                        string                      `json:"reconnect_push_url,omitempty"`
	PushMetricsURL                          string                      `json:"push_metrics_url,omitempty"`
	ReconnectPushMetricsURL                 string                      `json:"reconnect_push_metrics_url,omitempty"`
	PushConsumerMetricsURL                  string                      `json:"push_consumer_metrics_url,omitempty"`
	RouteBackend                            string                      `json:"route_backend,omitempty"`
	PushAuthMode                            string                      `json:"push_auth_mode,omitempty"`
	PushAuthTokenTransport                  string                      `json:"push_auth_token_transport,omitempty"`
	PushAuthTokenSource                     string                      `json:"push_auth_token_source,omitempty"`
	IdentityGatewayTokenFormat              string                      `json:"identity_gateway_token_format,omitempty"`
	IdentityTokenMethod                     string                      `json:"identity_token_method,omitempty"`
	PushAuthTokenTTLSeconds                 int64                       `json:"push_auth_token_ttl_seconds,omitempty"`
	PushAuthSecretConfigured                bool                        `json:"push_auth_hmac_secret_configured"`
	PushAuthPreviousSecretsConfigured       bool                        `json:"push_auth_hmac_previous_secrets_configured"`
	PushAuthTokenSigningSecretExplicit      bool                        `json:"push_auth_token_signing_secret_explicit"`
	PushAuthTokenSignedWithNonCurrentSecret bool                        `json:"push_auth_token_signed_with_non_current_secret"`
	PushAuthQueryIdentitySent               bool                        `json:"push_auth_query_identity_sent"`
	RedisKeyPrefix                          string                      `json:"redis_key_prefix,omitempty"`
	PushWSGatewayID                         string                      `json:"push_ws_gateway_id,omitempty"`
	PushReconnectGatewayID                  string                      `json:"push_reconnect_gateway_id,omitempty"`
	PushConsumerGatewayID                   string                      `json:"push_consumer_gateway_id,omitempty"`
	Scenario                                string                      `json:"scenario"`
	TenantID                                string                      `json:"tenant_id"`
	ConversationID                          string                      `json:"conversation_id"`
	OwnerUserID                             string                      `json:"owner_user_id"`
	ReceiverUserID                          string                      `json:"receiver_user_id"`
	ReceiverDeviceID                        string                      `json:"receiver_device_id"`
	ReceiverDeviceIDs                       []string                    `json:"receiver_device_ids,omitempty"`
	StartedAt                               time.Time                   `json:"started_at"`
	FinishedAt                              time.Time                   `json:"finished_at"`
	Success                                 bool                        `json:"success"`
	Error                                   string                      `json:"error,omitempty"`
	ServerHello                             frameSnapshot               `json:"server_hello"`
	MemberJoin                              memberJoinSummary           `json:"member_join"`
	SendMessage                             sendSummary                 `json:"send_message"`
	MessageChange                           messageChangeSummary        `json:"message_change,omitempty"`
	DeliveryNotify                          frameSnapshot               `json:"delivery_notify"`
	ChangeDeliveryNotify                    frameSnapshot               `json:"change_delivery_notify,omitempty"`
	DeviceNotifications                     []deviceSummary             `json:"device_notifications,omitempty"`
	PullInbox                               pullSummary                 `json:"pull_inbox"`
	ChangePullInbox                         pullSummary                 `json:"change_pull_inbox,omitempty"`
	DeliveryAckOK                           frameSnapshot               `json:"delivery_ack_ok"`
	SlowClient                              *slowClientSummary          `json:"slow_client,omitempty"`
	ResumeReplay                            *resumeReplaySummary        `json:"resume_replay,omitempty"`
	RedisResumeNegative                     *redisResumeNegativeSummary `json:"redis_resume_negative,omitempty"`
	RedisFault                              *redisFaultSummary          `json:"redis_fault,omitempty"`
	IdentityRevoke                          *identityRevokeSummary      `json:"identity_revoke,omitempty"`
	PushMetricsBefore                       *pushMetrics                `json:"push_metrics_before,omitempty"`
	PushMetricsAfter                        *pushMetrics                `json:"push_metrics_after,omitempty"`
	PushConsumerMetrics                     *pushMetrics                `json:"push_consumer_metrics,omitempty"`
	CursorLastReceivedSeq                   *int64                      `json:"cursor_last_received_seq,omitempty"`
	UserInboxCount                          *int64                      `json:"user_inbox_count,omitempty"`
	DeliveryOutboxTotal                     *int64                      `json:"delivery_outbox_total,omitempty"`
	DeliveryOutboxPending                   *int64                      `json:"delivery_outbox_pending,omitempty"`
	DeliveryOutboxPublished                 *int64                      `json:"delivery_outbox_published,omitempty"`
	DeliveryOutboxDLQ                       *int64                      `json:"delivery_outbox_dlq,omitempty"`
	Capacity                                *capacitySummary            `json:"capacity_summary,omitempty"`
	Latencies                               map[string]float64          `json:"latencies_ms"`
}

type capacitySummary struct {
	DurationMS              float64 `json:"duration_ms"`
	DeviceCount             int     `json:"device_count"`
	MessageCount            int     `json:"message_count"`
	NotifyFrameCount        int     `json:"notify_frame_count"`
	AckFrameCount           int     `json:"ack_frame_count"`
	PullInboxItemCount      int     `json:"pull_inbox_item_count"`
	DeliveryOutboxPublished int64   `json:"delivery_outbox_published"`
	MessagesPerSecond       float64 `json:"messages_per_second"`
	NotifyFramesPerSecond   float64 `json:"notify_frames_per_second"`
	AckFramesPerSecond      float64 `json:"ack_frames_per_second"`
	PullItemsPerSecond      float64 `json:"pull_items_per_second"`
}

type deviceSummary struct {
	DeviceID              string        `json:"device_id"`
	ServerHello           frameSnapshot `json:"server_hello"`
	DeliveryNotify        frameSnapshot `json:"delivery_notify"`
	DeliveryAckOK         frameSnapshot `json:"delivery_ack_ok"`
	CursorLastReceivedSeq *int64        `json:"cursor_last_received_seq,omitempty"`
}

type slowClientSummary struct {
	MessageCount       int           `json:"message_count"`
	FirstSeq           int64         `json:"first_seq"`
	LastSeq            int64         `json:"last_seq"`
	NotifyFramesRead   int           `json:"notify_frames_read"`
	ResumeHintReceived bool          `json:"resume_hint_received"`
	ResumeHint         frameSnapshot `json:"resume_hint,omitempty"`
	CloseStatus        string        `json:"close_status,omitempty"`
	ReconnectedHello   frameSnapshot `json:"reconnected_hello"`
	ReplayFramesRead   int           `json:"replay_frames_read"`
	RecoveryPullInbox  pullSummary   `json:"recovery_pull_inbox"`
	AckOK              frameSnapshot `json:"ack_ok"`
}

type resumeReplaySummary struct {
	OriginalHello       frameSnapshot `json:"original_hello"`
	OriginalNotify      frameSnapshot `json:"original_notify"`
	ReconnectedHello    frameSnapshot `json:"reconnected_hello"`
	ReplayedNotify      frameSnapshot `json:"replayed_notify"`
	LastReceivedSeq     int64         `json:"last_received_seq"`
	ReplayMetricsBefore *pushMetrics  `json:"replay_metrics_before,omitempty"`
	ReplayMetricsAfter  *pushMetrics  `json:"replay_metrics_after,omitempty"`
	PullInbox           pullSummary   `json:"pull_inbox"`
	AckOK               frameSnapshot `json:"ack_ok"`
}

type redisResumeNegativeSummary struct {
	UnknownRequestedToken string        `json:"unknown_requested_token"`
	UnknownHello          frameSnapshot `json:"unknown_hello"`
	UnknownHint           frameSnapshot `json:"unknown_hint"`
	PermissionDenied      frameSnapshot `json:"permission_denied"`
	GapHello              frameSnapshot `json:"gap_hello"`
	GapHint               frameSnapshot `json:"gap_hint"`
	GapMessageCount       int           `json:"gap_message_count"`
	FirstSeq              int64         `json:"first_seq"`
	LastSeq               int64         `json:"last_seq"`
	OriginalNotify        frameSnapshot `json:"original_notify"`
	LastNotify            frameSnapshot `json:"last_notify"`
	RecoveryPullInbox     pullSummary   `json:"recovery_pull_inbox"`
	AckOK                 frameSnapshot `json:"ack_ok"`
	SkippedFramesWhileAck int           `json:"skipped_frames_while_ack"`
	MetricsBefore         *pushMetrics  `json:"metrics_before,omitempty"`
	MetricsAfter          *pushMetrics  `json:"metrics_after,omitempty"`
}

type redisFaultSummary struct {
	FaultCommand        string        `json:"fault_command"`
	CommandOutput       string        `json:"command_output,omitempty"`
	NotifyReceived      bool          `json:"notify_received"`
	UnexpectedNotify    frameSnapshot `json:"unexpected_notify,omitempty"`
	NotifyWaitError     string        `json:"notify_wait_error,omitempty"`
	RecoveryPullInbox   pullSummary   `json:"recovery_pull_inbox"`
	AckOK               frameSnapshot `json:"ack_ok"`
	DeliveryOutboxTotal int64         `json:"delivery_outbox_total"`
}

type identityRevokeSummary struct {
	InitialHello           frameSnapshot `json:"initial_hello"`
	Scope                  string        `json:"scope"`
	RevokedDeviceID        string        `json:"revoked_device_id"`
	RevokedSessionID       string        `json:"revoked_session_id,omitempty"`
	ActiveCloseHint        frameSnapshot `json:"active_close_hint"`
	ActiveCloseStatus      string        `json:"active_close_status,omitempty"`
	ActiveNotifyFramesRead int           `json:"active_notify_frames_read"`
	SurvivorHello          frameSnapshot `json:"survivor_hello,omitempty"`
	SurvivorPong           frameSnapshot `json:"survivor_pong,omitempty"`
	DeniedFrame            frameSnapshot `json:"denied_frame"`
	ReconnectAttempts      int           `json:"reconnect_attempts"`
}

type pushMetrics struct {
	ConnectedSessions           int               `json:"connected_sessions"`
	SessionQueueFullCount       uint64            `json:"session_queue_full_count"`
	SlowSessionEvictedCount     uint64            `json:"slow_session_evicted_count"`
	IdentitySessionEvictedCount uint64            `json:"identity_session_evicted_count"`
	ResumeBufferReplayCount     uint64            `json:"resume_buffer_replay_count"`
	ResumeBufferMissCount       uint64            `json:"resume_buffer_miss_count"`
	ResumeBufferStoredFrames    int               `json:"resume_buffer_stored_frames"`
	ResumeBufferTokenCount      int               `json:"resume_buffer_token_count"`
	ResumeBufferExpiredCount    uint64            `json:"resume_buffer_expired_count"`
	RedisRegistryMetrics        redisRouteMetrics `json:"redis_registry_metrics,omitempty"`
	RedisSubscriberMetrics      redisRouteMetrics `json:"redis_subscriber_metrics,omitempty"`
	AuthJWKMetrics              *authJWKMetrics   `json:"auth_jwks,omitempty"`
}

type authJWKMetrics struct {
	RemoteURLConfigured bool  `json:"remote_url_configured"`
	CachedKeyCount      int   `json:"cached_key_count"`
	LastRefreshSuccess  int64 `json:"last_refresh_success_ms,omitempty"`
	LastRefreshFailure  int64 `json:"last_refresh_failure_ms,omitempty"`
	RefreshFailures     int64 `json:"refresh_failure_count"`
}

type redisRouteMetrics struct {
	RedisRouteRegisterErrorCount       uint64 `json:"redis_route_register_error_count"`
	RedisRouteRenewErrorCount          uint64 `json:"redis_route_renew_error_count"`
	RedisRouteLookupErrorCount         uint64 `json:"redis_route_lookup_error_count"`
	RedisRouteRemoteMatchedSessions    uint64 `json:"redis_route_remote_matched_sessions"`
	RedisRouteRemotePublishCallCount   uint64 `json:"redis_route_remote_publish_call_count"`
	RedisRouteRemotePublishErrorCount  uint64 `json:"redis_route_remote_publish_error_count"`
	RedisRouteRemoteNoSubscriberCount  uint64 `json:"redis_route_remote_no_subscriber_count"`
	RedisRouteRemoteEnqueuedSessions   uint64 `json:"redis_route_remote_enqueued_sessions"`
	RedisRouteStaleRemovedCount        uint64 `json:"redis_route_stale_removed_count"`
	RedisRouteCleanupErrorCount        uint64 `json:"redis_route_cleanup_error_count"`
	RedisRouteSubscriberMessageCount   uint64 `json:"redis_route_subscriber_message_count,omitempty"`
	RedisRouteSubscriberMalformedCount uint64 `json:"redis_route_subscriber_malformed_count,omitempty"`
	RedisRouteSubscriberEnqueuedCount  uint64 `json:"redis_route_subscriber_enqueued_count,omitempty"`
	RedisRouteSubscriberEvictedCount   uint64 `json:"redis_route_subscriber_evicted_count,omitempty"`
	RedisRouteSubscriberErrorCount     uint64 `json:"redis_route_subscriber_error_count,omitempty"`
	RedisResumeReplayCount             uint64 `json:"redis_resume_replay_count,omitempty"`
	RedisResumeMissCount               uint64 `json:"redis_resume_miss_count,omitempty"`
	RedisResumeAppendCount             uint64 `json:"redis_resume_append_count,omitempty"`
	RedisResumeAppendErrorCount        uint64 `json:"redis_resume_append_error_count,omitempty"`
	RedisResumePermissionDeniedCount   uint64 `json:"redis_resume_permission_denied_count,omitempty"`
}

type frameSnapshot struct {
	Op              string `json:"op"`
	RequestID       string `json:"request_id,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	ResumeToken     string `json:"resume_token,omitempty"`
	EventID         string `json:"event_id,omitempty"`
	ConversationID  string `json:"conversation_id,omitempty"`
	ConversationSeq int64  `json:"conversation_seq,omitempty"`
	SourceEventID   string `json:"source_event_id,omitempty"`
	SourceEventType string `json:"source_event_type,omitempty"`
	MessageID       string `json:"message_id,omitempty"`
	PullRequired    bool   `json:"pull_required,omitempty"`
	LastReceivedSeq int64  `json:"last_received_seq,omitempty"`
	Code            string `json:"code,omitempty"`
	Message         string `json:"message,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Retryable       bool   `json:"retryable,omitempty"`
}

type memberJoinSummary struct {
	ChangeID          string `json:"change_id"`
	BoundarySeq       int64  `json:"boundary_seq"`
	MemberVersion     int64  `json:"member_version"`
	PermissionVersion int64  `json:"permission_version"`
}

type sendSummary struct {
	MessageID       string `json:"message_id"`
	ConversationSeq int64  `json:"conversation_seq"`
}

type messageChangeSummary struct {
	Action          string `json:"action,omitempty"`
	MessageID       string `json:"message_id,omitempty"`
	ConversationSeq int64  `json:"conversation_seq,omitempty"`
	ChangeVersion   int32  `json:"change_version,omitempty"`
	SourceEventType string `json:"source_event_type,omitempty"`
}

type pullSummary struct {
	ItemCount int     `json:"item_count"`
	MaxSeq    int64   `json:"max_seq"`
	Items     []item  `json:"items"`
	P95MS     float64 `json:"p95_ms"`
	P99MS     float64 `json:"p99_ms"`
}

type item struct {
	ConversationSeq int64  `json:"conversation_seq"`
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	MessageID       string `json:"message_id"`
	SenderID        string `json:"sender_id"`
}

type cursor struct {
	ConversationID string `json:"conversation_id"`
	Seq            int64  `json:"seq"`
}

type verifiedAuthIdentity struct {
	tenantID  string
	userID    string
	deviceID  string
	sessionID string
	traceID   string
	requestID string
}
