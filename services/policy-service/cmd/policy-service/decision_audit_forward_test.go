package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
)

func TestValidateDecisionAuditForwardEndpointRequiresHTTPSByDefault(t *testing.T) {
	if _, err := validateDecisionAuditForwardEndpoint("http://audit.example.test/ingest", false); err == nil {
		t.Fatalf("expected insecure endpoint to be rejected by default")
	}
	if _, err := validateDecisionAuditForwardEndpoint("http://audit.example.test/ingest", true); err != nil {
		t.Fatalf("expected insecure endpoint override to pass: %v", err)
	}
	if _, err := validateDecisionAuditForwardEndpoint("https://audit.example.test/ingest", false); err != nil {
		t.Fatalf("expected https endpoint to pass: %v", err)
	}
}

func TestPolicyDecisionAuditForwardConfigFromEnv(t *testing.T) {
	clearDecisionAuditForwardEnv(t)
	t.Setenv("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ENDPOINT", "https://audit.example.test/ingest")
	t.Setenv("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_BEARER_TOKEN", "secret-token")
	t.Setenv("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_TIMEOUT", "3s")
	t.Setenv("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_LIMIT", "17")

	config, err := policyDecisionAuditForwardConfigFromEnv()
	if err != nil {
		t.Fatalf("load forward config: %v", err)
	}
	if config.Endpoint != "https://audit.example.test/ingest" ||
		config.BearerToken != "secret-token" ||
		config.Timeout != 3*time.Second ||
		config.Limit != 17 ||
		config.DryRun {
		t.Fatalf("unexpected forward config: %+v", config)
	}
}

func TestForwardDecisionAuditPostsLowSensitivePayload(t *testing.T) {
	var receivedAuth string
	var received decisionAuditForwardPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		receivedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	summary, err := forwardDecisionAudit(context.Background(), decisionAuditForwardConfig{
		Endpoint:    server.URL,
		BearerToken: "sink-secret",
		Timeout:     time.Second,
		Client:      server.Client(),
	}, []postgresinfra.DecisionAuditExportRow{sampleDecisionAuditRow()}, map[string]string{
		"tenant_id": "tenant-a",
		"event_id":  "",
	})
	if err != nil {
		t.Fatalf("forward decision audit: %v", err)
	}
	if !summary.Success ||
		summary.StatusCode != http.StatusAccepted ||
		summary.StatusFamily != "2xx" ||
		summary.RowCount != 1 ||
		summary.EndpointHost == "" {
		t.Fatalf("unexpected forward summary: %+v", summary)
	}
	if receivedAuth != "Bearer sink-secret" {
		t.Fatalf("expected bearer auth header, got %q", receivedAuth)
	}
	if received.Schema != policyDecisionAuditForwardSchema ||
		received.RowCount != 1 ||
		len(received.Rows) != 1 ||
		received.Rows[0].EventID != "event-1" ||
		received.Rows[0].ActorUserKey != "actor-key" ||
		received.Rows[0].DecisionSource != "CONTENT_MODERATION" {
		t.Fatalf("unexpected forward payload: %+v", received)
	}
	raw, _ := json.Marshal(received)
	for _, forbidden := range []string{"user@example.com", "provider body", "message text", "sink-secret"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("forward payload leaked forbidden text %q: %s", forbidden, raw)
		}
	}
}

func TestForwardDecisionAuditWritesFailureSummaryWithoutProviderBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider body with secret-token", http.StatusBadGateway)
	}))
	defer server.Close()

	summary, err := forwardDecisionAudit(context.Background(), decisionAuditForwardConfig{
		Endpoint:    server.URL,
		BearerToken: "sink-secret",
		Timeout:     time.Second,
		Client:      server.Client(),
	}, []postgresinfra.DecisionAuditExportRow{sampleDecisionAuditRow()}, map[string]string{
		"tenant_id": "tenant-a",
	})
	if err == nil {
		t.Fatalf("expected sink failure")
	}
	if summary.Success ||
		summary.ErrorClass != "SINK_REJECTED" ||
		summary.StatusCode != http.StatusBadGateway ||
		summary.StatusFamily != "5xx" {
		t.Fatalf("unexpected failure summary: %+v", summary)
	}
	outputPath := filepath.Join(t.TempDir(), "policy-decision-audit-forward.json")
	if err := writeDecisionAuditForwardSummary(outputPath, summary); err != nil {
		t.Fatalf("write forward summary: %v", err)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read forward summary: %v", err)
	}
	for _, forbidden := range []string{"provider body", "secret-token", "sink-secret", "message text"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("forward summary leaked forbidden text %q: %s", forbidden, raw)
		}
	}
}

func TestForwardDecisionAuditDryRunDoesNotRequireEndpoint(t *testing.T) {
	summary, err := forwardDecisionAudit(context.Background(), decisionAuditForwardConfig{
		DryRun: true,
	}, []postgresinfra.DecisionAuditExportRow{sampleDecisionAuditRow()}, nil)
	if err != nil {
		t.Fatalf("dry-run forward: %v", err)
	}
	if !summary.Success || !summary.DryRun || summary.StatusFamily != "DRY_RUN" || summary.RowCount != 1 {
		t.Fatalf("unexpected dry-run summary: %+v", summary)
	}
}

func sampleDecisionAuditRow() postgresinfra.DecisionAuditExportRow {
	publishedAt := time.Date(2026, 6, 18, 10, 5, 0, 0, time.UTC)
	return postgresinfra.DecisionAuditExportRow{
		EventID:                  "event-1",
		TenantID:                 "tenant-a",
		ActorUserKey:             "actor-key",
		DeviceKey:                "device-key",
		ConversationKey:          "conversation-key",
		MessageKey:               "message-key",
		Action:                   "SEND",
		MessageIDPresent:         true,
		DirectPeerContextPresent: true,
		DirectPeerKey:            "peer-key",
		Allowed:                  false,
		PermissionVersion:        43,
		Classification:           "CONTENT_PROVIDER_DENIED",
		ReasonCode:               "CONTENT_PROVIDER_DENIED",
		DecisionSource:           "CONTENT_MODERATION",
		Status:                   "PUBLISHED",
		EventType:                "policy.message_action_decision.v1",
		EventVersion:             "v1",
		Producer:                 "policy-service",
		PartitionKey:             "tenant-a:conversation-key",
		CorrelationID:            "request-1",
		TraceID:                  "trace-1",
		CreatedAt:                time.Date(2026, 6, 18, 10, 4, 0, 0, time.UTC),
		PublishedAt:              &publishedAt,
	}
}

func clearDecisionAuditForwardEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ENDPOINT",
		"NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_BEARER_TOKEN",
		"NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_TIMEOUT",
		"NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ALLOW_INSECURE",
		"NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_DRY_RUN",
		"NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_OUTPUT",
		"NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_LIMIT",
	} {
		t.Setenv(name, "")
	}
}
