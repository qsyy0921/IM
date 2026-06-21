package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	admineventsv1 "github.com/qsyy0921/IM/schemas/kafka/admin/v1"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildAdminEventSubmitted(t *testing.T) {
	value, err := BuildKafkaValue(adminOutboxMessage(types.AdminEventOperationSubmitted, submittedPayload()))
	if err != nil {
		t.Fatalf("build kafka value: %v", err)
	}
	var event admineventsv1.AdminEvent
	if err := proto.Unmarshal(value, &event); err != nil {
		t.Fatalf("decode admin event: %v", err)
	}
	submitted := event.GetOperationSubmitted()
	if submitted == nil {
		t.Fatalf("expected submitted payload: %+v", &event)
	}
	if event.EventId != "admop-1:submitted" ||
		event.EventType != types.AdminEventOperationSubmitted ||
		event.PartitionKey != "tenant-admin:admop-1" ||
		event.Producer != "admin-service" ||
		submitted.OperationId != "admop-1" ||
		submitted.PayloadHash != "sha256:payload" {
		t.Fatalf("unexpected event: %+v payload=%+v", &event, submitted)
	}
}

func TestBuildAdminEventApproved(t *testing.T) {
	event, err := BuildAdminEvent(adminOutboxMessage(types.AdminEventOperationApproved, approvedPayload(types.DecisionApprove)))
	if err != nil {
		t.Fatalf("build approved event: %v", err)
	}
	approved := event.GetOperationApproved()
	if approved == nil || approved.ApprovalId != "admappr-1" || approved.Decision != types.DecisionApprove {
		t.Fatalf("unexpected approved event: %+v", event)
	}
}

func TestBuildAdminEventExecutedAndFailed(t *testing.T) {
	executedEvent, err := BuildAdminEvent(adminOutboxMessage(types.AdminEventOperationExecuted, executedPayload()))
	if err != nil {
		t.Fatalf("build executed event: %v", err)
	}
	executed := executedEvent.GetOperationExecuted()
	if executed == nil ||
		executed.ResultId != "admres-1" ||
		executed.DownstreamService != "local-noop" ||
		executed.DownstreamRequestRef != "operation:admop-1" {
		t.Fatalf("unexpected executed event: %+v", executedEvent)
	}

	failedPayload := executedPayload()
	failedPayload["status"] = types.OperationStatusFailed
	failedPayload["failure_class"] = "EXECUTOR_UNAVAILABLE"
	failedPayload["public_error"] = "admin operation execution failed"
	failedEvent, err := BuildAdminEvent(adminOutboxMessage(types.AdminEventOperationFailed, failedPayload))
	if err != nil {
		t.Fatalf("build failed event: %v", err)
	}
	failed := failedEvent.GetOperationFailed()
	if failed == nil ||
		failed.ResultId != "admres-1" ||
		failed.FailureClass != "EXECUTOR_UNAVAILABLE" ||
		failed.PublicError != "admin operation execution failed" {
		t.Fatalf("unexpected failed event: %+v", failedEvent)
	}
}

func TestBuildAdminEventCompensationRequested(t *testing.T) {
	payload := approvedPayload(types.DecisionApprove)
	payload["status"] = types.OperationStatusCompensationRequested
	payload["compensation_requested_by_hash"] = "sha256:compensator"
	payload["compensation_reason_ref"] = "reason-sha256:compensation"
	event, err := BuildAdminEvent(adminOutboxMessage(types.AdminEventOperationCompensationRequested, payload))
	if err != nil {
		t.Fatalf("build compensation requested event: %v", err)
	}
	compensation := event.GetOperationCompensationRequested()
	if compensation == nil ||
		compensation.Status != types.OperationStatusCompensationRequested ||
		compensation.CompensationRequestedByHash != "sha256:compensator" ||
		compensation.CompensationReasonRef != "reason-sha256:compensation" {
		t.Fatalf("unexpected compensation event: %+v", event)
	}
}

