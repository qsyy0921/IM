package main

import "time"

type summary struct {
	Commit                                   string                  `json:"commit"`
	CommitFull                               string                  `json:"commit_full"`
	GitDirty                                 bool                    `json:"git_dirty"`
	GitStatusShort                           string                  `json:"git_status_short,omitempty"`
	ConversationTarget                       string                  `json:"conversation_target"`
	MessageTarget                            string                  `json:"message_target"`
	DeliveryTarget                           string                  `json:"delivery_target"`
	ReceiptTarget                            string                  `json:"receipt_target"`
	ConversationTLSEnabled                   bool                    `json:"conversation_tls_enabled"`
	MessageTLSEnabled                        bool                    `json:"message_tls_enabled"`
	DeliveryTLSEnabled                       bool                    `json:"delivery_tls_enabled"`
	ReceiptTLSEnabled                        bool                    `json:"receipt_tls_enabled"`
	VerifiedAuthMetadata                     bool                    `json:"verified_auth_metadata"`
	ResultDir                                string                  `json:"result_dir"`
	TenantID                                 string                  `json:"tenant_id"`
	ConversationID                           string                  `json:"conversation_id"`
	OwnerUserID                              string                  `json:"owner_user_id"`
	ReceiverUserID                           string                  `json:"receiver_user_id"`
	ReceiverDeviceID                         string                  `json:"receiver_device_id"`
	DeliveryConsumerGroup                    string                  `json:"delivery_consumer_group,omitempty"`
	ReceiptConsumerGroup                     string                  `json:"receipt_consumer_group,omitempty"`
	ReceiptEventsTopic                       string                  `json:"receipt_events_topic,omitempty"`
	ReceiptEventsGroup                       string                  `json:"receipt_events_group,omitempty"`
	CapacityMode                             bool                    `json:"capacity_mode,omitempty"`
	VUs                                      int                     `json:"vus,omitempty"`
	ConfiguredDurationSeconds                float64                 `json:"configured_duration_seconds,omitempty"`
	StartedAt                                time.Time               `json:"started_at"`
	FinishedAt                               time.Time               `json:"finished_at"`
	Success                                  bool                    `json:"success"`
	Error                                    string                  `json:"error,omitempty"`
	MemberJoin                               memberJoinSummary       `json:"member_join"`
	SendMessage                              sendSummary             `json:"send_message"`
	PullInbox                                pullSummary             `json:"pull_inbox"`
	AckDelivery                              ackSummary              `json:"ack_delivery"`
	ReceiptBeforeReadBySeq                   receiptStateSummary     `json:"receipt_before_read_by_seq"`
	ConversationListBefore                   conversationListSummary `json:"conversation_list_before_read"`
	ConversationListUnreadBeforeRead         conversationListSummary `json:"conversation_list_unread_before_read"`
	ReceiptAfterReadBySeq                    receiptStateSummary     `json:"receipt_after_read_by_seq"`
	ReceiptAfterReadByMsgID                  receiptStateSummary     `json:"receipt_after_read_by_message_id"`
	ConversationListAfter                    conversationListSummary `json:"conversation_list_after_read"`
	ConversationListUnreadAfterRead          conversationListSummary `json:"conversation_list_unread_after_read"`
	ArchiveConversation                      archiveSummary          `json:"archive_conversation"`
	ConversationListArchivedDefault          conversationListSummary `json:"conversation_list_archived_default"`
	ConversationListArchivedIncluded         conversationListSummary `json:"conversation_list_archived_included"`
	SendMessageWhileArchived                 sendSummary             `json:"send_message_while_archived"`
	PullInboxWhileArchived                   pullSummary             `json:"pull_inbox_while_archived"`
	AckDeliveryWhileArchived                 ackSummary              `json:"ack_delivery_while_archived"`
	ConversationListAfterArchivedNewDefault  conversationListSummary `json:"conversation_list_after_archived_new_message_default"`
	ConversationListAfterArchivedNewIncluded conversationListSummary `json:"conversation_list_after_archived_new_message_included"`
	UnarchiveConversation                    archiveSummary          `json:"unarchive_conversation"`
	ConversationListAfterUnarchive           conversationListSummary `json:"conversation_list_after_unarchive"`
	PinConversation                          pinSummary              `json:"pin_conversation"`
	ConversationListAfterPin                 conversationListSummary `json:"conversation_list_after_pin"`
	UnpinConversation                        pinSummary              `json:"unpin_conversation"`
	ConversationListAfterUnpin               conversationListSummary `json:"conversation_list_after_unpin"`
	MuteConversation                         muteSummary             `json:"mute_conversation"`
	ConversationListAfterMute                conversationListSummary `json:"conversation_list_after_mute"`
	UnmuteConversation                       muteSummary             `json:"unmute_conversation"`
	ConversationListAfterUnmute              conversationListSummary `json:"conversation_list_after_unmute"`
	MarkRead                                 markReadSummary         `json:"mark_read"`
	MarkReadTooFar                           negativeCallSummary     `json:"mark_read_too_far"`
	ReceiptProjection                        receiptProjectionStats  `json:"receipt_projection"`
	ReceiptOutbox                            receiptOutboxStats      `json:"receipt_outbox"`
	ReceiptKafkaEvents                       []receiptKafkaEvent     `json:"receipt_kafka_events"`
	DeliveryOutbox                           outboxStats             `json:"delivery_outbox"`
	Capacity                                 *capacitySummary        `json:"capacity_summary,omitempty"`
	LatenciesMS                              map[string]float64      `json:"latencies_ms"`
	CapacityMessageCount                     int                     `json:"-"`
	CapacityPullItemCount                    int                     `json:"-"`
	CapacityAckCount                         int                     `json:"-"`
	CapacityMarkReadCount                    int                     `json:"-"`
	CapacityReceiptKafkaEventCount           int                     `json:"-"`
	CapacityErrorCount                       int                     `json:"-"`
	CapacityLatencySamplesMS                 []float64               `json:"-"`
}

