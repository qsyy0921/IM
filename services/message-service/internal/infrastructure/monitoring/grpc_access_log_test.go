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

func TestUnaryAccessLogInterceptorLogsTraceAndRequestIDs(t *testing.T) {
	var logs bytes.Buffer
	interceptor := UnaryAccessLogInterceptor(log.New(&logs, "", 0))
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTraceID, " trace-1 ",
		metadataRequestID, "request-1",
		"authorization", "Bearer should-not-be-logged",
	))

	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.message.v1.MessageService/SendMessage"}, func(context.Context, any) (any, error) {
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
		entry.Method != "/nexusim.message.v1.MessageService/SendMessage" ||
		entry.Code != codes.PermissionDenied.String() ||
		entry.TraceID != "trace-1" ||
		entry.RequestID != "request-1" {
		t.Fatalf("unexpected grpc request log: %+v", entry)
	}
	if strings.Contains(logs.String(), "should-not-be-logged") || strings.Contains(logs.String(), "authorization") {
		t.Fatalf("log leaked auth metadata: %s", logs.String())
	}
}

func TestUnaryAccessLogInterceptorMapsPlainErrorToUnknown(t *testing.T) {
	var logs bytes.Buffer
	interceptor := UnaryAccessLogInterceptor(log.New(&logs, "", 0))

	_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: "/test.Service/Method"}, func(context.Context, any) (any, error) {
		return nil, errors.New("plain failure")
	})
	if err == nil {
		t.Fatal("expected plain failure")
	}

	var entry grpcRequestLog
	if err := json.Unmarshal([]byte(strings.TrimSpace(logs.String())), &entry); err != nil {
		t.Fatalf("decode grpc request log: %v; raw=%s", err, logs.String())
	}
	if entry.Code != codes.Unknown.String() {
		t.Fatalf("expected Unknown code, got %+v", entry)
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
