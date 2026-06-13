package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	apigrpc "github.com/qsyy0921/IM/services/api-gateway/internal/api/grpc"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_MODE"))
	switch mode {
	case "", "noop":
		log.Println("api-gateway runtime wiring is idle; set NEXUSIM_API_GATEWAY_MODE=grpc")
		return nil
	case "grpc":
		return runGRPC()
	default:
		return errors.New("unsupported NEXUSIM_API_GATEWAY_MODE")
	}
}

func runGRPC() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	authenticator, err := newAuthenticatorFromEnv()
	if err != nil {
		return err
	}
	defer authenticator.Close()

	conversationConn, err := dialBackend(
		envString("NEXUSIM_API_GATEWAY_CONVERSATION_ADDR", "127.0.0.1:10496"),
		grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_CONVERSATION_TLS"),
	)
	if err != nil {
		return err
	}
	defer conversationConn.Close()
	messageConn, err := dialBackend(
		envString("NEXUSIM_API_GATEWAY_MESSAGE_ADDR", "127.0.0.1:10495"),
		grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_MESSAGE_TLS"),
	)
	if err != nil {
		return err
	}
	defer messageConn.Close()
	deliveryConn, err := dialBackend(
		envString("NEXUSIM_API_GATEWAY_DELIVERY_ADDR", "127.0.0.1:10497"),
		grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_DELIVERY_TLS"),
	)
	if err != nil {
		return err
	}
	defer deliveryConn.Close()
	receiptConn, err := dialBackend(
		envString("NEXUSIM_API_GATEWAY_RECEIPT_ADDR", "127.0.0.1:10499"),
		grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_RECEIPT_TLS"),
	)
	if err != nil {
		return err
	}
	defer receiptConn.Close()

	gateway := apigrpc.NewServer(apigrpc.Config{
		Authenticator: authenticator,
		Conversation:  conversationv1.NewConversationServiceClient(conversationConn),
		Message:       messagev1.NewMessageServiceClient(messageConn),
		Delivery:      deliveryv1.NewDeliveryServiceClient(deliveryConn),
		Receipt:       receiptv1.NewReceiptServiceClient(receiptConn),
	})
	server := grpcgo.NewServer()
	apigrpc.Register(server, gateway)

	addr := envString("NEXUSIM_API_GATEWAY_GRPC_ADDR", "0.0.0.0:12000")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()
	log.Printf("api-gateway gRPC listening on %s", addr)
	if err := server.Serve(listener); err != nil && !errors.Is(err, grpcgo.ErrServerStopped) {
		return err
	}
	return ctx.Err()
}

func dialBackend(addr string, tlsConfig grpcClientTLSConfig) (*grpcgo.ClientConn, error) {
	transportCredentials := grpcgo.WithTransportCredentials(insecure.NewCredentials())
	if tlsConfig.Enabled() {
		creds, err := grpcClientTLSCredentials(tlsConfig)
		if err != nil {
			return nil, err
		}
		transportCredentials = grpcgo.WithTransportCredentials(creds)
	}
	return grpcgo.NewClient("passthrough:///"+addr, transportCredentials)
}

func newAuthenticatorFromEnv() (*gatewayauth.Authenticator, error) {
	jwksJSON, err := loadJWKSetJSON()
	if err != nil {
		return nil, err
	}
	return gatewayauth.NewAuthenticator(gatewayauth.Config{
		Mode:               gatewayauth.Mode(envString("NEXUSIM_API_GATEWAY_AUTH_MODE", "hmac")),
		Secret:             os.Getenv("NEXUSIM_API_GATEWAY_AUTH_HMAC_SECRET"),
		PreviousSecrets:    splitCSV(os.Getenv("NEXUSIM_API_GATEWAY_AUTH_HMAC_PREVIOUS_SECRETS")),
		JWKSetJSON:         jwksJSON,
		JWKSetURL:          os.Getenv("NEXUSIM_API_GATEWAY_AUTH_JWKS_URL"),
		JWKRefreshInterval: envDuration("NEXUSIM_API_GATEWAY_AUTH_JWKS_REFRESH_INTERVAL", 5*time.Minute),
		TrustedIssuers:     splitCSV(os.Getenv("NEXUSIM_API_GATEWAY_AUTH_TRUSTED_ISSUERS")),
		Audience:           envString("NEXUSIM_API_GATEWAY_AUTH_AUDIENCE", "push-gateway"),
	})
}

func loadJWKSetJSON() (string, error) {
	if value := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_AUTH_JWKS_JSON")); value != "" {
		return value, nil
	}
	path := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_AUTH_JWKS_FILE"))
	if path == "" {
		return "", nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func envString(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func splitCSV(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

type grpcClientTLSConfig struct {
	EnvPrefix      string
	CAFile         string
	ServerName     string
	ClientCertFile string
	ClientKeyFile  string
}

func grpcClientTLSConfigFromEnv(envPrefix string) grpcClientTLSConfig {
	envPrefix = strings.TrimSpace(envPrefix)
	return grpcClientTLSConfig{
		EnvPrefix:      envPrefix,
		CAFile:         envString(envPrefix+"_CA_FILE", ""),
		ServerName:     envString(envPrefix+"_SERVER_NAME", ""),
		ClientCertFile: envString(envPrefix+"_CLIENT_CERT_FILE", ""),
		ClientKeyFile:  envString(envPrefix+"_CLIENT_KEY_FILE", ""),
	}
}

func (config grpcClientTLSConfig) Enabled() bool {
	return strings.TrimSpace(config.CAFile) != "" ||
		strings.TrimSpace(config.ServerName) != "" ||
		strings.TrimSpace(config.ClientCertFile) != "" ||
		strings.TrimSpace(config.ClientKeyFile) != ""
}

func grpcClientTLSCredentials(config grpcClientTLSConfig) (credentials.TransportCredentials, error) {
	caFile := strings.TrimSpace(config.CAFile)
	if caFile == "" {
		return nil, errors.New(config.EnvPrefix + "_CA_FILE is required when service TLS is configured")
	}
	clientCertFile := strings.TrimSpace(config.ClientCertFile)
	clientKeyFile := strings.TrimSpace(config.ClientKeyFile)
	if (clientCertFile == "") != (clientKeyFile == "") {
		return nil, errors.New(config.EnvPrefix + "_CLIENT_CERT_FILE and " + config.EnvPrefix + "_CLIENT_KEY_FILE must be configured together")
	}
	pemBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New(config.EnvPrefix + "_CA_FILE does not contain a valid PEM certificate")
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
