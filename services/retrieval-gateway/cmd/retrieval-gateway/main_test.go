package main

import (
	"io"
	"net/http"
	"testing"

	retrievaltypes "github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
)

func TestValidateRetrievalGatewayMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc"} {
		if err := validateRetrievalGatewayMode(mode); err != nil {
			t.Fatalf("mode %s should be valid: %v", mode, err)
		}
	}
	if err := validateRetrievalGatewayMode("bad"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestRetrievalGraphExpansionDepthFromEnv(t *testing.T) {
	t.Setenv("NEXUSIM_RETRIEVAL_GRAPH_EXPANSION_DEPTH", "")
	depth, err := retrievalGraphExpansionDepthFromEnv()
	if err != nil {
		t.Fatalf("default depth should be accepted: %v", err)
	}
	if depth != retrievaltypes.DefaultGraphExpansionDepth {
		t.Fatalf("expected default depth %d, got %d", retrievaltypes.DefaultGraphExpansionDepth, depth)
	}

	t.Setenv("NEXUSIM_RETRIEVAL_GRAPH_EXPANSION_DEPTH", "0")
	depth, err = retrievalGraphExpansionDepthFromEnv()
	if err != nil {
		t.Fatalf("depth zero should be accepted: %v", err)
	}
	if depth != 0 {
		t.Fatalf("expected depth zero, got %d", depth)
	}

	t.Setenv("NEXUSIM_RETRIEVAL_GRAPH_EXPANSION_DEPTH", "2")
	depth, err = retrievalGraphExpansionDepthFromEnv()
	if err != nil {
		t.Fatalf("depth two should be accepted: %v", err)
	}
	if depth != 2 {
		t.Fatalf("expected depth two, got %d", depth)
	}
}

func TestRetrievalGraphExpansionDepthFromEnvRejectsInvalidValues(t *testing.T) {
	t.Setenv("NEXUSIM_RETRIEVAL_GRAPH_EXPANSION_DEPTH", "not-a-number")
	if _, err := retrievalGraphExpansionDepthFromEnv(); err == nil {
		t.Fatal("expected non-numeric graph expansion depth to be rejected")
	}

	t.Setenv("NEXUSIM_RETRIEVAL_GRAPH_EXPANSION_DEPTH", "-1")
	if _, err := retrievalGraphExpansionDepthFromEnv(); err == nil {
		t.Fatal("expected negative graph expansion depth to be rejected")
	}

	t.Setenv("NEXUSIM_RETRIEVAL_GRAPH_EXPANSION_DEPTH", "4")
	if _, err := retrievalGraphExpansionDepthFromEnv(); err == nil {
		t.Fatal("expected graph expansion depth above maximum to be rejected")
	}
}

func TestValidateRetrievalDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	for _, addr := range []string{"", "localhost:11919", "127.0.0.1:11919", "172.30.80.22:11919"} {
		if err := validateRetrievalDebugListenerConfig(addr, false); err != nil {
			t.Fatalf("addr %s should be accepted: %v", addr, err)
		}
	}
}

func TestValidateRetrievalDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateRetrievalDebugListenerConfig("8.8.8.8:11919", false); err == nil {
		t.Fatal("expected public listener rejection")
	}
}

func TestValidateRetrievalDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateRetrievalDebugListenerConfig("8.8.8.8:11919", true); err != nil {
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
	if body != "nexusim_retrieval_gateway_info 1\n" {
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
