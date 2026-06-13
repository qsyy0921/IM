package rpc

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"strings"

	"google.golang.org/grpc/credentials"
)

type GRPCClientTLSConfig struct {
	CAFile         string
	ServerName     string
	ClientCertFile string
	ClientKeyFile  string
}

func (config GRPCClientTLSConfig) Enabled() bool {
	return strings.TrimSpace(config.CAFile) != "" ||
		strings.TrimSpace(config.ServerName) != "" ||
		strings.TrimSpace(config.ClientCertFile) != "" ||
		strings.TrimSpace(config.ClientKeyFile) != ""
}

func grpcClientTLSCredentials(
	config GRPCClientTLSConfig,
	caFileEnvName string,
	clientCertFileEnvName string,
	clientKeyFileEnvName string,
) (credentials.TransportCredentials, error) {
	caFile := strings.TrimSpace(config.CAFile)
	if caFile == "" {
		return nil, errors.New(caFileEnvName + " is required when service TLS is configured")
	}
	clientCertFile := strings.TrimSpace(config.ClientCertFile)
	clientKeyFile := strings.TrimSpace(config.ClientKeyFile)
	if (clientCertFile == "") != (clientKeyFile == "") {
		return nil, errors.New(clientCertFileEnvName + " and " + clientKeyFileEnvName + " must be configured together")
	}
	pemBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New(caFileEnvName + " does not contain a valid PEM certificate")
	}
	tlsConfig := &tls.Config{
		RootCAs:    roots,
		ServerName: strings.TrimSpace(config.ServerName),
		MinVersion: tls.VersionTLS12,
	}
	if clientCertFile != "" {
		cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return credentials.NewTLS(tlsConfig), nil
}
