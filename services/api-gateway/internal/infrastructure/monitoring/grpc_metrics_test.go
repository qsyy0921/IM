package monitoring

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGRPCMetricsRecordsCodeAndWritesLowSensitiveLog(t *testing.T) {
	metrics := NewGRPCMetrics()
	var logs bytes.Buffer
	logger := log.New(&logs, "", 0)
	interceptor := metrics.UnaryServerInterceptor(logger)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTraceID, "trace-1",
		metadataRequestID, "request-1",
		"authorization", "Bearer should-not-be-logged",
	))

	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.api/Test"}, func(context.Context, any) (any, error) {
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.TotalRequests != 1 || snapshot.TotalErrors != 1 {
		t.Fatalf("unexpected snapshot totals: %+v", snapshot)
	}
	if len(snapshot.Methods) != 1 || snapshot.Methods[0].Codes["PermissionDenied"] != 1 {
		t.Fatalf("unexpected method snapshot: %+v", snapshot.Methods)
	}
	line := logs.String()
	for _, want := range []string{
		`"service":"api-gateway"`,
		`"event":"grpc_request"`,
		`"method":"/nexusim.api/Test"`,
		`"code":"PermissionDenied"`,
		`"trace_id":"trace-1"`,
		`"request_id":"request-1"`,
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected log to contain %s, got %s", want, line)
		}
	}
	if strings.Contains(line, "should-not-be-logged") || strings.Contains(line, "authorization") {
		t.Fatalf("log leaked auth metadata: %s", line)
	}
}
