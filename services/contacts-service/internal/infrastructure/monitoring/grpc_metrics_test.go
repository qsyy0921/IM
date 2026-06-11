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
