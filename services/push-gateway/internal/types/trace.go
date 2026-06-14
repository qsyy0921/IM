package types

type WebSocketTraceContext struct {
	AuthMode     string
	RouteBackend string
	GatewayID    string
	TLSEnabled   bool
}
