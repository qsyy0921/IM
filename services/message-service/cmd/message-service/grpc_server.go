package main

import (
	"crypto/tls"
	"errors"
	"log"
	"strings"

	grpcapi "github.com/qsyy0921/IM/services/message-service/internal/api/grpc"
	monitoringinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/monitoring"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func newGRPCServer() (*grpc.Server, error) {
	authMode := envString("NEXUSIM_MESSAGE_AUTH_MODE", "body")
	tlsConfig, ok, err := messageGRPCTLSConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return newGRPCServerWithConfig(authMode, tlsConfig, ok)
}

func newGRPCServerWithConfig(authMode string, tlsConfig *tls.Config, tlsEnabled bool, traceInterceptors ...grpc.UnaryServerInterceptor) (*grpc.Server, error) {
	interceptors := make([]grpc.UnaryServerInterceptor, 0, 3)
	interceptors = append(interceptors, monitoringinfra.UnaryAccessLogInterceptor(log.Default()))
	for _, interceptor := range traceInterceptors {
		if interceptor != nil {
			interceptors = append(interceptors, interceptor)
		}
	}
	switch strings.ToLower(strings.TrimSpace(authMode)) {
	case "body", "request", "legacy":
	case "metadata", "verified-metadata":
		interceptors = append(interceptors, grpcapi.VerifiedAuthUnaryInterceptor(true))
	default:
		return nil, errors.New("unsupported NEXUSIM_MESSAGE_AUTH_MODE")
	}
	serverOptions := make([]grpc.ServerOption, 0, 2)
	if len(interceptors) > 0 {
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(interceptors...))
	}
	if tlsEnabled {
		serverOptions = append(serverOptions, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	return grpc.NewServer(serverOptions...), nil
}
