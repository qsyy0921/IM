package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"testing"

	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGRPCMetricsInterceptorRecordsRequestsAndLogsJSON(t *testing.T) {
	metrics := NewGRPCMetrics()
	var logs bytes.Buffer
	interceptor := metrics.UnaryServerInterceptor(log.New(&logs, "", 0))

	_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.contacts.v1.ContactsService/ListContacts"}, func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("interceptor success: %v", err)
	}
	_, err = interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.contacts.v1.ContactsService/SendContactRequest"}, func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	})
	if err == nil {
		t.Fatal("expected permission denied")
	}

	snapshot := metrics.Snapshot()
	if snapshot.TotalRequests != 2 || snapshot.TotalErrors != 1 || len(snapshot.Methods) != 2 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if !strings.Contains(logs.String(), `"event":"grpc_request"`) || !strings.Contains(logs.String(), `"code":"PermissionDenied"`) {
		t.Fatalf("expected structured grpc logs, got %s", logs.String())
	}
}

func TestGRPCMetricsInterceptorLogsTraceAndRequestIDs(t *testing.T) {
	metrics := NewGRPCMetrics()
	var logs bytes.Buffer
	interceptor := metrics.UnaryServerInterceptor(log.New(&logs, "", 0))
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTraceID, " trace-1 ",
		metadataRequestID, "request-1",
		"authorization", "Bearer should-not-be-logged",
	))

	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.contacts.v1.ContactsService/ListContacts"}, func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("interceptor success: %v", err)
	}

	var entry grpcRequestLog
	if err := json.Unmarshal([]byte(strings.TrimSpace(logs.String())), &entry); err != nil {
		t.Fatalf("decode grpc request log: %v; raw=%s", err, logs.String())
	}
	if entry.Service != serviceName ||
		entry.Event != "grpc_request" ||
		entry.Method != "/nexusim.contacts.v1.ContactsService/ListContacts" ||
		entry.Code != codes.OK.String() ||
		entry.TraceID != "trace-1" ||
		entry.RequestID != "request-1" {
		t.Fatalf("unexpected grpc request log: %+v", entry)
	}
	if strings.Contains(logs.String(), "should-not-be-logged") || strings.Contains(logs.String(), "authorization") {
		t.Fatalf("log leaked auth metadata: %s", logs.String())
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

func TestGRPCMetricsInterceptorDropsUnsafeCorrelationMetadata(t *testing.T) {
	metrics := NewGRPCMetrics()
	var logs bytes.Buffer
	interceptor := metrics.UnaryServerInterceptor(log.New(&logs, "", 0))
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTraceID, "trace user=user1@example.com",
		metadataRequestID, "request-token=secret-token",
		"authorization", "Bearer should-not-be-logged",
	))

	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.contacts.v1.ContactsService/ListContacts"}, func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("interceptor success: %v", err)
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

func TestGRPCMetricsInterceptorMapsPlainErrorToUnknown(t *testing.T) {
	metrics := NewGRPCMetrics()
	interceptor := metrics.UnaryServerInterceptor(log.New(&bytes.Buffer{}, "", 0))

	_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: "/test.Service/Method"}, func(ctx context.Context, req any) (any, error) {
		return nil, errors.New("plain failure")
	})
	if err == nil {
		t.Fatal("expected plain failure")
	}

	snapshot := metrics.Snapshot()
	if snapshot.TotalRequests != 1 || snapshot.TotalErrors != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if len(snapshot.Methods) != 1 || snapshot.Methods[0].Codes[codes.Unknown.String()] != 1 {
		t.Fatalf("expected Unknown code count, got %+v", snapshot.Methods)
	}
}