type capacitySummary struct {
	DurationMS                float64 `json:"duration_ms"`
	MessageCount              int     `json:"message_count"`
	PullItemCount             int     `json:"pull_item_count"`
	AckCount                  int     `json:"ack_count"`
	MarkReadCount             int     `json:"mark_read_count"`
	ReceiptStateQueryCount    int     `json:"receipt_state_query_count"`
	ConversationListCallCount int     `json:"conversation_list_call_count"`
	StateMutationCount        int     `json:"state_mutation_count"`
	ReceiptKafkaEventCount    int     `json:"receipt_kafka_event_count"`
	ReceiptOutboxPublished    int64   `json:"receipt_outbox_published"`
	ReceiptOutboxPending      int64   `json:"receipt_outbox_pending"`
	ReceiptOutboxDLQ          int64   `json:"receipt_outbox_dlq"`
	DeliveryOutboxPublished   int64   `json:"delivery_outbox_published"`
	DeliveryOutboxPending     int64   `json:"delivery_outbox_pending"`
	DeliveryOutboxDLQ         int64   `json:"delivery_outbox_dlq"`
	OperationsPerSecond       float64 `json:"operations_per_second"`
	MessagesPerSecond         float64 `json:"messages_per_second"`
	ReceiptEventsPerSecond    float64 `json:"receipt_events_per_second"`
	ErrorCount                int     `json:"error_count,omitempty"`
	VUs                       int     `json:"vus,omitempty"`
	LatencyP95MS              float64 `json:"latency_p95_ms,omitempty"`
	LatencyP99MS              float64 `json:"latency_p99_ms,omitempty"`
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

type pullSummary struct {
	ItemCount int          `json:"item_count"`
	MaxSeq    int64        `json:"max_seq"`
	Items     []pulledItem `json:"items"`
	P95MS     float64      `json:"p95_ms"`
	P99MS     float64      `json:"p99_ms"`
}

type pulledItem struct {
	ConversationSeq int64  `json:"conversation_seq"`
	EventID         string `json:"event_id"`
	MessageID       string `json:"message_id"`
	SenderID        string `json:"sender_id"`
}

type ackSummary struct {
	LastReceivedSeq int64   `json:"last_received_seq"`
	LatencyMS       float64 `json:"latency_ms"`
}

type markReadSummary struct {
	LastReadSeq int64   `json:"last_read_seq"`
	LatencyMS   float64 `json:"latency_ms"`
}

type archiveSummary struct {
	Archived  bool    `json:"archived"`
	LatencyMS float64 `json:"latency_ms"`
}

type pinSummary struct {
	Pinned    bool    `json:"pinned"`
	LatencyMS float64 `json:"latency_ms"`
}

type muteSummary struct {
	Muted     bool    `json:"muted"`
	LatencyMS float64 `json:"latency_ms"`
}

type negativeCallSummary struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Passed  bool   `json:"passed"`
}

