package monitoring

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
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

func (runtime *TraceRuntime) StartWebSocketConnection(ctx context.Context, header http.Header, trace types.WebSocketTraceContext) (context.Context, func(error)) {
	if runtime == nil || !runtime.config.Enabled || runtime.tracer == nil {
		return ctx, func(error) {}
	}
	ctx = propagation.TraceContext{}.Extract(ctx, propagation.HeaderCarrier(header))
	ctx, span := runtime.tracer.Start(
		ctx,
		"push.websocket.session",
		oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		oteltrace.WithAttributes(
			attribute.String("network.protocol.name", "websocket"),
			attribute.String("nexusim.push.auth_mode", lowCardinalityValue(trace.AuthMode)),
			attribute.String("nexusim.push.route_backend", lowCardinalityValue(trace.RouteBackend)),
			attribute.Bool("nexusim.push.gateway_id_configured", strings.TrimSpace(trace.GatewayID) != ""),
			attribute.Bool("nexusim.push.tls_enabled", trace.TLSEnabled),
		),
	)
	return ctx, func(err error) {
		defer span.End()
		if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, types.ErrSessionEvicted) {
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "websocket session failed")
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
			return nil, errors.New("NEXUSIM_PUSH_OTEL_TRACES_OTLP_ENDPOINT is required for otlp-grpc exporter")
		}
		options := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(config.OTLPEndpoint)}
		if config.OTLPInsecure {
			options = append(options, otlptracegrpc.WithInsecure())
		}
		return otlptracegrpc.New(ctx, options...)
	default:
		return nil, errors.New("unsupported push-gateway OpenTelemetry trace exporter")
	}
}

func lowCardinalityValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	return value
}
