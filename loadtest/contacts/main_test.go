package main

import (
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestEnvBool(t *testing.T) {
	t.Setenv("NEXUSIM_TEST_BOOL", "yes")
	if !envBool("NEXUSIM_TEST_BOOL", false) {
		t.Fatal("expected yes to be true")
	}
	t.Setenv("NEXUSIM_TEST_BOOL", "0")
	if envBool("NEXUSIM_TEST_BOOL", true) {
		t.Fatal("expected 0 to be false")
	}
	t.Setenv("NEXUSIM_TEST_BOOL", "invalid")
	if !envBool("NEXUSIM_TEST_BOOL", true) {
		t.Fatal("expected invalid value to keep fallback")
	}
}

func TestRequestContextWithoutVerifiedMetadata(t *testing.T) {
	ctx, cancel, auth := requestContext(config{
		tenantID:       "tenant-1",
		requestTimeout: 1,
	}, "user-1", "device-1", "request-1", "trace-1")
	defer cancel()
	if auth.GetTenantId() != "tenant-1" || auth.GetUserId() != "user-1" || auth.GetDeviceId() != "device-1" {
		t.Fatalf("unexpected auth context: %+v", auth)
	}
	if _, ok := metadata.FromOutgoingContext(ctx); ok {
		t.Fatal("did not expect metadata when verified metadata is disabled")
	}
}

func TestRequestContextWithVerifiedMetadata(t *testing.T) {
	ctx, cancel, _ := requestContext(config{
		tenantID:         "tenant-1",
		requestTimeout:   1,
		verifiedMetadata: true,
	}, "user-1", "device-1", "request-1", "trace-1")
	defer cancel()
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	assertMetadataValue(t, md, metadataTenantID, "tenant-1")
	assertMetadataValue(t, md, metadataUserID, "user-1")
	assertMetadataValue(t, md, metadataDeviceID, "device-1")
	assertMetadataValue(t, md, metadataSessionID, "contacts-smoke-device-1")
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
