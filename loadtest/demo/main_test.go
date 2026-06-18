package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	gatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/gateway/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc/metadata"
)

var _ conversationv1.ConversationServiceClient = gatewayConversationClient{}
var _ messagev1.MessageServiceClient = (gatewayv1.GatewayServiceClient)(nil)
var _ deliveryv1.DeliveryServiceClient = (gatewayv1.GatewayServiceClient)(nil)
var _ receiptv1.ReceiptServiceClient = (gatewayv1.GatewayServiceClient)(nil)

func TestEnvBool(t *testing.T) {
	t.Setenv("NEXUSIM_TEST_BOOL", "true")
	if !envBool("NEXUSIM_TEST_BOOL", false) {
		t.Fatal("expected true env bool")
	}
	t.Setenv("NEXUSIM_TEST_BOOL", "off")
	if envBool("NEXUSIM_TEST_BOOL", true) {
		t.Fatal("expected false env bool")
	}
	t.Setenv("NEXUSIM_TEST_BOOL", "invalid")
	if !envBool("NEXUSIM_TEST_BOOL", true) {
		t.Fatal("expected invalid env bool to keep fallback")
	}
}

func TestWithVerifiedAuthMetadataDisabled(t *testing.T) {
	ctx := withVerifiedAuthMetadata(context.Background(), config{}, demoAuth{
		tenantID: "tenant-1",
		userID:   "user-1",
		deviceID: "device-1",
	})
	if _, ok := metadata.FromOutgoingContext(ctx); ok {
		t.Fatal("did not expect outgoing metadata when disabled")
	}
}

