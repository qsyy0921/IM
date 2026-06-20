package main

import "testing"

func TestValidateNotificationServiceMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc", "delivery-worker", "outbox-relay"} {
		if err := validateNotificationServiceMode(mode); err != nil {
			t.Fatalf("mode %s should be valid: %v", mode, err)
		}
	}
	if err := validateNotificationServiceMode("provider-worker"); err == nil {
		t.Fatal("unsupported mode should fail")
	}
}

func TestValidateNotificationDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validateNotificationDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty debug listener should pass: %v", err)
	}
	if err := validateNotificationDebugListenerConfig("127.0.0.1:11928", false); err != nil {
		t.Fatalf("loopback debug listener should pass: %v", err)
	}
	if err := validateNotificationDebugListenerConfig("172.30.80.31:11928", false); err != nil {
		t.Fatalf("private debug listener should pass: %v", err)
	}
}

func TestValidateNotificationDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateNotificationDebugListenerConfig("0.0.0.0:11928", false); err == nil {
		t.Fatal("public debug listener should fail without allow flag")
	}
}

func TestValidateNotificationDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateNotificationDebugListenerConfig("0.0.0.0:11928", true); err != nil {
		t.Fatalf("public debug listener should pass with allow flag: %v", err)
	}
}

func TestNotificationProviderFromEnvSupportsWebhook(t *testing.T) {
	t.Setenv("NEXUSIM_NOTIFICATION_PROVIDER_MODE", "WEBHOOK")
	t.Setenv("NEXUSIM_NOTIFICATION_PROVIDER_ID", "provider-webhook-1")
	t.Setenv("NEXUSIM_NOTIFICATION_WEBHOOK_URL", "http://127.0.0.1/provider")
	provider, classifier, providerID, err := notificationProviderFromEnv()
	if err != nil {
		t.Fatalf("webhook provider should be valid: %v", err)
	}
	if provider == nil || classifier == nil || providerID != "provider-webhook-1" {
		t.Fatalf("unexpected webhook provider wiring provider=%T classifier=%T id=%q", provider, classifier, providerID)
	}
}

func TestNotificationProviderFromEnvRejectsUnsupportedProvider(t *testing.T) {
	t.Setenv("NEXUSIM_NOTIFICATION_PROVIDER_MODE", "smtp")
	if _, _, _, err := notificationProviderFromEnv(); err == nil {
		t.Fatal("unsupported notification provider should fail")
	}
}
