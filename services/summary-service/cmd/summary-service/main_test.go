package main

import (
	"io"
	"net/http"
	"testing"
)

func TestValidateSummaryServiceMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc"} {
		if err := validateSummaryServiceMode(mode); err != nil {
			t.Fatalf("mode %s should be valid: %v", mode, err)
		}
	}
	if err := validateSummaryServiceMode("bad"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestSummaryProviderFromEnvDefaultsToExtractive(t *testing.T) {
	t.Setenv("NEXUSIM_SUMMARY_PROVIDER_MODE", "")
	if _, err := summaryProviderFromEnv(); err != nil {
		t.Fatalf("default provider should be valid: %v", err)
	}
}

func TestSummaryProviderFromEnvRequiresExternalEndpoint(t *testing.T) {
	t.Setenv("NEXUSIM_SUMMARY_PROVIDER_MODE", "external-http")
	t.Setenv("NEXUSIM_SUMMARY_LLM_ENDPOINT", "")
	if _, err := summaryProviderFromEnv(); err == nil {
		t.Fatal("expected missing external endpoint error")
	}
}

func TestSummaryProviderFromEnvAllowsLocalExternalEndpoint(t *testing.T) {
	t.Setenv("NEXUSIM_SUMMARY_PROVIDER_MODE", "external-http")
	t.Setenv("NEXUSIM_SUMMARY_LLM_ENDPOINT", "http://127.0.0.1:18081/llm")
	if _, err := summaryProviderFromEnv(); err != nil {
		t.Fatalf("local external endpoint should be valid: %v", err)
	}
}

func TestSummaryProviderFromEnvRejectsUnsupportedMode(t *testing.T) {
	t.Setenv("NEXUSIM_SUMMARY_PROVIDER_MODE", "bad")
	if _, err := summaryProviderFromEnv(); err == nil {
		t.Fatal("expected unsupported provider mode error")
	}
}

func TestValidateSummaryDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	for _, addr := range []string{"", "localhost:11921", "127.0.0.1:11921", "172.30.80.24:11921"} {
		if err := validateSummaryDebugListenerConfig(addr, false); err != nil {
			t.Fatalf("addr %s should be accepted: %v", addr, err)
		}
	}
}

func TestValidateSummaryDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateSummaryDebugListenerConfig("8.8.8.8:11921", false); err == nil {
		t.Fatal("expected public listener rejection")
	}
}

func TestValidateSummaryDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateSummaryDebugListenerConfig("8.8.8.8:11921", true); err != nil {
		t.Fatalf("expected public listener override: %v", err)
	}
}

func TestNewDebugHandlerExposesMetrics(t *testing.T) {
	server := http.Server{Handler: newDebugHandler()}
	request, err := http.NewRequest(http.MethodGet, "/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &responseRecorder{header: http.Header{}}
	server.Handler.ServeHTTP(recorder, request)
	if recorder.status != 0 && recorder.status != http.StatusOK {
		t.Fatalf("unexpected status %d", recorder.status)
	}
	body := string(recorder.body)
	if body != "nexusim_summary_service_info 1\n" {
		t.Fatalf("unexpected body %q", body)
	}
}

type responseRecorder struct {
	header http.Header
	status int
	body   string
}

func (recorder *responseRecorder) Header() http.Header {
	return recorder.header
}

func (recorder *responseRecorder) WriteHeader(statusCode int) {
	recorder.status = statusCode
}

func (recorder *responseRecorder) Write(bytes []byte) (int, error) {
	recorder.body += string(bytes)
	return len(bytes), nil
}

var _ http.ResponseWriter = (*responseRecorder)(nil)
var _ io.Writer = (*responseRecorder)(nil)
