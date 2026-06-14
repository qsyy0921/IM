package monitoring

import (
	"context"
	"testing"

	gatewaytypes "github.com/qsyy0921/IM/services/api-gateway/internal/types"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestTraceRuntimeDisabledIsNoop(t *testing.T) {
	runtime, err := NewTraceRuntime(context.Background(), TraceConfig{})
	if err != nil {
		t.Fatalf("new disabled trace runtime: %v", err)
	}
	called := false
	_, err = runtime.UnaryServerInterceptor()(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.api/Test"}, func(context.Context, any) (any, error) {
		called = true
		return "ok", nil
	})
	if err != nil || !called {
		t.Fatalf("disabled trace runtime should call handler, called=%t err=%v", called, err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Enabled {
		t.Fatalf("expected disabled trace snapshot, got %+v", snapshot)
	}
}

func TestTraceRuntimeRecordsServerSpanWithTraceparent(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	runtime := &TraceRuntime{
		config: TraceConfig{
			Enabled:       true,
			ServiceName:   serviceName,
			Exporter:      "test",
			SamplingRatio: 1,
		},
		provider: provider,
		tracer:   provider.Tracer(serviceName),
	}
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown test tracer provider: %v", err)
		}
	}()

	ctx, _ := gatewaytypes.ContextWithCorrelation(metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		metadataRequestID, "request-incoming",
		"authorization", "Bearer should-not-be-exported",
	)))
	_, err := runtime.UnaryServerInterceptor()(ctx, nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.gateway.v1.GatewayService/SendMessage"}, func(ctx context.Context, _ any) (any, error) {
		gatewaytypes.PublishCorrelation(ctx, "trace-final", "request-final")
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("flush spans: %v", err)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	span := spans[0]
	if span.Name != "/nexusim.gateway.v1.GatewayService/SendMessage" {
		t.Fatalf("unexpected span name: %s", span.Name)
	}
	if got := span.Parent.TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("expected traceparent parent trace, got %s", got)
	}
	for key, want := range map[attribute.Key]string{
		"rpc.system":           "grpc",
		"rpc.method":           "/nexusim.gateway.v1.GatewayService/SendMessage",
		"rpc.grpc.status_code": "PermissionDenied",
	} {
		if got := spanAttributeString(span.Attributes, key); got != want {
			t.Fatalf("expected span attribute %s=%q, got %q", key, want, got)
		}
	}
	for _, forbidden := range []attribute.Key{"nexusim.trace_id", "nexusim.request_id"} {
		if got := spanAttributeString(span.Attributes, forbidden); got != "" {
			t.Fatalf("span must not export high-cardinality correlation attribute %s=%q", forbidden, got)
		}
	}
	for _, attribute := range span.Attributes {
		if attribute.Value.AsString() == "should-not-be-exported" || string(attribute.Key) == "authorization" {
			t.Fatalf("span leaked auth metadata: %+v", span.Attributes)
		}
	}
}

func TestTraceRuntimeRecordsClientSpanAndInjectsTraceparent(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	runtime := &TraceRuntime{
		config: TraceConfig{
			Enabled:       true,
			ServiceName:   serviceName,
			Exporter:      "test",
			SamplingRatio: 1,
		},
		provider: provider,
		tracer:   provider.Tracer(serviceName),
	}
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown test tracer provider: %v", err)
		}
	}()

	ctx, _ := gatewaytypes.ContextWithCorrelation(metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		metadataRequestID, "request-outgoing",
		"authorization", "Bearer should-not-be-exported",
	)))
	var injectedTraceparent string
	err := runtime.UnaryClientInterceptor()(ctx, "/nexusim.message.v1.MessageService/SendMessage", nil, nil, nil, func(ctx context.Context, _ string, _ any, _ any, _ *grpcgo.ClientConn, _ ...grpcgo.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			t.Fatalf("expected outgoing metadata")
		}
		values := md.Get("traceparent")
		if len(values) != 1 || values[0] == "" {
			t.Fatalf("expected injected traceparent metadata, got %v", values)
		}
		injectedTraceparent = values[0]
		gatewaytypes.PublishCorrelation(ctx, "trace-client-final", "request-client-final")
		return status.Error(codes.Unavailable, "backend unavailable")
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
	if injectedTraceparent == "" {
		t.Fatalf("expected traceparent to be captured")
	}
	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("flush spans: %v", err)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	span := spans[0]
	if span.Name != "/nexusim.message.v1.MessageService/SendMessage" {
		t.Fatalf("unexpected span name: %s", span.Name)
	}
	if got := spanAttributeString(span.Attributes, "rpc.system"); got != "grpc" {
		t.Fatalf("expected rpc.system=grpc, got %q", got)
	}
	if got := spanAttributeString(span.Attributes, "rpc.grpc.status_code"); got != "Unavailable" {
		t.Fatalf("expected grpc status Unavailable, got %q", got)
	}
	for _, forbidden := range []attribute.Key{"nexusim.trace_id", "nexusim.request_id"} {
		if got := spanAttributeString(span.Attributes, forbidden); got != "" {
			t.Fatalf("span must not export high-cardinality correlation attribute %s=%q", forbidden, got)
		}
	}
	for _, attribute := range span.Attributes {
		if attribute.Value.AsString() == "should-not-be-exported" || string(attribute.Key) == "authorization" {
			t.Fatalf("span leaked auth metadata: %+v", span.Attributes)
		}
	}
}

func TestTraceRuntimeRejectsOTLPExporterWithoutEndpoint(t *testing.T) {
	_, err := NewTraceRuntime(context.Background(), TraceConfig{
		Enabled:       true,
		ServiceName:   serviceName,
		Exporter:      traceExporterOTLPGRPC,
		SamplingRatio: 1,
	})
	if err == nil {
		t.Fatalf("expected otlp-grpc exporter without endpoint to fail")
	}
}

func spanAttributeString(attributes []attribute.KeyValue, key attribute.Key) string {
	for _, item := range attributes {
		if item.Key == key {
			return item.Value.AsString()
		}
	}
	return ""
}
