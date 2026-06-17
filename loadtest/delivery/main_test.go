package main

import (
	"context"
	"math"
	"testing"
	"time"

	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
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

func TestDeliveryAuthUsesExpectedIdentity(t *testing.T) {
	auth := verifiedAuthIdentity{
		tenantID:  "tenant-1",
		userID:    "user-1",
		deviceID:  "device-1",
		sessionID: "session-1",
		traceID:   "trace-1",
		requestID: "request-1",
	}
	got := deliveryAuth(auth)
	if got.GetTenantId() != "tenant-1" || got.GetUserId() != "user-1" || got.GetDeviceId() != "device-1" {
		t.Fatalf("unexpected delivery auth: %+v", got)
	}
	if got.GetSessionId() != "session-1" || got.GetTraceId() != "trace-1" || got.GetRequestId() != "request-1" {
		t.Fatalf("unexpected delivery request metadata: %+v", got)
	}
}

func TestEnvBoolUsesFirstConfiguredValue(t *testing.T) {
	t.Setenv("NEXUSIM_DELIVERY_TEST_BOOL_A", "")
	t.Setenv("NEXUSIM_DELIVERY_TEST_BOOL_B", "true")
	if !envBool(false, "NEXUSIM_DELIVERY_TEST_BOOL_A", "NEXUSIM_DELIVERY_TEST_BOOL_B") {
		t.Fatal("expected true from second configured env")
	}
}

func TestBuildCapacitySummary(t *testing.T) {
	started := time.Date(2026, 6, 16, 1, 2, 3, 0, time.UTC)
	inboxCount := int64(8)
	outboxTotal := int64(3)
	outboxPending := int64(1)
	outboxDLQ := int64(0)
	checkpoint := int64(42)
	result := &summary{
		PollCount:             4,
		ItemCount:             8,
		ExpectedCount:         6,
		PullP95MS:             12.5,
		PullP99MS:             20.5,
		AckEnabled:            true,
		AckLatencyMS:          3.5,
		InboxCount:            &inboxCount,
		DeliveryOutboxTotal:   &outboxTotal,
		DeliveryOutboxPending: &outboxPending,
		DeliveryOutboxDLQ:     &outboxDLQ,
		CheckpointOffsetValue: &checkpoint,
		StartedAt:             started,
		FinishedAt:            started.Add(2 * time.Second),
	}

	capacity := buildCapacitySummary(result)
	if capacity == nil {
		t.Fatal("expected capacity summary")
	}
	if capacity.DurationMS != 2000 ||
		capacity.PollCount != 4 ||
		capacity.ItemCount != 8 ||
		capacity.ExpectedCount != 6 ||
		!capacity.AckEnabled ||
		capacity.AckLatencyMS != 3.5 {
		t.Fatalf("unexpected capacity fields: %+v", capacity)
	}
	assertFloatNear(t, capacity.PullsPerSecond, 2)
	assertFloatNear(t, capacity.ItemsPerSecond, 4)
	if capacity.InboxCount == nil || *capacity.InboxCount != 8 ||
		capacity.DeliveryOutboxTotal == nil || *capacity.DeliveryOutboxTotal != 3 ||
		capacity.DeliveryOutboxPending == nil || *capacity.DeliveryOutboxPending != 1 ||
		capacity.DeliveryOutboxDLQ == nil || *capacity.DeliveryOutboxDLQ != 0 ||
		capacity.CheckpointOffsetValue == nil || *capacity.CheckpointOffsetValue != 42 {
		t.Fatalf("unexpected pointer fields: %+v", capacity)
	}
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

func TestRecordPullResponseAggregatesAndCapsSamples(t *testing.T) {
	result := &summary{}
	items := make([]*deliveryv1.InboxItem, 0, 105)
	for seq := int64(1); seq <= 105; seq++ {
		items = append(items, &deliveryv1.InboxItem{
			ConversationSeq: seq,
			EventId:         "event",
			EventType:       "message.persisted.v1",
			MessageId:       "message",
			SenderId:        "sender",
		})
	}

	recordPullResponse(result, &deliveryv1.PullInboxResponse{
		Items:   items,
		HasMore: true,
	}, true)

	if result.ItemCount != 105 || result.MaxSeq != 105 || !result.HasMore {
		t.Fatalf("unexpected aggregate fields: %+v", result)
	}
	if len(result.Items) != 100 {
		t.Fatalf("expected 100 sampled items, got %d", len(result.Items))
	}
	if result.Items[0].ConversationSeq != 1 || result.Items[99].ConversationSeq != 100 {
		t.Fatalf("unexpected sampled bounds: first=%+v last=%+v", result.Items[0], result.Items[99])
	}
}

func TestRecordPullResponseSnapshotOverwritesPreviousAttempt(t *testing.T) {
	result := &summary{}
	recordPullResponse(result, &deliveryv1.PullInboxResponse{
		Items: []*deliveryv1.InboxItem{{
			ConversationSeq: 10,
			EventId:         "old-event",
		}},
		HasMore: true,
	}, false)
	recordPullResponse(result, &deliveryv1.PullInboxResponse{
		Items: []*deliveryv1.InboxItem{{
			ConversationSeq: 2,
			EventId:         "new-event",
		}},
		HasMore: false,
	}, false)

	if result.ItemCount != 1 || result.MaxSeq != 2 || result.HasMore {
		t.Fatalf("unexpected snapshot fields: %+v", result)
	}
	if len(result.Items) != 1 || result.Items[0].EventID != "new-event" {
		t.Fatalf("expected latest response sample only, got %+v", result.Items)
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
