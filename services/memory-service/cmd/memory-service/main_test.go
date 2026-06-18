package main

import (
	"io"
	"net/http"
	"testing"
)

func TestValidateMemoryServiceMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc", "timeline-consumer"} {
		if err := validateMemoryServiceMode(mode); err != nil {
			t.Fatalf("mode %s should be valid: %v", mode, err)
		}
	}
	if err := validateMemoryServiceMode("bad"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestValidateMemoryDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	for _, addr := range []string{"", "localhost:11918", "127.0.0.1:11918", "172.30.80.21:11918"} {
		if err := validateMemoryDebugListenerConfig(addr, false); err != nil {
			t.Fatalf("addr %s should be accepted: %v", addr, err)
		}
	}
}

func TestValidateMemoryDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateMemoryDebugListenerConfig("8.8.8.8:11918", false); err == nil {
		t.Fatal("expected public listener rejection")
	}
}

func TestValidateMemoryDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateMemoryDebugListenerConfig("8.8.8.8:11918", true); err != nil {
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
	if body != "nexusim_memory_service_info 1\n" {
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
