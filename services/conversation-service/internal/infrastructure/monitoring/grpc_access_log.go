package monitoring

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	metadataTraceID          = "x-nexusim-trace-id"
	metadataRequestID        = "x-nexusim-request-id"
	maxGRPCLogMetadataLength = 128
	serviceName              = "conversation-service"
)

func UnaryAccessLogInterceptor(logger *log.Logger) grpcgo.UnaryServerInterceptor {
	if logger == nil {
		logger = log.Default()
	}
	return func(ctx context.Context, request any, info *grpcgo.UnaryServerInfo, handler grpcgo.UnaryHandler) (any, error) {
		started := time.Now()
		response, err := handler(ctx, request)
		traceID, requestID := grpcLogMetadata(ctx)
		writeGRPCRequestLog(logger, info.FullMethod, status.Code(err).String(), time.Since(started).Milliseconds(), traceID, requestID)
		return response, err
	}
}

type grpcRequestLog struct {
	Service   string `json:"service"`
	Event     string `json:"event"`
	Method    string `json:"method"`
	Code      string `json:"code"`
	LatencyMS int64  `json:"latency_ms"`
	TraceID   string `json:"trace_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func writeGRPCRequestLog(logger *log.Logger, method string, code string, latencyMS int64, traceID string, requestID string) {
	line, err := json.Marshal(grpcRequestLog{
		Service:   serviceName,
		Event:     "grpc_request",
		Method:    method,
		Code:      code,
		LatencyMS: latencyMS,
		TraceID:   traceID,
		RequestID: requestID,
	})
	if err != nil {
		logger.Printf("conversation-service grpc_request method=%s code=%s latency_ms=%d", method, code, latencyMS)
		return
	}
	logger.Print(string(line))
}

func grpcLogMetadata(ctx context.Context) (string, string) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ""
	}
	return firstGRPCLogMetadataValue(md, metadataTraceID), firstGRPCLogMetadataValue(md, metadataRequestID)
}

func firstGRPCLogMetadataValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return trimGRPCLogMetadata(values[0])
}

func trimGRPCLogMetadata(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxGRPCLogMetadataLength {
		return value
	}
	return string(runes[:maxGRPCLogMetadataLength])
}