type receiptStateSummary struct {
	RequestBy         string             `json:"request_by"`
	ConversationSeq   int64              `json:"conversation_seq"`
	MessageID         string             `json:"message_id"`
	ReceivedUserCount int32              `json:"received_user_count"`
	ReadUserCount     int32              `json:"read_user_count"`
	VisibilityMode    string             `json:"visibility_mode"`
	Receivers         []receiptUserState `json:"receivers"`
}

type receiptUserState struct {
	UserID           string `json:"user_id"`
	ReceivedSeq      int64  `json:"received_seq"`
	ReceivedAtUnixMS int64  `json:"received_at_unix_ms"`
	ReadSeq          int64  `json:"read_seq"`
	ReadAtUnixMS     int64  `json:"read_at_unix_ms"`
}

type conversationListSummary struct {
	ItemCount           int                        `json:"item_count"`
	Items               []conversationSummaryItem  `json:"items"`
	NextPageCursor      string                     `json:"next_page_cursor,omitempty"`
	ProjectionWatermark projectionWatermarkSummary `json:"projection_watermark"`
	LatencyMS           float64                    `json:"latency_ms"`
}

type conversationSummaryItem struct {
	ConversationID  string `json:"conversation_id"`
	LastVisibleSeq  int64  `json:"last_visible_seq"`
	LastMessageID   string `json:"last_message_id"`
	LastSenderID    string `json:"last_sender_id"`
	UnreadCount     int64  `json:"unread_count"`
	LastReadSeq     int64  `json:"last_read_seq"`
	UpdatedAtUnixMS int64  `json:"updated_at_unix_ms"`
	Archived        bool   `json:"archived"`
	Pinned          bool   `json:"pinned"`
	Muted           bool   `json:"muted"`
}

type projectionWatermarkSummary struct {
	Source          string `json:"source"`
	OffsetValue     int64  `json:"offset_value"`
	UpdatedAtUnixMS int64  `json:"updated_at_unix_ms"`
}

type receiptProjectionStats struct {
	InboxProjectionCount     int64 `json:"inbox_projection_count"`
	InboxProjectionMinSeq    int64 `json:"inbox_projection_min_seq"`
	InboxProjectionMaxSeq    int64 `json:"inbox_projection_max_seq"`
	DeviceReceivedCursorSeq  int64 `json:"device_received_cursor_seq"`
	UserReceivedCursorSeq    int64 `json:"user_received_cursor_seq"`
	UserReadCursorSeq        int64 `json:"user_read_cursor_seq"`
	MessageReceiptStateCount int64 `json:"message_receipt_state_count"`
	ReceiverReceivedSeq      int64 `json:"receiver_received_seq"`
	ReceiverReadSeq          int64 `json:"receiver_read_seq"`
	ReceiptCheckpointOffset  int64 `json:"receipt_checkpoint_offset"`
	DeliveryCheckpointOffset int64 `json:"delivery_checkpoint_offset"`
}

type receiptOutboxStats struct {
	Total       int64            `json:"total"`
	Pending     int64            `json:"pending"`
	Published   int64            `json:"published"`
	DLQ         int64            `json:"dlq"`
	ByEventType map[string]int64 `json:"by_event_type"`
}

type receiptKafkaEvent struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	Partition        int    `json:"partition"`
	Offset           int64  `json:"offset"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
	PayloadType      string `json:"payload_type"`
	MessageID        string `json:"message_id"`
	UserID           string `json:"user_id"`
	DeviceID         string `json:"device_id"`
	CursorSeq        int64  `json:"cursor_seq"`
}

type outboxStats struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Published int64 `json:"published"`
	DLQ       int64 `json:"dlq"`
}
