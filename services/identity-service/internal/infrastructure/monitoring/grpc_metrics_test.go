package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"strings"
	"testing"

	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestUnaryServerInterceptorLogsTraceAndRequestIDs(t *testing.T) {
	var logs bytes.Buffer
	metrics := NewGRPCMetrics()
	interceptor := metrics.UnaryServerInterceptor(log.New(&logs, "", 0))
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTraceID, " trace-1 ",
		metadataRequestID, "request-1",
	))
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.identity.v1.IdentityService/Login"}, func(context.Context, any) (any, error) {
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
	var entry grpcRequestLog
	if err := json.Unmarshal([]byte(strings.TrimSpace(logs.String())), &entry); err != nil {
		t.Fatalf("decode grpc request log: %v; raw=%s", err, logs.String())
	}
	if entry.Service != serviceName ||
		entry.Event != "grpc_request" ||
		entry.Method != "/nexusim.identity.v1.IdentityService/Login" ||
		entry.Code != codes.PermissionDenied.String() ||
		entry.TraceID != "trace-1" ||
		entry.RequestID != "request-1" {
		t.Fatalf("unexpected grpc request log: %+v", entry)
	}
	snapshot := metrics.Snapshot()
	if snapshot.TotalRequests != 1 || snapshot.TotalErrors != 1 || len(snapshot.Methods) != 1 {
		t.Fatalf("unexpected grpc metrics snapshot: %+v", snapshot)
	}
}

func TestGRPCLogMetadataTrimsAndBoundsValues(t *testing.T) {
	longTraceID := strings.Repeat("t", maxGRPCLogMetadataLength+16)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTraceID, " \t"+longTraceID+" ",
		metadataRequestID, "request-2",
	))
	traceID, requestID := grpcLogMetadata(ctx)
	if len([]rune(traceID)) != maxGRPCLogMetadataLength {
		t.Fatalf("expected trace id to be bounded to %d runes, got %d", maxGRPCLogMetadataLength, len([]rune(traceID)))
	}
	if strings.Contains(traceID, " ") || strings.Contains(traceID, "\t") {
		t.Fatalf("expected trace id to be trimmed, got %q", traceID)
	}
	if requestID != "request-2" {
		t.Fatalf("unexpected request id %q", requestID)
	}
}

func TestUnaryServerInterceptorDropsUnsafeCorrelationLogMetadata(t *testing.T) {
	var logs bytes.Buffer
	metrics := NewGRPCMetrics()
	interceptor := metrics.UnaryServerInterceptor(log.New(&logs, "", 0))
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTraceID, "trace user=user1@example.com",
		metadataRequestID, "request-token=secret-token",
		"authorization", "Bearer should-not-be-logged",
	))
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.identity.v1.IdentityService/Login"}, func(context.Context, any) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	line := logs.String()
	for _, leaked := range []string{
		"user1@example.com",
		"secret-token",
		"should-not-be-logged",
		"authorization",
		`"trace_id"`,
		`"request_id"`,
	} {
		if strings.Contains(line, leaked) {
			t.Fatalf("log leaked unsafe correlation metadata %q: %s", leaked, line)
		}
	}
}
