package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/delivery-service/internal/infrastructure/postgres"
)

func TestLoadDeliveryGRPCCredentialsFromEnvDisabledByDefault(t *testing.T) {
	clearDeliveryGRPCTLSConfig(t)
	creds, ok, err := loadDeliveryGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load delivery grpc credentials: %v", err)
	}
	if ok || creds != nil {
		t.Fatalf("expected delivery grpc tls to be disabled by default, ok=%t creds=%T", ok, creds)
	}
}

func TestNewGRPCServerAcceptsMetadataAuthMode(t *testing.T) {
	clearDeliveryGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_DELIVERY_AUTH_MODE", "metadata")

	server, err := newGRPCServer(nil)
	if err != nil {
		t.Fatalf("new grpc server: %v", err)
	}
	server.Stop()
}

func TestNewGRPCServerRejectsUnsupportedAuthMode(t *testing.T) {
	clearDeliveryGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_DELIVERY_AUTH_MODE", "unknown")

	server, err := newGRPCServer(nil)
	if err == nil {
		if server != nil {
			server.Stop()
		}
		t.Fatalf("expected unsupported delivery auth mode to fail")
	}
}

func TestDeliveryTraceConfigDefaultsToDisabled(t *testing.T) {
	clearDeliveryTraceConfig(t)
	config, err := deliveryTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load delivery trace config: %v", err)
	}
	if config.Enabled ||
		config.ServiceName != "delivery-service" ||
		config.Exporter != "stdout" ||
		config.SamplingRatio != 1 {
		t.Fatalf("unexpected default trace config: %+v", config)
	}
}

func TestDeliveryTraceConfigLoadsOTLPGRPC(t *testing.T) {
	clearDeliveryTraceConfig(t)
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_ENABLED", "true")
	t.Setenv("NEXUSIM_DELIVERY_OTEL_SERVICE_NAME", "delivery-service-test")
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_EXPORTER", "otlp-grpc")
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_OTLP_ENDPOINT", "127.0.0.1:4317")
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_OTLP_INSECURE", "true")
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_SAMPLING_RATIO", "0.5")

	config, err := deliveryTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load delivery trace config: %v", err)
	}
	if !config.Enabled ||
		config.ServiceName != "delivery-service-test" ||
		config.Exporter != "otlp-grpc" ||
		config.OTLPEndpoint != "127.0.0.1:4317" ||
		!config.OTLPInsecure ||
		config.SamplingRatio != 0.5 {
		t.Fatalf("unexpected otlp trace config: %+v", config)
	}
}

func TestDeliveryTraceConfigRejectsInvalidValues(t *testing.T) {
	clearDeliveryTraceConfig(t)
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_ENABLED", "sometimes")
	if _, err := deliveryTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid enabled bool to fail")
	}

	clearDeliveryTraceConfig(t)
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_SAMPLING_RATIO", "2")
	if _, err := deliveryTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid sampling ratio to fail")
	}

	clearDeliveryTraceConfig(t)
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_OTLP_INSECURE", "sometimes")
	if _, err := deliveryTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid otlp insecure bool to fail")
	}
}

func TestValidateTrustedMetadataListenerConfigAllowsPrivateAddressWithoutMTLS(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"172.31.50.10:10497",
		"metadata",
		nil,
	)
	if err != nil {
		t.Fatalf("expected private address to be allowed without mTLS, got %v", err)
	}
}

func TestValidateTrustedMetadataListenerConfigRequiresMTLSForPublicAddress(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10497",
		"verified-metadata",
		nil,
	)
	if err == nil {
		t.Fatalf("expected public address without mTLS client cert to fail")
	}
}

func TestValidateTrustedMetadataListenerConfigAllowsMTLSForPublicAddress(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10497",
		"verified-metadata",
		&tls.Config{ClientAuth: tls.RequireAndVerifyClientCert},
	)
	if err != nil {
		t.Fatalf("expected public address with mTLS client cert to be allowed, got %v", err)
	}
}

func TestValidateTrustedMetadataListenerConfigIgnoresBodyAuth(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10497",
		"body",
		nil,
	)
	if err != nil {
		t.Fatalf("expected body auth to skip trusted metadata guard, got %v", err)
	}
}

func TestDeliveryDebugAddrPrefersServiceSpecificEnv(t *testing.T) {
	t.Setenv("NEXUSIM_DEBUG_ADDR", "127.0.0.1:19200")
	t.Setenv("NEXUSIM_DELIVERY_DEBUG_ADDR", "127.0.0.1:19201")

	if addr := deliveryDebugAddr(); addr != "127.0.0.1:19201" {
		t.Fatalf("expected service-specific debug addr to win, got %q", addr)
	}
}

func TestValidateDeliveryDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1:11907", "localhost:11907", "172.31.50.10:11907"} {
		if err := validateDeliveryDebugListenerConfig(addr, false); err != nil {
			t.Fatalf("expected delivery debug listener %q to be allowed: %v", addr, err)
		}
	}
}

func TestValidateDeliveryDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:11907", ":11907", "8.8.8.8:11907"} {
		if err := validateDeliveryDebugListenerConfig(addr, false); err == nil {
			t.Fatalf("expected delivery debug listener %q to be rejected by default", addr)
		}
	}
}

func TestValidateDeliveryDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateDeliveryDebugListenerConfig("0.0.0.0:11907", true); err != nil {
		t.Fatalf("expected explicit public delivery debug listener opt-in to be allowed: %v", err)
	}
}

func TestLoadDeliveryGRPCCredentialsFromEnvRequiresCertKeyPair(t *testing.T) {
	clearDeliveryGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CERT_FILE", "server.crt")
	if _, ok, err := loadDeliveryGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected partial delivery grpc tls config to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadDeliveryGRPCCredentialsFromEnvLoadsServerTLS(t *testing.T) {
	clearDeliveryGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeDeliveryTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_KEY_FILE", keyFile)

	tlsConfig, ok, err := deliveryGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load delivery grpc tls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected delivery grpc tls config, ok=%t", ok)
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 minimum, got %d", tlsConfig.MinVersion)
	}

	creds, ok, err := loadDeliveryGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load delivery grpc tls credentials: %v", err)
	}
	if !ok || creds == nil {
		t.Fatalf("expected delivery grpc tls credentials, ok=%t creds=%T", ok, creds)
	}
}

func TestLoadDeliveryGRPCCredentialsFromEnvRejectsInvalidRequireClientCert(t *testing.T) {
	clearDeliveryGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_REQUIRE_CLIENT_CERT", "sometimes")
	if _, ok, err := loadDeliveryGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected invalid delivery client-cert bool to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadDeliveryGRPCCredentialsFromEnvRequiresClientCAForMTLS(t *testing.T) {
	clearDeliveryGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeDeliveryTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_REQUIRE_CLIENT_CERT", "true")
	if _, ok, err := loadDeliveryGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected delivery mtls without ca to fail, ok=%t err=%v", ok, err)
	}
}

func TestDeliveryGRPCTLSConfigLoadsMTLS(t *testing.T) {
	clearDeliveryGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeDeliveryTLSTestCert(t, dir, "server")
	caFile, _ := writeDeliveryTLSTestCert(t, dir, "ca")
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_CA_FILE", caFile)

	tlsConfig, ok, err := deliveryGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load delivery grpc mtls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected delivery grpc mtls config, ok=%t", ok)
	}
	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected client cert verification, got %v", tlsConfig.ClientAuth)
	}
	if tlsConfig.ClientCAs == nil {
		t.Fatalf("expected client CA pool")
	}
}

func TestDeliveryGRPCTLSConfigAllowsClientIdentity(t *testing.T) {
	clearDeliveryGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeDeliveryTLSTestCert(t, dir, "server")
	caFile, _ := writeDeliveryTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeDeliveryTLSTestCertWithSANs(t, dir, "client", []string{"push-gateway.nexusim.local"}, []string{"spiffe://nexusim/push-gateway"})
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", " PUSH-GATEWAY.NEXUSIM.LOCAL ")
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_URIS", "spiffe://nexusim/other-client")

	tlsConfig, ok, err := deliveryGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load delivery grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readDeliveryTLSTestCert(t, clientCertFile)}}); err != nil {
		t.Fatalf("expected client identity to be allowed: %v", err)
	}
}

func TestDeliveryGRPCTLSConfigAllowsClientURIIdentity(t *testing.T) {
	clearDeliveryGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeDeliveryTLSTestCert(t, dir, "server")
	caFile, _ := writeDeliveryTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeDeliveryTLSTestCertWithSANs(t, dir, "client", nil, []string{"spiffe://nexusim/push-gateway"})
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_URIS", "spiffe://nexusim/push-gateway")

	tlsConfig, ok, err := deliveryGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load delivery grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readDeliveryTLSTestCert(t, clientCertFile)}}); err != nil {
		t.Fatalf("expected client uri identity to be allowed: %v", err)
	}
}

