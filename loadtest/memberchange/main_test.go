package main

import (
	"context"
	"testing"

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
