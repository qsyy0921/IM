package main

import (
	"context"
	"math"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
)

func TestWithVerifiedAuthMetadataDisabled(t *testing.T) {
	ctx := withVerifiedAuthMetadata(context.Background(), config{}, verifiedAuthIdentity{
		tenantID: "tenant-1",
		userID:   "user-1",
		deviceID: "device-1",
	})
	if _, ok := metadata.FromOutgoingContext(ctx); ok {
		t.Fatal("did not expect outgoing metadata when disabled")
	}
}

func TestWithVerifiedAuthMetadataAddsOutgoingMetadata(t *testing.T) {
	ctx := withVerifiedAuthMetadata(context.Background(), config{verifiedAuthMetadata: true}, verifiedAuthIdentity{
		tenantID:  "tenant-1",
		userID:    "user-1",
		deviceID:  "device-1",
		sessionID: "session-1",
		traceID:   "trace-1",
		requestID: "request-1",
	})
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	assertMetadataValue(t, md, metadataTenantID, "tenant-1")
	assertMetadataValue(t, md, metadataUserID, "user-1")
	assertMetadataValue(t, md, metadataDeviceID, "device-1")
	assertMetadataValue(t, md, metadataSessionID, "session-1")
	assertMetadataValue(t, md, metadataTraceID, "trace-1")
	assertMetadataValue(t, md, metadataRequestID, "request-1")
}

func TestReceiptAuthHelpersUseExpectedIdentities(t *testing.T) {
	cfg := config{
		tenantID:         "tenant-1",
		ownerUserID:      "owner-1",
		receiverUserID:   "receiver-1",
		receiverDeviceID: "device-1",
	}
	owner := ownerAuth(cfg, "trace-owner", "request-owner")
	if auth := messageAuth(owner); auth.GetUserId() != "owner-1" || auth.GetDeviceId() != "receipt-smoke-owner-device" {
		t.Fatalf("unexpected owner auth: %+v", auth)
	}
	receiver := receiverAuth(cfg, "trace-receiver", "request-receiver")
	if auth := receiptAuth(receiver); auth.GetUserId() != "receiver-1" || auth.GetDeviceId() != "device-1" {
		t.Fatalf("unexpected receiver auth: %+v", auth)
	}
}

func TestBuildCapacitySummary(t *testing.T) {
	started := time.Date(2026, 6, 16, 1, 2, 3, 0, time.UTC)
	result := &summary{
		StartedAt:  started,
		FinishedAt: started.Add(4 * time.Second),
		SendMessage: sendSummary{
			MessageID:       "msg-1",
			ConversationSeq: 2,
		},
		SendMessageWhileArchived: sendSummary{
			MessageID:       "msg-2",
			ConversationSeq: 3,
		},
		PullInbox:              pullSummary{ItemCount: 1},
		PullInboxWhileArchived: pullSummary{ItemCount: 1},
		AckDelivery:            ackSummary{LastReceivedSeq: 2},
		AckDeliveryWhileArchived: ackSummary{
			LastReceivedSeq: 3,
		},
		MarkRead: markReadSummary{LastReadSeq: 2},
		ReceiptBeforeReadBySeq: receiptStateSummary{
			ConversationSeq: 2,
		},
		ReceiptAfterReadBySeq: receiptStateSummary{
			ConversationSeq: 2,
		},
		ReceiptAfterReadByMsgID: receiptStateSummary{
			MessageID: "msg-1",
		},
		ConversationListBefore:                  conversationListSummary{LatencyMS: 1},
		ConversationListUnreadBeforeRead:        conversationListSummary{ItemCount: 1},
		ConversationListAfter:                   conversationListSummary{ProjectionWatermark: projectionWatermarkSummary{OffsetValue: 9}},
		ConversationListUnreadAfterRead:         conversationListSummary{LatencyMS: 1},
		ConversationListArchivedDefault:         conversationListSummary{LatencyMS: 1},
		ConversationListArchivedIncluded:        conversationListSummary{LatencyMS: 1},
		ConversationListAfterArchivedNewDefault: conversationListSummary{LatencyMS: 1},
		ConversationListAfterArchivedNewIncluded: conversationListSummary{
			LatencyMS: 1,
		},
		ConversationListAfterUnarchive: conversationListSummary{LatencyMS: 1},
		ConversationListAfterPin:       conversationListSummary{LatencyMS: 1},
		ConversationListAfterUnpin:     conversationListSummary{LatencyMS: 1},
		ConversationListAfterMute:      conversationListSummary{LatencyMS: 1},
		ConversationListAfterUnmute:    conversationListSummary{LatencyMS: 1},
		ArchiveConversation:            archiveSummary{LatencyMS: 1},
		UnarchiveConversation:          archiveSummary{LatencyMS: 1},
		PinConversation:                pinSummary{LatencyMS: 1},
		UnpinConversation:              pinSummary{LatencyMS: 1},
		MuteConversation:               muteSummary{LatencyMS: 1},
		UnmuteConversation:             muteSummary{LatencyMS: 1},
		MarkReadTooFar:                 negativeCallSummary{Passed: true},
		ReceiptOutbox:                  receiptOutboxStats{Published: 2, Pending: 1, DLQ: 0},
		DeliveryOutbox:                 outboxStats{Published: 3, Pending: 0, DLQ: 1},
		ReceiptKafkaEvents: []receiptKafkaEvent{
			{EventID: "event-1"},
			{EventID: "event-2"},
		},
	}

	capacity := buildCapacitySummary(result)
	if capacity == nil {
		t.Fatal("expected capacity summary")
	}
	if capacity.DurationMS != 4000 ||
		capacity.MessageCount != 2 ||
		capacity.PullItemCount != 2 ||
		capacity.AckCount != 2 ||
		capacity.MarkReadCount != 1 ||
		capacity.ReceiptStateQueryCount != 3 ||
		capacity.ConversationListCallCount != 13 ||
		capacity.StateMutationCount != 7 ||
		capacity.ReceiptKafkaEventCount != 2 {
		t.Fatalf("unexpected capacity counts: %+v", capacity)
	}
	if capacity.ReceiptOutboxPublished != 2 ||
		capacity.ReceiptOutboxPending != 1 ||
		capacity.ReceiptOutboxDLQ != 0 ||
		capacity.DeliveryOutboxPublished != 3 ||
		capacity.DeliveryOutboxPending != 0 ||
		capacity.DeliveryOutboxDLQ != 1 {
		t.Fatalf("unexpected outbox counts: %+v", capacity)
	}
	assertFloatNear(t, capacity.MessagesPerSecond, 0.5)
	assertFloatNear(t, capacity.ReceiptEventsPerSecond, 0.5)
	assertFloatNear(t, capacity.OperationsPerSecond, 7.5)
}