func TestWithVerifiedAuthMetadataAddsOutgoingMetadata(t *testing.T) {
	ctx := withVerifiedAuthMetadata(context.Background(), config{verifiedAuthMetadata: true}, demoAuth{
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

func TestWithUserFacingAuthMetadataUsesGatewayHMACToken(t *testing.T) {
	ctx, err := withUserFacingAuthMetadata(context.Background(), config{
		gatewayAuthMode:       "hmac",
		gatewayAuthHMACSecret: "gateway-secret",
		gatewayAuthTokenTTL:   time.Minute,
	}, demoAuth{
		tenantID:  "tenant-1",
		userID:    "user-1",
		deviceID:  "device-1",
		sessionID: "session-1",
		traceID:   "trace-1",
		requestID: "request-1",
	})
	if err != nil {
		t.Fatalf("withUserFacingAuthMetadata returned error: %v", err)
	}
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	if got := md.Get("authorization"); len(got) != 1 || got[0] == "" {
		t.Fatalf("expected authorization bearer metadata, got %v", got)
	}
	authenticator, err := gatewayauth.NewAuthenticator(gatewayauth.Config{
		Mode:     gatewayauth.ModeHMAC,
		Secret:   "gateway-secret",
		Audience: "api-gateway",
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token := strings.TrimPrefix(md.Get("authorization")[0], "Bearer ")
	if _, err := authenticator.Authenticate(httptest.NewRequest(http.MethodGet, "/?token="+token, nil)); err != nil {
		t.Fatalf("expected generated token to use api-gateway audience: %v", err)
	}
	assertMetadataValue(t, md, metadataRequestID, "request-1")
	if values := md.Get(metadataTenantID); len(values) != 0 {
		t.Fatalf("did not expect trusted metadata when using api-gateway auth, got %v", values)
	}
}

func TestWithUserFacingAuthMetadataUsesGatewayMockToken(t *testing.T) {
	ctx, err := withUserFacingAuthMetadata(context.Background(), config{gatewayAuthMode: "mock"}, demoAuth{
		tenantID:  "tenant-1",
		userID:    "user-1",
		deviceID:  "device-1",
		traceID:   "trace-1",
		requestID: "request-1",
	})
	if err != nil {
		t.Fatalf("withUserFacingAuthMetadata returned error: %v", err)
	}
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	assertMetadataValue(t, md, metadataToken, "tenant-1:user-1:device-1")
	assertMetadataValue(t, md, metadataTraceID, "trace-1")
	assertMetadataValue(t, md, metadataRequestID, "request-1")
}

func TestWebSocketDialOptionsCombinesHeaderAndTLS(t *testing.T) {
	caFile := writeTestCACert(t)
	options, err := webSocketDialOptions(config{
		pushTLS: grpctls.Config{
			CAFile:     caFile,
			ServerName: "push-gateway.nexusim.local",
		},
	}, http.Header{"Authorization": []string{"Bearer token-1"}})
	if err != nil {
		t.Fatalf("webSocketDialOptions returned error: %v", err)
	}
	if options == nil {
		t.Fatal("expected dial options")
	}
	if got := options.HTTPHeader.Get("Authorization"); got != "Bearer token-1" {
		t.Fatalf("Authorization header = %q", got)
	}
	if options.HTTPClient == nil {
		t.Fatal("expected HTTP client for WSS TLS")
	}
}

func TestWebSocketTLSConfigRequiresCAFile(t *testing.T) {
	_, err := webSocketTLSConfig(grpctls.Config{ServerName: "push-gateway.nexusim.local"}, "push-tls")
	if err == nil {
		t.Fatal("expected missing CA file error")
	}
}

func TestWebSocketTLSConfigRequiresClientCertPair(t *testing.T) {
	caFile := writeTestCACert(t)
	_, err := webSocketTLSConfig(grpctls.Config{
		CAFile:         caFile,
		ClientCertFile: filepath.Join(t.TempDir(), "client.crt"),
	}, "push-tls")
	if err == nil {
		t.Fatal("expected client cert/key pair error")
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
		GatewayFacade:   true,
		GatewayAuthMode: "hmac",
		StartedAt:       started,
		FinishedAt:      started.Add(2 * time.Second),
		ServerHello:     serverFrame{Op: opServerHello},
		MemberJoin:      memberJoinSummary{ChangeID: "change-1", BoundarySeq: 1},
		SendMessage:     sendSummary{MessageID: "msg-1", ConversationSeq: 2},
		Notify:          serverFrame{Op: opDeliveryNotify, ConversationSeq: 2},
		PullInbox:       pullSummary{ItemCount: 1, MaxSeq: 2},
		WebSocketAck:    serverFrame{Op: opDeliveryAckOK, LastReceivedSeq: 2},
		MarkRead:        markReadSummary{LastReadSeq: 2},
		ListBeforeRead: conversationListSummary{
			ItemCount: 1,
			Items: []conversationSummaryItem{
				{ConversationID: "conv-1", LastVisibleSeq: 2, UnreadCount: 1},
			},
		},
		ListAfterRead: conversationListSummary{
			ItemCount: 1,
			Items: []conversationSummaryItem{
				{ConversationID: "conv-1", LastVisibleSeq: 2, UnreadCount: 0},
			},
		},
		Postgres:         postgresSummary{UserInboxCount: 1, UserConversationSummaries: 1},
		PolicyAuditKafka: &policyAuditKafkaSummary{EventCount: 1},
	}

	capacity := buildCapacitySummary(s)
	if capacity == nil {
		t.Fatal("expected capacity summary")
	}
	if !capacity.GatewayFacade || capacity.GatewayAuthMode != "hmac" {
		t.Fatalf("unexpected gateway fields: %+v", capacity)
	}
	if capacity.UserFacingOperationCount != 7 {
		t.Fatalf("operation count = %d, want 7", capacity.UserFacingOperationCount)
	}
	if capacity.WebSocketFrameCount != 3 {
		t.Fatalf("websocket frame count = %d, want 3", capacity.WebSocketFrameCount)
	}
	if capacity.MessageCount != 1 || capacity.NotifyFrameCount != 1 {
		t.Fatalf("unexpected message/notify counts: %+v", capacity)
	}
	if capacity.ItemsPulled != 1 || capacity.MaxConversationSeq != 2 {
		t.Fatalf("unexpected inbox/seq fields: %+v", capacity)
	}
	if capacity.UnreadBeforeRead != 1 || capacity.UnreadAfterRead != 0 {
		t.Fatalf("unexpected unread fields: %+v", capacity)
	}
	if capacity.PostgresUserInboxCount != 1 || capacity.PostgresSummaryCount != 1 || capacity.PolicyAuditKafkaEvents != 1 {
		t.Fatalf("unexpected aggregate fields: %+v", capacity)
	}
	assertFloatNear(t, capacity.OperationsPerSecond, 3.5)
}

func TestBuildCapacitySummaryUsesCapacityCounts(t *testing.T) {
	started := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	s := summary{
		GatewayFacade:    true,
		StartedAt:        started,
		FinishedAt:       started.Add(10 * time.Second),
		ServerHello:      serverFrame{Op: opServerHello},
		MemberJoin:       memberJoinSummary{ChangeID: "change-1", BoundarySeq: 1},
		SendMessage:      sendSummary{MessageID: "msg-last", ConversationSeq: 42},
		Notify:           serverFrame{Op: opDeliveryNotify, ConversationSeq: 42},
		MessageCount:     20,
		NotifyFrameCount: 20,
		PullInbox:        pullSummary{ItemCount: 1, MaxSeq: 42},
		WebSocketAck:     serverFrame{Op: opDeliveryAckOK, LastReceivedSeq: 42},
		MarkRead:         markReadSummary{LastReadSeq: 42},
		ListBeforeRead:   conversationListSummary{ItemCount: 1},
		ListAfterRead:    conversationListSummary{ItemCount: 1},
		Postgres:         postgresSummary{UserInboxCount: 20, UserConversationSummaries: 1},
		PolicyAuditKafka: &policyAuditKafkaSummary{EventCount: 20},
	}

	capacity := buildCapacitySummary(s)
	if capacity == nil {
		t.Fatal("expected capacity summary")
	}
	if capacity.UserFacingOperationCount != 26 {
		t.Fatalf("operation count = %d, want 26", capacity.UserFacingOperationCount)
	}
	if capacity.WebSocketFrameCount != 22 || capacity.MessageCount != 20 || capacity.NotifyFrameCount != 20 {
		t.Fatalf("unexpected websocket/message counts: %+v", capacity)
	}
	assertFloatNear(t, capacity.OperationsPerSecond, 2.6)
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

func writeTestCACert(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "ca.crt")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create CA file: %v", err)
	}
	defer file.Close()
	if err := pem.Encode(file, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("write CA file: %v", err)
	}
	return path
}
