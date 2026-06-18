package main

import (
	"io"
	"net/http"
	"testing"
)

func TestValidateRAGServiceMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc"} {
		if err := validateRAGServiceMode(mode); err != nil {
			t.Fatalf("mode %s should be valid: %v", mode, err)
		}
	}
	if err := validateRAGServiceMode("bad"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestValidateRAGDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	for _, addr := range []string{"", "localhost:11920", "127.0.0.1:11920", "172.30.80.23:11920"} {
		if err := validateRAGDebugListenerConfig(addr, false); err != nil {
			t.Fatalf("addr %s should be accepted: %v", addr, err)
		}
	}
}

func TestValidateRAGDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateRAGDebugListenerConfig("8.8.8.8:11920", false); err == nil {
		t.Fatal("expected public listener rejection")
	}
}

func TestValidateRAGDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateRAGDebugListenerConfig("8.8.8.8:11920", true); err != nil {
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
	if body != "nexusim_rag_service_info 1\n" {
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
