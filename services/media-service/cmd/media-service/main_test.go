package main

import (
	"testing"
	"time"
)

func TestValidateMediaServiceModeAllowsRuntimeModes(t *testing.T) {
	for _, mode := range []string{"noop", "grpc", "processing-worker", "outbox-relay"} {
		if err := validateMediaServiceMode(mode); err != nil {
			t.Fatalf("expected media service mode %q to be allowed: %v", mode, err)
		}
	}
	if err := validateMediaServiceMode("unknown"); err == nil {
		t.Fatalf("expected unsupported media service mode to fail")
	}
}

func TestMediaDebugAddrPrefersServiceSpecificEnv(t *testing.T) {
	t.Setenv("NEXUSIM_DEBUG_ADDR", "127.0.0.1:19100")
	t.Setenv("NEXUSIM_MEDIA_DEBUG_ADDR", "127.0.0.1:19101")

	if got := mediaDebugAddr(); got != "127.0.0.1:19101" {
		t.Fatalf("expected service-specific media debug addr, got %q", got)
	}
}

func TestValidateMediaDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1:11927", "localhost:11927", "172.30.80.30:11927"} {
		if err := validateMediaDebugListenerConfig(addr, false); err != nil {
			t.Fatalf("expected private debug addr %q to be allowed: %v", addr, err)
		}
	}
}

func TestValidateMediaDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateMediaDebugListenerConfig("0.0.0.0:11927", false); err == nil {
		t.Fatalf("expected public media debug addr to be rejected by default")
	}
}

func TestValidateMediaDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateMediaDebugListenerConfig("0.0.0.0:11927", true); err != nil {
		t.Fatalf("expected explicit public debug opt-in to pass: %v", err)
	}
}

func TestMediaOutboxRelayConfigHelpers(t *testing.T) {
	t.Setenv("NEXUSIM_MEDIA_OUTBOX_BATCH_SIZE", "25")
	t.Setenv("NEXUSIM_MEDIA_OUTBOX_POLL_INTERVAL", "250ms")
	t.Setenv("NEXUSIM_KAFKA_BROKERS", " kafka:29092, host.docker.internal:9092 ,, ")

	if got := envInt("NEXUSIM_MEDIA_OUTBOX_BATCH_SIZE", 500); got != 25 {
		t.Fatalf("expected env int override, got %d", got)
	}
	if got := envDuration("NEXUSIM_MEDIA_OUTBOX_POLL_INTERVAL", time.Second); got != 250*time.Millisecond {
		t.Fatalf("expected env duration override, got %s", got)
	}
	brokers := splitCSV(" kafka:29092, host.docker.internal:9092 ,, ")
	if len(brokers) != 2 || brokers[0] != "kafka:29092" || brokers[1] != "host.docker.internal:9092" {
		t.Fatalf("unexpected broker parsing: %#v", brokers)
	}
}
