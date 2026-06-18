package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestDecisionAuditOutboxRecordsPolicyDecisionIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	decidedAt := time.Date(2026, 6, 13, 10, 20, 30, 0, time.UTC)
	outbox := NewDecisionAuditOutbox(
		pool,
		WithDecisionAuditEventID(func() (string, error) {
			return "policy-audit-event-1", nil
		}),
		WithDecisionAuditClock(func() time.Time {
			return decidedAt
		}),
	)
	command := testPolicyCommand(types.MessageActionSend)
	command.DirectPeerUserID = "peer-policy"
	command.AuthContext.TraceID = "trace-policy-audit"
	command.AuthContext.RequestID = "request-policy-audit"
	command.AuthContext.SessionID = "session-should-not-be-persisted"
	decision := types.MessageActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ConversationID:    command.ConversationID,
		Action:            command.Action,
		Allowed:           false,
		PermissionVersion: 12,
		Classification:    "CONTACT_BLOCKED",
		Reason:            "contact blocked",
		DecisionSource:    types.PolicyDecisionSourceContactProjection,
	}

	if err := outbox.RecordPolicyDecision(ctx, command, decision); err != nil {
		t.Fatalf("record policy decision: %v", err)
	}

	var row struct {
		EventID           string
		TenantID          string
		ActorUserKey      string
		DeviceKey         string
		ConversationKey   string
		MessageKey        string
		Action            string
		DirectPeerPresent bool
		DirectPeerKey     string
		Allowed           bool
		PermissionVersion int64
		Classification    string
		ReasonCode        string
		DecisionSource    string
		EventType         string
		Producer          string
		PartitionKey      string
		CorrelationID     string
		TraceID           string
		PayloadJSON       string
		Status            string
	}
	err := pool.QueryRow(ctx, `
SELECT
    event_id,
    tenant_id,
    actor_user_key,
    device_key,
    conversation_key,
    message_key,
    action,
    direct_peer_context_present,
    direct_peer_key,
    allowed,
    permission_version,
    classification,
    reason_code,
    decision_source,
    event_type,
    producer,
    partition_key,
    correlation_id,
    trace_id,
    payload_json::text,
    status
FROM policy_decision_audit_outbox
WHERE tenant_id = $1
`, command.AuthContext.TenantID).Scan(
		&row.EventID,
		&row.TenantID,
		&row.ActorUserKey,
		&row.DeviceKey,
		&row.ConversationKey,
		&row.MessageKey,
		&row.Action,
		&row.DirectPeerPresent,
		&row.DirectPeerKey,
		&row.Allowed,
		&row.PermissionVersion,
		&row.Classification,
		&row.ReasonCode,
		&row.DecisionSource,
		&row.EventType,
		&row.Producer,
		&row.PartitionKey,
		&row.CorrelationID,
		&row.TraceID,
		&row.PayloadJSON,
		&row.Status,
	)
	if err != nil {
		t.Fatalf("read audit outbox: %v", err)
	}
	expectedActorKey := policyAuditStableKey(string(command.AuthContext.TenantID), "user", string(command.AuthContext.UserID))
	expectedDeviceKey := policyAuditStableKey(string(command.AuthContext.TenantID), "device", string(command.AuthContext.DeviceID))
	expectedConversationKey := policyAuditStableKey(string(command.AuthContext.TenantID), "conversation", string(command.ConversationID))
	expectedDirectPeerKey := policyAuditStableKey(string(command.AuthContext.TenantID), "user", string(command.DirectPeerUserID))
	if row.EventID != "policy-audit-event-1" ||
		row.TenantID != string(command.AuthContext.TenantID) ||
		row.ActorUserKey != expectedActorKey ||
		row.DeviceKey != expectedDeviceKey ||
		row.ConversationKey != expectedConversationKey ||
		row.MessageKey != "" ||
		row.Action != string(types.MessageActionSend) ||
		!row.DirectPeerPresent ||
		row.DirectPeerKey != expectedDirectPeerKey ||
		row.Allowed ||
		row.PermissionVersion != 12 ||
		row.Classification != "CONTACT_BLOCKED" ||
		row.ReasonCode != "CONTACT_BLOCKED" ||
		row.DecisionSource != string(types.PolicyDecisionSourceContactProjection) ||
		row.EventType != policyDecisionAuditEventType ||
		row.Producer != "policy-service" ||
		row.PartitionKey != "tenant-policy:"+expectedConversationKey ||
		row.CorrelationID != "request-policy-audit" ||
		row.TraceID != "trace-policy-audit" ||
		row.Status != "PENDING" {
		t.Fatalf("unexpected audit row: %+v", row)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
		t.Fatalf("decode audit payload: %v", err)
	}
	if payload["event_id"] != "policy-audit-event-1" ||
		payload["actor_user_key"] != expectedActorKey ||
		payload["device_key"] != expectedDeviceKey ||
		payload["conversation_key"] != expectedConversationKey ||
		payload["direct_peer_key"] != expectedDirectPeerKey ||
		payload["direct_peer_context_present"] != true ||
		payload["classification"] != "CONTACT_BLOCKED" ||
		payload["reason_code"] != "CONTACT_BLOCKED" ||
		payload["decision_source"] != string(types.PolicyDecisionSourceContactProjection) ||
		payload["decided_at"] != decidedAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected audit payload: %+v", payload)
	}
	for _, raw := range []string{"user-policy", "device-policy", "peer-policy", "conv-policy", "session-should-not-be-persisted", "contact blocked"} {
		if strings.Contains(row.PayloadJSON, raw) {
			t.Fatalf("audit payload must not include raw value %q: %s", raw, row.PayloadJSON)
		}
	}
}

func TestDecisionAuditOutboxRejectsInvalidDecision(t *testing.T) {
	outbox := NewDecisionAuditOutbox(nil)
	err := outbox.RecordPolicyDecision(context.Background(), testPolicyCommand(types.MessageActionSend), types.MessageActionDecision{})
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
}
