package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

const policyDecisionAuditEventType = "policy.message_action_decision.v1"

type DecisionAuditOutbox struct {
	pool          *pgxpool.Pool
	eventID       func() (string, error)
	now           func() time.Time
	stageObserver DecisionAuditStageObserver
}

type DecisionAuditOutboxOption func(*DecisionAuditOutbox)

type DecisionAuditStageObserver interface {
	RecordPolicyDecisionStage(action types.MessageAction, stage string, failed bool, latencyMS int64)
}

func NewDecisionAuditOutbox(pool *pgxpool.Pool, opts ...DecisionAuditOutboxOption) *DecisionAuditOutbox {
	outbox := &DecisionAuditOutbox{
		pool:    pool,
		eventID: newPolicyAuditEventID,
		now:     func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(outbox)
	}
	return outbox
}

func WithDecisionAuditEventID(fn func() (string, error)) DecisionAuditOutboxOption {
	return func(outbox *DecisionAuditOutbox) {
		if fn != nil {
			outbox.eventID = fn
		}
	}
}

func WithDecisionAuditStageObserver(observer DecisionAuditStageObserver) DecisionAuditOutboxOption {
	return func(outbox *DecisionAuditOutbox) {
		outbox.stageObserver = observer
	}
}

func WithDecisionAuditClock(clock func() time.Time) DecisionAuditOutboxOption {
	return func(outbox *DecisionAuditOutbox) {
		if clock != nil {
			outbox.now = clock
		}
	}
}

