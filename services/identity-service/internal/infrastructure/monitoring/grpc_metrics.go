package monitoring

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	metadataTraceID          = "x-nexusim-trace-id"
	metadataRequestID        = "x-nexusim-request-id"
	maxGRPCLogMetadataLength = 128
)

type GRPCMetrics struct {
	mu      sync.Mutex
	methods map[string]*grpcMethodMetrics
}

type grpcMethodMetrics struct {
	count          int64
	errorCount     int64
	totalLatencyMS int64
	maxLatencyMS   int64
	codes          map[string]int64
}

func NewGRPCMetrics() *GRPCMetrics {
	return &GRPCMetrics{methods: make(map[string]*grpcMethodMetrics)}
}

func (metrics *GRPCMetrics) UnaryServerInterceptor(logger *log.Logger) grpcgo.UnaryServerInterceptor {
	if metrics == nil {
		metrics = NewGRPCMetrics()
	}
	if logger == nil {
		logger = log.Default()
	}
	return func(ctx context.Context, request any, info *grpcgo.UnaryServerInfo, handler grpcgo.UnaryHandler) (any, error) {
		started := time.Now()
		response, err := handler(ctx, request)
		code := status.Code(err).String()
		latencyMS := time.Since(started).Milliseconds()
		metrics.record(info.FullMethod, code, latencyMS)
		traceID, requestID := grpcLogMetadata(ctx)
		writeGRPCRequestLog(logger, info.FullMethod, code, latencyMS, traceID, requestID)
		return response, err
	}
}

func (metrics *GRPCMetrics) Snapshot() GRPCSnapshot {
	if metrics == nil {
		return GRPCSnapshot{}
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	snapshot := GRPCSnapshot{Methods: make([]GRPCMethodSnapshot, 0, len(metrics.methods))}
	for method, methodMetrics := range metrics.methods {
		codes := make(map[string]int64, len(methodMetrics.codes))
		for code, count := range methodMetrics.codes {
			codes[code] = count
		}
		methodSnapshot := GRPCMethodSnapshot{
			Method:       method,
			Count:        methodMetrics.count,
			ErrorCount:   methodMetrics.errorCount,
			LatencyAvgMS: averageLatency(methodMetrics.totalLatencyMS, methodMetrics.count),
			LatencyMaxMS: methodMetrics.maxLatencyMS,
			Codes:        codes,
		}
		snapshot.TotalRequests += methodMetrics.count
		snapshot.TotalErrors += methodMetrics.errorCount
		snapshot.Methods = append(snapshot.Methods, methodSnapshot)
	}
	return snapshot
}

func (metrics *GRPCMetrics) record(method string, code string, latencyMS int64) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	methodMetrics := metrics.methods[method]
	if methodMetrics == nil {
		methodMetrics = &grpcMethodMetrics{codes: make(map[string]int64)}
		metrics.methods[method] = methodMetrics
	}
	methodMetrics.count++
	methodMetrics.codes[code]++
	if code != "OK" {
		methodMetrics.errorCount++
	}
	methodMetrics.totalLatencyMS += latencyMS
	if latencyMS > methodMetrics.maxLatencyMS {
		methodMetrics.maxLatencyMS = latencyMS
	}
}

type GRPCSnapshot struct {
	TotalRequests int64                `json:"total_requests"`
	TotalErrors   int64                `json:"total_errors"`
	Methods       []GRPCMethodSnapshot `json:"methods"`
}

type GRPCMethodSnapshot struct {
	Method       string           `json:"method"`
	Count        int64            `json:"count"`
	ErrorCount   int64            `json:"error_count"`
	LatencyAvgMS int64            `json:"latency_avg_ms"`
	LatencyMaxMS int64            `json:"latency_max_ms"`
	Codes        map[string]int64 `json:"codes"`
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
		logger.Printf("identity-service grpc_request method=%s code=%s latency_ms=%d", method, code, latencyMS)
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

func averageLatency(total int64, count int64) int64 {
	if count <= 0 {
		return 0
	}
	return total / count
}
