package monitoring

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	traceExporterStdout   = "stdout"
	traceExporterOTLPGRPC = "otlp-grpc"
)

type TraceConfig struct {
	Enabled       bool
	ServiceName   string
	Exporter      string
	OTLPEndpoint  string
	OTLPInsecure  bool
	SamplingRatio float64
}

type TraceRuntime struct {
	config   TraceConfig
	provider *sdktrace.TracerProvider
	tracer   oteltrace.Tracer
}

type TraceSnapshot struct {
	Enabled         bool    `json:"enabled"`
	ServiceName     string  `json:"service_name,omitempty"`
	Exporter        string  `json:"exporter,omitempty"`
	OTLPEndpointSet bool    `json:"otlp_endpoint_set,omitempty"`
	OTLPInsecure    bool    `json:"otlp_insecure,omitempty"`
	SamplingRatio   float64 `json:"sampling_ratio,omitempty"`
}

func NewTraceRuntime(ctx context.Context, config TraceConfig) (*TraceRuntime, error) {
	config = normalizeTraceConfig(config)
	runtime := &TraceRuntime{config: config}
	if !config.Enabled {
		return runtime, nil
	}
	exporter, err := newTraceExporter(ctx, config)
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.SamplingRatio)),
		sdktrace.WithResource(resource.NewWithAttributes(
			"",
			attribute.String("service.name", config.ServiceName),
		)),
	)
	runtime.provider = provider
	runtime.tracer = provider.Tracer(config.ServiceName)
	return runtime, nil
}

func (runtime *TraceRuntime) UnaryServerInterceptor() grpcgo.UnaryServerInterceptor {
	if runtime == nil || !runtime.config.Enabled || runtime.tracer == nil {
		return func(ctx context.Context, request any, _ *grpcgo.UnaryServerInfo, handler grpcgo.UnaryHandler) (any, error) {
			return handler(ctx, request)
		}
	}
	propagator := propagation.TraceContext{}
	return func(ctx context.Context, request any, info *grpcgo.UnaryServerInfo, handler grpcgo.UnaryHandler) (any, error) {
		ctx = propagator.Extract(ctx, grpcMetadataCarrier{metadataFromIncomingContext(ctx)})
		started := time.Now()
		ctx, span := runtime.tracer.Start(
			ctx,
			info.FullMethod,
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
			oteltrace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", info.FullMethod),
			),
		)
		defer span.End()

		response, err := handler(ctx, request)
		code := status.Code(err).String()
		span.SetAttributes(
			attribute.String("rpc.grpc.status_code", code),
			attribute.Int64("nexusim.grpc.latency_ms", time.Since(started).Milliseconds()),
		)
		traceID, requestID := grpcLogMetadata(ctx)
		if traceID != "" {
			span.SetAttributes(attribute.String("nexusim.trace_id", traceID))
		}
		if requestID != "" {
			span.SetAttributes(attribute.String("nexusim.request_id", requestID))
		}
		if err != nil {
			span.SetStatus(codes.Error, code)
			span.RecordError(err)
		}
		return response, err
	}
}

func (runtime *TraceRuntime) Shutdown(ctx context.Context) error {
	if runtime == nil || runtime.provider == nil {
		return nil
	}
	return runtime.provider.Shutdown(ctx)
}

func (runtime *TraceRuntime) Snapshot() TraceSnapshot {
	if runtime == nil {
		return TraceSnapshot{}
	}
	return TraceSnapshot{
		Enabled:         runtime.config.Enabled,
		ServiceName:     runtime.config.ServiceName,
		Exporter:        runtime.config.Exporter,
		OTLPEndpointSet: runtime.config.OTLPEndpoint != "",
		OTLPInsecure:    runtime.config.OTLPInsecure,
		SamplingRatio:   runtime.config.SamplingRatio,
	}
}

func normalizeTraceConfig(config TraceConfig) TraceConfig {
	config.ServiceName = strings.TrimSpace(config.ServiceName)
	if config.ServiceName == "" {
		config.ServiceName = serviceName
	}
	config.Exporter = strings.ToLower(strings.TrimSpace(config.Exporter))
	if config.Exporter == "" {
		config.Exporter = traceExporterStdout
	}
	config.OTLPEndpoint = strings.TrimSpace(config.OTLPEndpoint)
	if config.SamplingRatio <= 0 || config.SamplingRatio > 1 {
		config.SamplingRatio = 1
	}
	return config
}

func newTraceExporter(ctx context.Context, config TraceConfig) (sdktrace.SpanExporter, error) {
	switch config.Exporter {
	case traceExporterStdout:
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	case traceExporterOTLPGRPC:
		if config.OTLPEndpoint == "" {
			return nil, errors.New("NEXUSIM_CONTACTS_OTEL_TRACES_OTLP_ENDPOINT is required for otlp-grpc exporter")
		}
		options := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(config.OTLPEndpoint)}
		if config.OTLPInsecure {
			options = append(options, otlptracegrpc.WithInsecure())
		}
		return otlptracegrpc.New(ctx, options...)
	default:
		return nil, errors.New("unsupported contacts-service OpenTelemetry trace exporter")
	}
}

func metadataFromIncomingContext(ctx context.Context) metadata.MD {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return metadata.MD{}
	}
	return md
}

type grpcMetadataCarrier struct {
	md metadata.MD
}

func (carrier grpcMetadataCarrier) Get(key string) string {
	values := carrier.md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (carrier grpcMetadataCarrier) Set(key string, value string) {
	carrier.md.Set(key, value)
}

func (carrier grpcMetadataCarrier) Keys() []string {
	keys := make([]string, 0, len(carrier.md))
	for key := range carrier.md {
		keys = append(keys, key)
	}
	return keys
}
