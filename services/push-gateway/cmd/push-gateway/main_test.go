package main

import (
	"strings"
	"testing"
)

func TestLoadRedisClientConfigSingleDefaults(t *testing.T) {
	t.Setenv("NEXUSIM_PUSH_REDIS_MODE", "")
	t.Setenv("NEXUSIM_PUSH_REDIS_ADDR", "")
	t.Setenv("NEXUSIM_PUSH_REDIS_DB", "")

	config := loadRedisClientConfigFromEnv()
	if config.Mode != "single" {
		t.Fatalf("expected default mode single, got %q", config.Mode)
	}
	if config.Addr != "127.0.0.1:6379" {
		t.Fatalf("expected default addr, got %q", config.Addr)
	}
	if config.DB != 0 {
		t.Fatalf("expected default db 0, got %d", config.DB)
	}
}

func TestLoadRedisClientConfigSentinel(t *testing.T) {
	t.Setenv("NEXUSIM_PUSH_REDIS_MODE", "sentinel")
	t.Setenv("NEXUSIM_PUSH_REDIS_SENTINEL_ADDRS", "127.0.0.1:26379, 127.0.0.1:26380")
	t.Setenv("NEXUSIM_PUSH_REDIS_SENTINEL_MASTER_NAME", "mymaster")
	t.Setenv("NEXUSIM_PUSH_REDIS_USERNAME", "redis-user")
	t.Setenv("NEXUSIM_PUSH_REDIS_PASSWORD", "redis-pass")
	t.Setenv("NEXUSIM_PUSH_REDIS_SENTINEL_USERNAME", "sentinel-user")
	t.Setenv("NEXUSIM_PUSH_REDIS_SENTINEL_PASSWORD", "sentinel-pass")
	t.Setenv("NEXUSIM_PUSH_REDIS_DB", "2")

	config := loadRedisClientConfigFromEnv()
	if config.Mode != "sentinel" {
		t.Fatalf("expected sentinel mode, got %q", config.Mode)
	}
	if len(config.SentinelAddrs) != 2 ||
		config.SentinelAddrs[0] != "127.0.0.1:26379" ||
		config.SentinelAddrs[1] != "127.0.0.1:26380" {
		t.Fatalf("unexpected sentinel addrs: %#v", config.SentinelAddrs)
	}
	if config.SentinelMasterName != "mymaster" {
		t.Fatalf("unexpected sentinel master name: %q", config.SentinelMasterName)
	}
	if config.Username != "redis-user" || config.Password != "redis-pass" {
		t.Fatalf("unexpected redis auth: %q/%q", config.Username, config.Password)
	}
	if config.SentinelUsername != "sentinel-user" || config.SentinelPassword != "sentinel-pass" {
		t.Fatalf("unexpected sentinel auth: %q/%q", config.SentinelUsername, config.SentinelPassword)
	}
	if config.DB != 2 {
		t.Fatalf("expected db 2, got %d", config.DB)
	}
}

func TestNewRedisUniversalClientValidatesSentinelConfig(t *testing.T) {
	if _, err := newRedisUniversalClient(redisClientConfig{
		Mode:               "sentinel",
		SentinelMasterName: "",
		SentinelAddrs:      []string{"127.0.0.1:26379"},
	}); err == nil {
		t.Fatalf("expected missing sentinel master name error")
	}

	if _, err := newRedisUniversalClient(redisClientConfig{
		Mode:               "sentinel",
		SentinelMasterName: "mymaster",
	}); err == nil {
		t.Fatalf("expected missing sentinel addrs error")
	}

	client, err := newRedisUniversalClient(redisClientConfig{
		Mode:               "sentinel",
		SentinelMasterName: "mymaster",
		SentinelAddrs:      []string{"127.0.0.1:26379"},
	})
	if err != nil {
		t.Fatalf("expected sentinel client, got error: %v", err)
	}
	_ = client.Close()
}

func TestNewRedisUniversalClientRejectsUnsupportedMode(t *testing.T) {
	if _, err := newRedisUniversalClient(redisClientConfig{Mode: "cluster"}); err == nil {
		t.Fatalf("expected unsupported mode error")
	} else if !strings.Contains(err.Error(), "cluster") {
		t.Fatalf("expected unsupported mode error to include mode value, got %v", err)
	}
}

func TestDeliveryClientTLSConfigFromEnvDisabledByDefault(t *testing.T) {
	clearDeliveryClientTLSEnv(t)

	config, err := deliveryClientTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load delivery tls config: %v", err)
	}
	if config.Enabled() {
		t.Fatalf("expected delivery tls to be disabled by default")
	}
}

func TestDeliveryClientTLSConfigFromEnvRequiresCAFile(t *testing.T) {
	clearDeliveryClientTLSEnv(t)
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_SERVER_NAME", "delivery-service.nexusim.local")

	if _, err := deliveryClientTLSConfigFromEnv(); err == nil {
		t.Fatalf("expected missing CA file error")
	}
}

func TestDeliveryClientTLSConfigFromEnvRequiresClientKeyPair(t *testing.T) {
	clearDeliveryClientTLSEnv(t)
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE", "ca.pem")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE", "client.crt")

	if _, err := deliveryClientTLSConfigFromEnv(); err == nil {
		t.Fatalf("expected partial client key pair error")
	}
}

func TestDeliveryClientTLSConfigFromEnvLoadsCompleteConfig(t *testing.T) {
	clearDeliveryClientTLSEnv(t)
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE", "ca.pem")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_SERVER_NAME", "delivery-service.nexusim.local")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE", "client.crt")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_KEY_FILE", "client.key")

	config, err := deliveryClientTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load delivery tls config: %v", err)
	}
	if config.CAFile != "ca.pem" ||
		config.ServerName != "delivery-service.nexusim.local" ||
		config.ClientCertFile != "client.crt" ||
		config.ClientKeyFile != "client.key" {
		t.Fatalf("unexpected delivery tls config: %#v", config)
	}
}

func TestSplitCSVTrimsAndDropsEmptyValues(t *testing.T) {
	values := splitCSV(" old-1, ,old-2 , old-1 ")
	if len(values) != 3 || values[0] != "old-1" || values[1] != "old-2" || values[2] != "old-1" {
		t.Fatalf("unexpected values: %#v", values)
	}
	if values := splitCSV(" , , "); len(values) != 0 {
		t.Fatalf("expected empty values, got %#v", values)
	}
}

func clearDeliveryClientTLSEnv(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE", "")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_SERVER_NAME", "")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE", "")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_KEY_FILE", "")
}
