package main

import (
	"context"
	"testing"

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

func assertMetadataValue(t *testing.T, md metadata.MD, key string, want string) {
	t.Helper()
	values := md.Get(key)
	if len(values) != 1 || values[0] != want {
		t.Fatalf("metadata %s = %v, want [%s]", key, values, want)
	}
}