func TestBuildCapacitySummaryUsesCapacityCounters(t *testing.T) {
	started := time.Date(2026, 6, 18, 1, 2, 3, 0, time.UTC)
	result := &summary{
		CapacityMode:                   true,
		VUs:                            4,
		StartedAt:                      started,
		FinishedAt:                     started.Add(10 * time.Second),
		CapacityMessageCount:           10,
		CapacityPullItemCount:          10,
		CapacityAckCount:               10,
		CapacityMarkReadCount:          10,
		CapacityReceiptKafkaEventCount: 20,
		CapacityLatencySamplesMS:       []float64{1, 5, 10, 20, 50},
		ReceiptOutbox:                  receiptOutboxStats{Published: 20, Pending: 0, DLQ: 0},
		DeliveryOutbox:                 outboxStats{Published: 10, Pending: 0, DLQ: 0},
	}

	capacity := buildCapacitySummary(result)
	if capacity == nil {
		t.Fatal("expected capacity summary")
	}
	if capacity.VUs != 4 ||
		capacity.MessageCount != 10 ||
		capacity.PullItemCount != 10 ||
		capacity.AckCount != 10 ||
		capacity.MarkReadCount != 10 ||
		capacity.ReceiptKafkaEventCount != 20 ||
		capacity.ReceiptStateQueryCount != 0 ||
		capacity.ConversationListCallCount != 0 ||
		capacity.StateMutationCount != 0 {
		t.Fatalf("unexpected capacity counts: %+v", capacity)
	}
	assertFloatNear(t, capacity.OperationsPerSecond, 4)
	assertFloatNear(t, capacity.MessagesPerSecond, 1)
	assertFloatNear(t, capacity.ReceiptEventsPerSecond, 2)
	assertFloatNear(t, capacity.LatencyP95MS, 50)
	assertFloatNear(t, capacity.LatencyP99MS, 50)
}

func TestBuildCapacitySummaryRequiresPositiveDuration(t *testing.T) {
	if got := buildCapacitySummary(&summary{}); got != nil {
		t.Fatalf("expected nil capacity for empty timestamps, got %+v", got)
	}
	started := time.Date(2026, 6, 16, 1, 2, 3, 0, time.UTC)
	if got := buildCapacitySummary(&summary{StartedAt: started, FinishedAt: started}); got != nil {
		t.Fatalf("expected nil capacity for zero duration, got %+v", got)
	}
}

func assertMetadataValue(t *testing.T, md metadata.MD, key string, want string) {
	t.Helper()
	values := md.Get(key)
	if len(values) != 1 || values[0] != want {
		t.Fatalf("metadata %s = %v, want [%s]", key, values, want)
	}
}

func assertFloatNear(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.000001 {
		t.Fatalf("got %v, want %v", got, want)
	}
}