func TestBuildAdminEventRejectsSensitivePayloadFields(t *testing.T) {
	for _, field := range []string{"payload_json", "operation_payload_json", "operator_ref", "token", "secret", "provider_body"} {
		payload := submittedPayload()
		payload[field] = "must-not-leak"
		if _, err := BuildAdminEvent(adminOutboxMessage(types.AdminEventOperationSubmitted, payload)); err == nil {
			t.Fatalf("expected sensitive field %s to be rejected", field)
		}
	}
}

func TestBuildAdminEventRejectsUnsupportedAndMalformed(t *testing.T) {
	if _, err := BuildAdminEvent(adminOutboxMessage("admin.future.v9", submittedPayload())); err == nil {
		t.Fatalf("expected unsupported event type to fail")
	}
	message := adminOutboxMessage(types.AdminEventOperationSubmitted, submittedPayload())
	message.PayloadJSON = []byte(`{"tenant_id":`)
	if _, err := BuildAdminEvent(message); err == nil {
		t.Fatalf("expected malformed payload to fail")
	}
}

func TestRelayRunOncePublishesOnlyBuildableMessages(t *testing.T) {
	store := &fakeStore{
		messages: []types.OutboxMessage{
			adminOutboxMessage(types.AdminEventOperationSubmitted, submittedPayload()),
			adminOutboxMessage("admin.future.v9", map[string]any{"tenant_id": "tenant-admin"}),
		},
	}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{})
	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 2 || stats.Published != 1 || stats.Retried != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(publisher.records) != 1 {
		t.Fatalf("expected one published record, got %d", len(publisher.records))
	}
}

type fakeStore struct {
	messages []types.OutboxMessage
}

func (store *fakeStore) ProcessReadyBatch(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	publish func(context.Context, []types.OutboxMessage) []error,
) (types.OutboxRelayStats, error) {
	errs := publish(ctx, store.messages)
	stats := types.OutboxRelayStats{Fetched: len(store.messages)}
	for _, err := range errs {
		if err == nil {
			stats.Published++
			continue
		}
		stats.Retried++
	}
	if len(errs) != len(store.messages) {
		return types.OutboxRelayStats{}, errors.New("mismatched publish result")
	}
	return stats, nil
}

type fakePublisher struct {
	records []types.KafkaPublishRecord
}

func (publisher *fakePublisher) PublishBatch(_ context.Context, _ string, records []types.KafkaPublishRecord) error {
	publisher.records = append(publisher.records, records...)
	return nil
}

func adminOutboxMessage(eventType string, payload map[string]any) types.OutboxMessage {
	payloadJSON, _ := json.Marshal(payload)
	return types.OutboxMessage{
		EventID:          "admop-1:submitted",
		TenantID:         "tenant-admin",
		OperationID:      "admop-1",
		EventType:        eventType,
		EventVersion:     1,
		PartitionKey:     "tenant-admin:admop-1",
		Producer:         "admin-service",
		PayloadJSON:      payloadJSON,
		OccurredAt:       time.Date(2026, 6, 21, 8, 0, 0, 0, time.UTC),
		AggregateVersion: 1,
	}
}

func submittedPayload() map[string]any {
	return map[string]any{
		"tenant_id":              "tenant-admin",
		"operation_id":           "admop-1",
		"operation_type":         "USER_BAN",
		"target_ref_hash":        "sha256:target",
		"risk_level":             types.RiskLevelHigh,
		"status":                 types.OperationStatusSubmitted,
		"requested_by_hash":      "sha256:requester",
		"payload_schema_version": "admin.user_ban.v1",
		"payload_hash":           "sha256:payload",
		"reason_ref":             "reason:ticket-1",
		"correlation_id":         "corr-1",
		"causation_id":           "cause-1",
		"trace_id":               "trace-1",
	}
}

func approvedPayload(decision string) map[string]any {
	payload := submittedPayload()
	payload["status"] = types.OperationStatusApproved
	payload["approved_by_hash"] = "sha256:approver"
	payload["approval_id"] = "admappr-1"
	payload["decision"] = decision
	return payload
}

func executedPayload() map[string]any {
	payload := approvedPayload(types.DecisionApprove)
	payload["status"] = types.OperationStatusSucceeded
	payload["result_id"] = "admres-1"
	payload["downstream_service"] = "local-noop"
	payload["downstream_request_ref"] = "operation:admop-1"
	return payload
}