func TestDeliveryGRPCTLSConfigRejectsUnlistedClientIdentity(t *testing.T) {
	clearDeliveryGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeDeliveryTLSTestCert(t, dir, "server")
	caFile, _ := writeDeliveryTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeDeliveryTLSTestCertWithSANs(t, dir, "client", []string{"unknown.nexusim.local"}, nil)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "push-gateway.nexusim.local")

	tlsConfig, ok, err := deliveryGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load delivery grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readDeliveryTLSTestCert(t, clientCertFile)}}); err == nil {
		t.Fatalf("expected unlisted client identity to be rejected")
	}
}

func TestProjectionFailureCleanupConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("NEXUSIM_DELIVERY_PROJECTION_FAILURE_RETENTION", "")
	t.Setenv("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_BATCH_SIZE", "")

	config, err := projectionFailureCleanupConfigFromEnv()
	if err != nil {
		t.Fatalf("projection failure cleanup config: %v", err)
	}
	if config.Retention != 7*24*time.Hour {
		t.Fatalf("expected default retention, got %s", config.Retention)
	}
	if config.BatchSize != 5000 {
		t.Fatalf("expected default batch size, got %d", config.BatchSize)
	}
}

func TestProjectionFailureCleanupConfigFromEnvRejectsInvalidValues(t *testing.T) {
	t.Setenv("NEXUSIM_DELIVERY_PROJECTION_FAILURE_RETENTION", "0")
	t.Setenv("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_BATCH_SIZE", "0")

	if _, err := projectionFailureCleanupConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid projection failure cleanup config to fail")
	}
}

func TestWriteProjectionFailureAuditOutput(t *testing.T) {
	resolvedAt := time.Date(2026, 6, 16, 8, 30, 0, 0, time.UTC)
	resolvedOffset := int64(43)
	outputPath := filepath.Join(t.TempDir(), "projection-failure-audit.json")

	err := writeProjectionFailureAuditOutput(outputPath, []postgresinfra.ProjectionFailureAuditRow{
		{
			ConsumerGroup: "group-1",
			Topic:         "conversation.timeline.events",
			PartitionID:   0,
			OffsetValue:   41,
			EventID:       "event-1",
			EventType:     "message.revoked.v1",
			FailureClass:  "projection_dependency",
			FailureCount:  2,
			LastError:     "delivery projection dependency failed",
			LastSeenAt:    resolvedAt.Add(-time.Minute),
		},
		{
			ConsumerGroup:            "group-1",
			Topic:                    "conversation.timeline.events",
			PartitionID:              0,
			OffsetValue:              42,
			EventID:                  "event-2",
			EventType:                "message.edited.v1",
			FailureClass:             "db_write_failed",
			FailureCount:             1,
			LastError:                "delivery projection write failed",
			LastSeenAt:               resolvedAt,
			ResolvedAt:               &resolvedAt,
			ResolvedCheckpointOffset: &resolvedOffset,
		},
	}, true)
	if err != nil {
		t.Fatalf("write projection failure audit output: %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read projection failure audit output: %v", err)
	}
	var output struct {
		IncludeResolved bool `json:"include_resolved"`
		UnresolvedCount int  `json:"unresolved_count"`
		Rows            []struct {
			EventID                  string `json:"event_id"`
			LastError                string `json:"last_error"`
			Resolved                 bool   `json:"resolved"`
			ResolvedCheckpointOffset *int64 `json:"resolved_checkpoint_offset"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("decode projection failure audit output: %v", err)
	}
	if !output.IncludeResolved || output.UnresolvedCount != 1 || len(output.Rows) != 2 {
		t.Fatalf("unexpected projection failure audit output: %+v", output)
	}
	if output.Rows[0].EventID != "event-1" || output.Rows[0].Resolved || output.Rows[0].LastError != "delivery projection dependency failed" {
		t.Fatalf("unexpected unresolved row: %+v", output.Rows[0])
	}
	if output.Rows[1].EventID != "event-2" || !output.Rows[1].Resolved || output.Rows[1].ResolvedCheckpointOffset == nil || *output.Rows[1].ResolvedCheckpointOffset != resolvedOffset {
		t.Fatalf("unexpected resolved row: %+v", output.Rows[1])
	}
}

func TestWriteProjectionRepairAuditOutput(t *testing.T) {
	createdAt := time.Date(2026, 6, 16, 8, 45, 0, 0, time.UTC)
	failureOffset := int64(41)
	outputPath := filepath.Join(t.TempDir(), "projection-repair-audit.json")

	err := writeProjectionRepairAuditOutput(outputPath, []postgresinfra.ProjectionRepairAuditRow{
		{
			ConsumerGroup: "group-1",
			Topic:         "conversation.timeline.events",
			PartitionID:   0,
			Mode:          "rewind-unresolved-failure",
			Outcome:       "MUTATED",
			Operator:      "operator-1",
			Reason:        "replay blocked projection",
			DryRun:        false,
			BeforeOffset:  45,
			AfterOffset:   41,
			FailureOffset: &failureOffset,
			FailureEvent:  "event-1",
			FailureClass:  "projection_dependency",
			CreatedAt:     createdAt,
		},
		{
			ConsumerGroup: "group-1",
			Topic:         "conversation.timeline.events",
			PartitionID:   1,
			Mode:          "rewind-next-offset",
			Outcome:       "SKIPPED",
			SkipReason:    "target_offset_is_not_lower",
			Operator:      "operator-2",
			Reason:        "dry run",
			DryRun:        true,
			BeforeOffset:  10,
			AfterOffset:   10,
			CreatedAt:     createdAt.Add(time.Minute),
		},
	})
	if err != nil {
		t.Fatalf("write projection repair audit output: %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read projection repair audit output: %v", err)
	}
	var output struct {
		Rows []struct {
			Mode          string `json:"mode"`
			Outcome       string `json:"outcome"`
			SkipReason    string `json:"skip_reason"`
			BeforeOffset  int64  `json:"before_offset"`
			AfterOffset   int64  `json:"after_offset"`
			FailureOffset *int64 `json:"failure_offset"`
			FailureEvent  string `json:"failure_event_id"`
			FailureClass  string `json:"failure_class"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("decode projection repair audit output: %v", err)
	}
	if len(output.Rows) != 2 {
		t.Fatalf("unexpected projection repair audit output row count: %+v", output)
	}
	if output.Rows[0].Mode != "rewind-unresolved-failure" ||
		output.Rows[0].Outcome != "MUTATED" ||
		output.Rows[0].BeforeOffset != 45 ||
		output.Rows[0].AfterOffset != 41 ||
		output.Rows[0].FailureOffset == nil ||
		*output.Rows[0].FailureOffset != failureOffset ||
		output.Rows[0].FailureEvent != "event-1" ||
		output.Rows[0].FailureClass != "projection_dependency" {
		t.Fatalf("unexpected mutated repair row: %+v", output.Rows[0])
	}
	if output.Rows[1].Outcome != "SKIPPED" ||
		output.Rows[1].SkipReason != "target_offset_is_not_lower" ||
		output.Rows[1].FailureOffset != nil {
		t.Fatalf("unexpected skipped repair row: %+v", output.Rows[1])
	}
}

func clearDeliveryGRPCTLSConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CERT_FILE", "")
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_KEY_FILE", "")
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_CA_FILE", "")
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_REQUIRE_CLIENT_CERT", "")
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "")
	t.Setenv("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_URIS", "")
}

func clearDeliveryTraceConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_ENABLED", "")
	t.Setenv("NEXUSIM_DELIVERY_OTEL_SERVICE_NAME", "")
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_EXPORTER", "")
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_OTLP_ENDPOINT", "")
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_OTLP_INSECURE", "")
	t.Setenv("NEXUSIM_DELIVERY_OTEL_TRACES_SAMPLING_RATIO", "")
}

func writeDeliveryTLSTestCert(t *testing.T, dir string, name string) (string, string) {
	return writeDeliveryTLSTestCertWithSANs(t, dir, name, []string{"localhost"}, nil)
}

func writeDeliveryTLSTestCertWithSANs(t *testing.T, dir string, name string, dnsNames []string, uriNames []string) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate tls key: %v", err)
	}
	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		t.Fatalf("generate tls serial: %v", err)
	}
	uris := make([]*url.URL, 0, len(uriNames))
	for _, uriName := range uriNames {
		parsed, err := url.Parse(uriName)
		if err != nil {
			t.Fatalf("parse tls uri san: %v", err)
		}
		uris = append(uris, parsed)
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "delivery-" + name,
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              dnsNames,
		URIs:                  uris,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create tls cert: %v", err)
	}
	certFile := filepath.Join(dir, name+".crt")
	keyFile := filepath.Join(dir, name+".key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write tls cert: %v", err)
	}
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes}), 0o600); err != nil {
		t.Fatalf("write tls key: %v", err)
	}
	return certFile, keyFile
}

func readDeliveryTLSTestCert(t *testing.T, certFile string) *x509.Certificate {
	t.Helper()
	raw, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("read tls cert: %v", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatalf("decode tls cert")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse tls cert: %v", err)
	}
	return cert
}
