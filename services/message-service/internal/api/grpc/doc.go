// Package grpc adapts gRPC requests to app use cases.
//
// This layer owns protocol parsing, response mapping, and transport-level
// error conversion. It must not contain message business rules or SQL/Kafka
// implementation details.
package grpc
