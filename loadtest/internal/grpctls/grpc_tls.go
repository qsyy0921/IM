package grpctls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	CAFile         string
	ServerName     string
	ClientCertFile string
	ClientKeyFile  string
}

func (config Config) Enabled() bool {
	return strings.TrimSpace(config.CAFile) != "" ||
		strings.TrimSpace(config.ServerName) != "" ||
		strings.TrimSpace(config.ClientCertFile) != "" ||
		strings.TrimSpace(config.ClientKeyFile) != ""
}

func DialOption(config Config, flagPrefix string) (grpc.DialOption, error) {
	if !config.Enabled() {
		return grpc.WithTransportCredentials(insecure.NewCredentials()), nil
	}
	creds, err := TransportCredentials(config, flagPrefix)
	if err != nil {
		return nil, err
	}
	return grpc.WithTransportCredentials(creds), nil
}

func TransportCredentials(config Config, flagPrefix string) (credentials.TransportCredentials, error) {
	flagPrefix = strings.TrimSpace(flagPrefix)
	if flagPrefix == "" {
		flagPrefix = "grpc-tls"
	}
	caFile := strings.TrimSpace(config.CAFile)
	if caFile == "" {
		return nil, errors.New("--" + flagPrefix + "-ca-file is required when gRPC TLS is configured")
	}
	clientCertFile := strings.TrimSpace(config.ClientCertFile)
	clientKeyFile := strings.TrimSpace(config.ClientKeyFile)
	if (clientCertFile == "") != (clientKeyFile == "") {
		return nil, errors.New("--" + flagPrefix + "-client-cert-file and --" + flagPrefix + "-client-key-file must be configured together")
	}
	pemBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New("--" + flagPrefix + "-ca-file does not contain a valid PEM certificate")
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
