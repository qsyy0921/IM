package main

import "testing"

func TestPolicyClientTLSConfigFromEnvDisabledByDefault(t *testing.T) {
	clearPolicyClientTLSConfig(t)
	config, err := policyClientTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load policy client tls config: %v", err)
	}
	if config.Enabled() {
		t.Fatalf("expected policy client tls to be disabled by default: %+v", config)
	}
}

func TestPolicyClientTLSConfigFromEnvRequiresCAFile(t *testing.T) {
	clearPolicyClientTLSConfig(t)
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_SERVER_NAME", "policy-service.nexusim.local")
	if _, err := policyClientTLSConfigFromEnv(); err == nil {
		t.Fatalf("expected policy client tls without CA file to fail")
	}
}

func TestPolicyClientTLSConfigFromEnvRequiresClientKeyPair(t *testing.T) {
	clearPolicyClientTLSConfig(t)
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CA_FILE", "ca.pem")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE", "client.crt")
	if _, err := policyClientTLSConfigFromEnv(); err == nil {
		t.Fatalf("expected partial policy client certificate config to fail")
	}
}

func TestPolicyClientTLSConfigFromEnvLoadsTLS(t *testing.T) {
	clearPolicyClientTLSConfig(t)
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CA_FILE", "ca.pem")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_SERVER_NAME", "policy-service.nexusim.local")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE", "client.crt")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_KEY_FILE", "client.key")
	config, err := policyClientTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load policy client tls config: %v", err)
	}
	if config.CAFile != "ca.pem" ||
		config.ServerName != "policy-service.nexusim.local" ||
		config.ClientCertFile != "client.crt" ||
		config.ClientKeyFile != "client.key" {
		t.Fatalf("unexpected policy client tls config: %+v", config)
	}
}

func clearPolicyClientTLSConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CA_FILE", "")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_SERVER_NAME", "")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE", "")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_KEY_FILE", "")
}
