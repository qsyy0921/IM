package main

import (
	"context"
	"math"
	"testing"
	"time"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	"google.golang.org/grpc/metadata"
)

func TestParseMemberChangeType(t *testing.T) {
	tests := []struct {
		name string
		want conversationv1.MemberChangeType
	}{
		{name: "join", want: conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN},
		{name: "LEAVE", want: conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_LEAVE},
		{name: "remove", want: conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_REMOVE},
		{name: "role-changed", want: conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_ROLE_CHANGED},
	}
	for _, test := range tests {
		got, _, err := parseMemberChangeType(test.name)
		if err != nil {
			t.Fatalf("parseMemberChangeType(%q) returned error: %v", test.name, err)
		}
		if got != test.want {
			t.Fatalf("parseMemberChangeType(%q) = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestIsOwnerTransferChange(t *testing.T) {
	tests := []string{"owner-transfer", "OWNER_TRANSFER", "owner transfer"}
	for _, test := range tests {
		if !isOwnerTransferChange(test) {
			t.Fatalf("isOwnerTransferChange(%q) = false, want true", test)
		}
	}
	if isOwnerTransferChange("role-changed") {
		t.Fatal("isOwnerTransferChange(role-changed) = true, want false")
	}
}

func TestParseConfigDefaultsListUserToOperator(t *testing.T) {
	cfg := normalizeConfigDefaults(config{operatorUserID: "operator-1"})
	if cfg.listUserID != cfg.operatorUserID {
		t.Fatalf("listUserID = %q, want operator user %q", cfg.listUserID, cfg.operatorUserID)
	}
}

func TestParseMemberRole(t *testing.T) {
	tests := []struct {
		name string
		want conversationv1.MemberRole
	}{
		{name: "owner", want: conversationv1.MemberRole_MEMBER_ROLE_OWNER},
		{name: "ADMIN", want: conversationv1.MemberRole_MEMBER_ROLE_ADMIN},
		{name: "member", want: conversationv1.MemberRole_MEMBER_ROLE_MEMBER},
	}
	for _, test := range tests {
		got, _, err := parseMemberRole(test.name)
		if err != nil {
			t.Fatalf("parseMemberRole(%q) returned error: %v", test.name, err)
		}
		if got != test.want {
			t.Fatalf("parseMemberRole(%q) = %v, want %v", test.name, got, test.want)
		}
	}
}

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
	ctx := withVerifiedAuthMetadata(context.Background(), config{verifiedMetadata: true}, verifiedAuthIdentity{
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

func assertMetadataValue(t *testing.T, md metadata.MD, key string, want string) {
	t.Helper()
	values := md.Get(key)
	if len(values) != 1 || values[0] != want {
		t.Fatalf("metadata %s = %v, want [%s]", key, values, want)
	}
}

func TestBuildCapacitySummary(t *testing.T) {
	started := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	sagaCount := int64(4)
	sagaDoneCount := int64(3)
	timelineCount := int64(4)
	outboxTotal := int64(4)
	outboxPublished := int64(3)
	outboxPending := int64(1)
	outboxDLQ := int64(0)
	memberListCount := int64(6)
	seq := int64(9)
	s := summary{
		StartedAt:              started,
		FinishedAt:             started.Add(2 * time.Second),
		VUs:                    2,
		RequestCount:           4,
		SuccessCount:           3,
		ErrorCount:             1,
		SuccessRate:            0.75,
		RPS:                    2,
		AvgMS:                  10,
		P95MS:                  20,
		P99MS:                  30,
		ChangeType:             "JOIN",
		TargetRole:             "MEMBER",
		SagaCount:              &sagaCount,
		SagaDoneCount:          &sagaDoneCount,
		TimelineCount:          &timelineCount,
		OutboxTotalCount:       &outboxTotal,
		OutboxPublishedCount:   &outboxPublished,
		OutboxPendingCount:     &outboxPending,
		OutboxDLQCount:         &outboxDLQ,
		MemberListCount:        &memberListCount,
		ConversationSeqCurrent: &seq,
	}

	capacity := buildCapacitySummary(s)
	if capacity == nil {
		t.Fatal("expected capacity summary")
	}
	if capacity.VUs != 2 || capacity.RequestCount != 4 || capacity.SuccessCount != 3 || capacity.ErrorCount != 1 {
		t.Fatalf("unexpected count fields: %+v", capacity)
	}
	assertFloatNear(t, capacity.SuccessRate, 0.75)
	assertFloatNear(t, capacity.RequestsPerSecond, 2)
	if capacity.SagaCount != 4 || capacity.SagaDoneCount != 3 || capacity.TimelineCount != 4 {
		t.Fatalf("unexpected saga/timeline fields: %+v", capacity)
	}
	if capacity.OutboxTotalCount != 4 || capacity.OutboxPublishedCount != 3 || capacity.OutboxPendingCount != 1 || capacity.OutboxDLQCount != 0 {
		t.Fatalf("unexpected outbox fields: %+v", capacity)
	}
	if capacity.MemberListCount != 6 || capacity.ConversationSeqCurrent != 9 {
		t.Fatalf("unexpected member/seq fields: %+v", capacity)
	}
}

func TestBuildCapacitySummaryRequiresPositiveDuration(t *testing.T) {
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	if capacity := buildCapacitySummary(summary{StartedAt: now, FinishedAt: now}); capacity != nil {
		t.Fatalf("expected nil capacity for zero duration, got %+v", capacity)
	}
}

func assertFloatNear(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.000001 {
		t.Fatalf("float = %f, want %f", got, want)
	}
}
