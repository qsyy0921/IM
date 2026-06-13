package types

import "context"

type Correlation struct {
	TraceID   string
	RequestID string
}

type correlationKey struct{}

func ContextWithCorrelation(ctx context.Context) (context.Context, *Correlation) {
	correlation := &Correlation{}
	return context.WithValue(ctx, correlationKey{}, correlation), correlation
}

func PublishCorrelation(ctx context.Context, traceID string, requestID string) {
	correlation, ok := ctx.Value(correlationKey{}).(*Correlation)
	if !ok || correlation == nil {
		return
	}
	correlation.TraceID = traceID
	correlation.RequestID = requestID
}

func CorrelationFromContext(ctx context.Context) (Correlation, bool) {
	correlation, ok := ctx.Value(correlationKey{}).(*Correlation)
	if !ok || correlation == nil {
		return Correlation{}, false
	}
	return *correlation, true
}
