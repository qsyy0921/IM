// Package http adapts HTTP requests to app use cases.
//
// Phase 1 primarily uses gRPC; HTTP is reserved for api-gateway adaptation and
// local diagnostics. Business rules still belong to app/domain.
package http
