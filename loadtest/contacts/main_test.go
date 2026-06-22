package main

import (
	"math"
	"testing"
	"time"

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
		t.Fatal("expected invalid value to keep default")
	}
}

func TestEnvString(t *testing.T) {
	t.Setenv("NEXUSIM_TEST_STRING", " value ")
	if got := envString("NEXUSIM_TEST_STRING", "default"); got != "value" {
		t.Fatalf("envString = %q, want value", got)
	}
	t.Setenv("NEXUSIM_TEST_STRING", " ")
	if got := envString("NEXUSIM_TEST_STRING", "default"); got != "default" {
		t.Fatalf("envString default value = %q, want default", got)
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

func TestRequestContextWithGatewayMockAuth(t *testing.T) {
	ctx, cancel, auth := requestContext(config{
		tenantID:        "tenant-1",
		requestTimeout:  1,
		gatewayAuthMode: "mock",
	}, "user-1", "device-1", "request-1", "trace-1")
	defer cancel()
	if auth.GetSessionId() != "contacts-smoke-device-1" {
		t.Fatalf("unexpected session id: %s", auth.GetSessionId())
	}
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	assertMetadataValue(t, md, metadataToken, "tenant-1:user-1:device-1")
	assertMetadataValue(t, md, metadataTraceID, "trace-1")
	assertMetadataValue(t, md, metadataRequestID, "request-1")
	if values := md.Get(metadataTenantID); len(values) != 0 {
		t.Fatalf("did not expect verified tenant metadata with gateway auth, got %v", values)
	}
}

func TestRequestContextWithGatewayHMACAuth(t *testing.T) {
	ctx, cancel, _ := requestContext(config{
		tenantID:              "tenant-1",
		requestTimeout:        1,
		gatewayAuthMode:       "hmac",
		gatewayAuthHMACSecret: "secret",
		gatewayAuthAudience:   "api-gateway",
		gatewayAuthTokenTTL:   1,
	}, "user-1", "device-1", "request-1", "trace-1")
	defer cancel()
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	values := md.Get("authorization")
	if len(values) != 1 || values[0] == "" {
		t.Fatalf("expected authorization metadata, got %v", values)
	}
	assertMetadataValue(t, md, metadataRequestID, "request-1")
	if values := md.Get(metadataTenantID); len(values) != 0 {
		t.Fatalf("did not expect verified tenant metadata with gateway auth, got %v", values)
	}
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
	s := summary{
		Scenario:   "accept",
		StartedAt:  started,
		FinishedAt: started.Add(2 * time.Second),
		LatenciesMS: map[string]float64{
			"send_contact_request":    12,
			"respond_contact_request": 20,
			"list_sender_contacts":    7,
		},
		ContactsOutbox: outboxStats{Total: 2, Pending: 0, Published: 2, DLQ: 0},
		ContactKafkaEvents: []contactKafkaEvent{
			{EventID: "evt-1"},
			{EventID: "evt-2"},
		},
	}

	capacity := buildCapacitySummary(s)
	if capacity == nil {
		t.Fatal("expected capacity summary")
	}
	if capacity.Scenario != "accept" {
		t.Fatalf("scenario = %q, want accept", capacity.Scenario)
	}
	if capacity.OperationCount != 3 {
		t.Fatalf("operation count = %d, want 3", capacity.OperationCount)
	}
	if capacity.ContactEventCount != 2 {
		t.Fatalf("event count = %d, want 2", capacity.ContactEventCount)
	}
	if capacity.ContactsOutboxTotal != 2 || capacity.ContactsOutboxPending != 0 || capacity.ContactsOutboxDLQ != 0 {
		t.Fatalf("unexpected outbox capacity fields: %+v", capacity)
	}
	assertFloatNear(t, capacity.OperationsPerSecond, 1.5)
	assertFloatNear(t, capacity.EventsPerSecond, 1)
	assertFloatNear(t, capacity.LatencyP95MS, 20)
	assertFloatNear(t, capacity.LatencyP99MS, 20)
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