func (outbox *DecisionAuditOutbox) RecordPolicyDecision(
	ctx context.Context,
	command types.CheckMessageActionCommand,
	decision types.MessageActionDecision,
) error {
	if outbox == nil || outbox.pool == nil {
		return types.NewDependencyUnavailable("policy decision audit outbox is not configured")
	}
	if decision.PermissionVersion <= 0 || strings.TrimSpace(decision.Classification) == "" {
		return types.NewDependencyUnavailable("policy decision audit payload is invalid")
	}
	eventID, err := outbox.eventID()
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	decidedAt := outbox.now()
	classification := truncateAuditField(strings.TrimSpace(decision.Classification), 128)
	reasonCode := policyDecisionReasonCode(decision)
	decisionSource := policyDecisionSource(decision)
	traceID := truncateAuditField(strings.TrimSpace(command.AuthContext.TraceID), 128)
	requestID := truncateAuditField(strings.TrimSpace(command.AuthContext.RequestID), 128)
	messageIDPresent := decision.MessageID != ""
	directPeerContextPresent := command.DirectPeerUserID != ""
	actorUserKey := policyAuditStableKey(string(decision.TenantID), "user", string(decision.UserID))
	deviceKey := policyAuditStableKey(string(decision.TenantID), "device", string(command.AuthContext.DeviceID))
	conversationKey := policyAuditStableKey(string(decision.TenantID), "conversation", string(decision.ConversationID))
	messageKey := ""
	if messageIDPresent {
		messageKey = policyAuditStableKey(string(decision.TenantID), "message", string(decision.MessageID))
	}
	directPeerKey := ""
	if directPeerContextPresent {
		directPeerKey = policyAuditStableKey(string(decision.TenantID), "user", string(command.DirectPeerUserID))
	}
	payload := policyDecisionAuditPayload{
		EventID:                  eventID,
		TenantID:                 string(decision.TenantID),
		ActorUserKey:             actorUserKey,
		DeviceKey:                deviceKey,
		ConversationKey:          conversationKey,
		MessageKey:               messageKey,
		Action:                   string(decision.Action),
		MessageIDPresent:         messageIDPresent,
		DirectPeerContextPresent: directPeerContextPresent,
		DirectPeerKey:            directPeerKey,
		Allowed:                  decision.Allowed,
		PermissionVersion:        decision.PermissionVersion,
		Classification:           classification,
		ReasonCode:               reasonCode,
		DecisionSource:           decisionSource,
		TraceID:                  traceID,
		RequestID:                requestID,
		DecidedAt:                decidedAt.Format(time.RFC3339Nano),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	acquireStarted := time.Now()
	conn, err := outbox.pool.Acquire(ctx)
	outbox.recordStage(command.Action, "decision_audit_pool_acquire", err != nil, acquireStarted)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	defer conn.Release()

	insertStarted := time.Now()
	_, err = conn.Exec(ctx, `
INSERT INTO policy_decision_audit_outbox (
    event_id,
    tenant_id,
    aggregate_type,
    aggregate_id,
    mapping_version,
    actor_user_key,
    device_key,
    conversation_key,
    message_key,
    action,
    message_id_present,
    direct_peer_context_present,
    direct_peer_key,
    allowed,
    permission_version,
    classification,
    reason_code,
    decision_source,
    partition_key,
    correlation_id,
    causation_id,
    trace_id,
    payload_json,
    created_at,
    available_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23::jsonb, $24, $24, $24)
`, eventID,
		decision.TenantID,
		"policy_decision",
		policyDecisionPartitionKey(decision.TenantID, conversationKey),
		int64(1),
		actorUserKey,
		deviceKey,
		conversationKey,
		messageKey,
		decision.Action,
		messageIDPresent,
		directPeerContextPresent,
		directPeerKey,
		decision.Allowed,
		decision.PermissionVersion,
		classification,
		reasonCode,
		decisionSource,
		policyDecisionPartitionKey(decision.TenantID, conversationKey),
		requestID,
		requestID,
		traceID,
		string(payloadJSON),
		decidedAt,
	)
	outbox.recordStage(command.Action, "decision_audit_insert_exec", err != nil, insertStarted)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (outbox *DecisionAuditOutbox) recordStage(action types.MessageAction, stage string, failed bool, started time.Time) {
	if outbox == nil || outbox.stageObserver == nil {
		return
	}
	outbox.stageObserver.RecordPolicyDecisionStage(action, stage, failed, time.Since(started).Milliseconds())
}

type policyDecisionAuditPayload struct {
	EventID                  string `json:"event_id"`
	TenantID                 string `json:"tenant_id"`
	ActorUserKey             string `json:"actor_user_key"`
	DeviceKey                string `json:"device_key,omitempty"`
	ConversationKey          string `json:"conversation_key"`
	MessageKey               string `json:"message_key,omitempty"`
	Action                   string `json:"action"`
	MessageIDPresent         bool   `json:"message_id_present"`
	DirectPeerContextPresent bool   `json:"direct_peer_context_present"`
	DirectPeerKey            string `json:"direct_peer_key,omitempty"`
	Allowed                  bool   `json:"allowed"`
	PermissionVersion        int64  `json:"permission_version"`
	Classification           string `json:"classification"`
	ReasonCode               string `json:"reason_code,omitempty"`
	DecisionSource           string `json:"decision_source"`
	TraceID                  string `json:"trace_id,omitempty"`
	RequestID                string `json:"request_id,omitempty"`
	DecidedAt                string `json:"decided_at"`
}

func policyDecisionPartitionKey(tenantID types.TenantID, conversationKey string) string {
	return fmt.Sprintf("%s:%s", tenantID, conversationKey)
}

func truncateAuditField(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func newPolicyAuditEventID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16],
	), nil
}

func policyDecisionReasonCode(decision types.MessageActionDecision) string {
	if decision.Allowed {
		return ""
	}
	classification := strings.TrimSpace(decision.Classification)
	if classification != "" {
		return truncateAuditField(classification, 128)
	}
	return "POLICY_DENIED"
}

func policyDecisionSource(decision types.MessageActionDecision) string {
	source := strings.TrimSpace(string(decision.DecisionSource))
	if source == "" {
		return "UNSPECIFIED"
	}
	return truncateAuditField(source, 128)
}

func policyAuditStableKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}
