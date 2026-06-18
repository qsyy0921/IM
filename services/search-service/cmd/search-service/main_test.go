package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchServiceModeFromEnvDefaultsToNoop(t *testing.T) {
	t.Setenv("NEXUSIM_SEARCH_SERVICE_MODE", "")

	if mode := searchServiceModeFromEnv(); mode != "noop" {
		t.Fatalf("expected default noop mode, got %q", mode)
	}
}

func TestValidateSearchServiceModeAcceptsSupportedModes(t *testing.T) {
	for _, mode := range []string{"noop", "grpc"} {
		if err := validateSearchServiceMode(mode); err != nil {
			t.Fatalf("expected %s mode to be accepted: %v", mode, err)
		}
	}
}

func TestValidateSearchServiceModeRejectsUnknownMode(t *testing.T) {
	if err := validateSearchServiceMode("timeline-consumer"); err == nil {
		t.Fatalf("expected unsupported search-service mode to fail")
	}
}

func TestSearchDebugAddrPrefersServiceSpecificEnv(t *testing.T) {
	t.Setenv("NEXUSIM_DEBUG_ADDR", "127.0.0.1:19100")
	t.Setenv("NEXUSIM_SEARCH_DEBUG_ADDR", "127.0.0.1:19101")

	if addr := searchDebugAddr(); addr != "127.0.0.1:19101" {
		t.Fatalf("expected service-specific debug addr to win, got %q", addr)
	}
}

func TestValidateSearchDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1:11917", "localhost:11917", "172.31.50.10:11917"} {
		if err := validateSearchDebugListenerConfig(addr, false); err != nil {
			t.Fatalf("expected debug listener %q to be allowed: %v", addr, err)
		}
	}
}

func TestValidateSearchDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:11917", ":11917", "8.8.8.8:11917"} {
		if err := validateSearchDebugListenerConfig(addr, false); err == nil {
			t.Fatalf("expected debug listener %q to be rejected by default", addr)
		}
	}
}

func TestValidateSearchDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateSearchDebugListenerConfig("0.0.0.0:11917", true); err != nil {
		t.Fatalf("expected explicit public debug listener opt-in to be allowed: %v", err)
	}
}

func TestDebugHandlerExposesRuntimeEndpoints(t *testing.T) {
	handler := newDebugHandler()
	for _, path := range []string{"/healthz", "/readyz", "/debug/metrics", "/metrics"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("expected %s to return 200, got %d", path, response.Code)
		}
		if response.Body.Len() == 0 {
			t.Fatalf("expected %s to return body", path)
		}
	}
}
