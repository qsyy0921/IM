package main

import "time"

type userRole string

const (
	roleOwner    userRole = "owner"
	roleSender   userRole = "sender"
	roleReceiver userRole = "receiver"
)

type onlineMode string

const (
	onlineFast onlineMode = "online_fast"
	onlineSlow onlineMode = "online_slow"
	offline    onlineMode = "offline"
)

type loadUser struct {
	UserID     string     `json:"user_id"`
	DeviceID   string     `json:"device_id"`
	SessionID  string     `json:"session_id"`
	Role       userRole   `json:"role"`
	OnlineMode onlineMode `json:"online_mode"`
}

type userPlan struct {
	TenantID       string     `json:"tenant_id"`
	ConversationID string     `json:"conversation_id"`
	GroupSize      int        `json:"group_size"`
	Owner          loadUser   `json:"owner"`
	Senders        []loadUser `json:"senders"`
	Receivers      []loadUser `json:"receivers"`
	OnlineFast     int        `json:"online_fast"`
	OnlineSlow     int        `json:"online_slow"`
	Offline        int        `json:"offline"`
}

type sendStats struct {
	SuccessCount int       `json:"success_count"`
	ErrorCount   int       `json:"error_count"`
	LatencyAvgMS float64   `json:"latency_avg_ms"`
	LatencyP95MS float64   `json:"latency_p95_ms"`
	LatencyP99MS float64   `json:"latency_p99_ms"`
	MaxSeq       int64     `json:"max_seq"`
	Errors       []string  `json:"errors,omitempty"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	FinishedAt   time.Time `json:"finished_at,omitempty"`
}

type receiverStats struct {
	SampledReceivers int      `json:"sampled_receivers"`
	PullSuccessCount int      `json:"pull_success_count"`
	PullErrorCount   int      `json:"pull_error_count"`
	AckSuccessCount  int      `json:"ack_success_count"`
	AckErrorCount    int      `json:"ack_error_count"`
	MaxPulledSeq     int64    `json:"max_pulled_seq"`
	PullLatencyAvgMS float64  `json:"pull_latency_avg_ms"`
	PullLatencyP95MS float64  `json:"pull_latency_p95_ms"`
	PullLatencyP99MS float64  `json:"pull_latency_p99_ms"`
	Errors           []string `json:"errors,omitempty"`
}

type pushStats struct {
	Enabled                       bool                        `json:"enabled"`
	PushURL                       string                      `json:"push_url,omitempty"`
	SubscriberTotalCount          int                         `json:"subscriber_total_count"`
	SubscriberShardCount          int                         `json:"subscriber_shard_count"`
	SubscriberShardIndex          int                         `json:"subscriber_shard_index"`
	SubscriberCount               int                         `json:"subscriber_count"`
	SubscribeSuccessCount         int                         `json:"subscribe_success_count"`
	SubscribeErrorCount           int                         `json:"subscribe_error_count"`
	ConversationSignalSampleEvery int                         `json:"conversation_signal_sample_every"`
	ExpectedSignalsPerSubscriber  int                         `json:"expected_signals_per_subscriber"`
	ExpectedConversationSignals   int                         `json:"expected_conversation_signals"`
	ConversationSignalCount       int                         `json:"conversation_signal_count"`
	MaxConversationSeq            int64                       `json:"max_conversation_seq"`
	SubscriberSignals             []pushSignalSubscriberStats `json:"subscriber_signals,omitempty"`
	Errors                        []string                    `json:"errors,omitempty"`
	StartedAt                     time.Time                   `json:"started_at,omitempty"`
	FinishedAt                    time.Time                   `json:"finished_at,omitempty"`
}

type pushSignalSubscriberStats struct {
	UserID             string  `json:"user_id"`
	DeviceID           string  `json:"device_id"`
	SignalCount        int     `json:"signal_count"`
	MaxConversationSeq int64   `json:"max_conversation_seq"`
	FirstSignalAfterMS float64 `json:"first_signal_after_ms,omitempty"`
	LastSignalAfterMS  float64 `json:"last_signal_after_ms,omitempty"`
	Completed          bool    `json:"completed"`
	Error              string  `json:"error,omitempty"`
}

type postgresStats struct {
	ConversationMemberCount       int64  `json:"conversation_member_count"`
	DeliveryMembershipActiveCount int64  `json:"delivery_membership_active_count"`
	ConversationMode              string `json:"conversation_mode"`
	FanoutMode                    string `json:"fanout_mode"`
	FanoutPolicyVersion           int64  `json:"fanout_policy_version"`
	MessageLogCount               int64  `json:"message_log_count"`
	DeliveryTimelineRows          int64  `json:"delivery_timeline_rows"`
	UserInboxRows                 int64  `json:"user_inbox_rows"`
	DeliveryOutboxRows            int64  `json:"delivery_outbox_rows"`
	MessageOutboxPending          int64  `json:"message_outbox_pending"`
	MessageOutboxDLQ              int64  `json:"message_outbox_dlq"`
	DeliveryOutboxPending         int64  `json:"delivery_outbox_pending"`
	DeliveryOutboxDLQ             int64  `json:"delivery_outbox_dlq"`
}

type summary struct {
	SchemaVersion              int            `json:"schema_version"`
	RunName                    string         `json:"run_name"`
	Commit                     string         `json:"commit"`
	GitDirty                   bool           `json:"git_dirty"`
	GitStatusShort             string         `json:"git_status_short,omitempty"`
	RunnerMode                 string         `json:"runner_mode"`
	DryRun                     bool           `json:"dry_run"`
	VerifiedAuthMetadata       bool           `json:"verified_auth_metadata"`
	TenantID                   string         `json:"tenant_id"`
	ConversationID             string         `json:"conversation_id"`
	GroupSize                  int            `json:"group_size"`
	SenderCount                int            `json:"sender_count"`
	OnlineRatio                float64        `json:"online_ratio"`
	SlowClientRatio            float64        `json:"slow_client_ratio"`
	ACKRatio                   float64        `json:"ack_ratio"`
	MessageRate                float64        `json:"message_rate"`
	DurationSeconds            float64        `json:"duration_seconds"`
	MessageCount               int            `json:"message_count"`
	ExpectedInboxRows          int64          `json:"expected_inbox_rows"`
	ExpectedTimelineRows       int64          `json:"expected_timeline_rows"`
	ActualFanoutMode           string         `json:"actual_fanout_mode,omitempty"`
	ExpectedFanoutMode         string         `json:"expected_fanout_mode,omitempty"`
	RequireDeliveryOutboxDrain bool           `json:"require_delivery_outbox_drain"`
	UserPlan                   userPlan       `json:"user_plan"`
	Send                       sendStats      `json:"send"`
	Push                       pushStats      `json:"push"`
	Receiver                   receiverStats  `json:"receiver"`
	Postgres                   *postgresStats `json:"postgres,omitempty"`
	Success                    bool           `json:"success"`
	Error                      string         `json:"error,omitempty"`
	StartedAt                  time.Time      `json:"started_at"`
	FinishedAt                 time.Time      `json:"finished_at"`
}
