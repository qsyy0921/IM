package monitoring

import (
	"bytes"
	"context"
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
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTraceID, "trace-1",
		metadataRequestID, "request-1",
	))

	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.policy.v1.PolicyService/CheckMessageAction"}, func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("interceptor success: %v", err)
	}
	_, err = interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.policy.v1.PolicyService/CheckMessageAction"}, func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.Unavailable, "policy unavailable")
	})
	if err == nil {
		t.Fatal("expected unavailable")
	}

	snapshot := metrics.Snapshot()
	if snapshot.TotalRequests != 2 || snapshot.TotalErrors != 1 || len(snapshot.Methods) != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	logBody := logs.String()
	if !strings.Contains(logBody, `"event":"grpc_request"`) ||
		!strings.Contains(logBody, `"code":"Unavailable"`) ||
		!strings.Contains(logBody, `"trace_id":"trace-1"`) ||
		!strings.Contains(logBody, `"request_id":"request-1"`) {
		t.Fatalf("expected structured grpc logs, got %s", logBody)
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

	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.policy.v1.PolicyService/CheckMessageAction"}, func(ctx context.Context, req any) (any, error) {
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
