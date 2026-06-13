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
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTraceID, "trace-1",
		metadataRequestID, "request-1",
	))

	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.conversation.v1.ConversationService/GetSendContext"}, func(context.Context, any) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected grpc error: %v", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.TotalRequests != 1 || snapshot.TotalErrors != 0 || len(snapshot.Methods) != 1 {
		t.Fatalf("unexpected grpc snapshot: %+v", snapshot)
	}
	if snapshot.Methods[0].Codes[codes.OK.String()] != 1 {
		t.Fatalf("expected OK code count, got %+v", snapshot.Methods[0].Codes)
	}

	var entry grpcRequestLog
	if err := json.Unmarshal([]byte(strings.TrimSpace(logs.String())), &entry); err != nil {
		t.Fatalf("decode grpc request log: %v; raw=%s", err, logs.String())
	}
	if entry.TraceID != "trace-1" || entry.RequestID != "request-1" {
		t.Fatalf("unexpected grpc request log metadata: %+v", entry)
	}
}

func TestGRPCMetricsInterceptorMapsPlainErrorToUnknown(t *testing.T) {
	metrics := NewGRPCMetrics()
	interceptor := metrics.UnaryServerInterceptor(log.New(&bytes.Buffer{}, "", 0))

	_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: "/test.Service/Method"}, func(context.Context, any) (any, error) {
		return nil, errors.New("plain failure")
	})
	if err == nil {
		t.Fatal("expected plain failure")
	}

	snapshot := metrics.Snapshot()
	if snapshot.TotalRequests != 1 || snapshot.TotalErrors != 1 {
		t.Fatalf("unexpected grpc snapshot: %+v", snapshot)
	}
	if snapshot.Methods[0].Codes[status.Code(err).String()] != 1 {
		t.Fatalf("expected recorded status code, got %+v", snapshot.Methods[0].Codes)
	}
}
