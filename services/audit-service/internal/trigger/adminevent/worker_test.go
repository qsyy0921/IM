package adminevent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	admineventsv1 "github.com/qsyy0921/IM/schemas/kafka/admin/v1"
	"github.com/qsyy0921/IM/services/audit-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestBuildAppendCommandFromSubmittedEvent(t *testing.T) {
	command, err := BuildAppendCommand(adminMessage(submittedEvent()))
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if command.AuthContext.TenantID != "tenant-admin" ||
		command.AuditStream != auditStreamAdmin ||
		command.SourceService != sourceService ||
		command.SourceEventID != "admin-event-1" ||
		command.RecordType != "ADMIN_OPERATION" ||
		command.Action != "SUBMIT_ADMIN_OPERATION" ||
		command.Outcome != "SUBMITTED" ||
		command.IdempotencyKey != "admin-event:admin-event-1" {
		t.Fatalf("unexpected command: %+v", command)
	}
	var attributes map[string]any
	if err := json.Unmarshal([]byte(command.AttributesJSON), &attributes); err != nil {
		t.Fatalf("decode attributes: %v", err)
	}
	if attributes["operation_type"] != "AUDIT_EXPORT_REQUEST" ||
		attributes["payload_hash"] != "sha256:payload" ||
		attributes["target_ref_hash"] != "sha256:target" {
		t.Fatalf("unexpected attributes: %+v", attributes)
	}
}

func TestBuildAppendCommandFromFailedEventDoesNotPersistPublicError(t *testing.T) {
	event := submittedEvent()
	event.EventType = "admin.operation.failed.v1"
	event.Payload = &admineventsv1.AdminEvent_OperationFailed{OperationFailed: &admineventsv1.AdminOperationFailedV1{
		TenantId:             "tenant-admin",
		OperationId:          "admop-1",
		OperationType:        "CONFIG_PUBLISH",
		TargetRefHash:        "sha256:target",
		RiskLevel:            "HIGH",
		Status:               "FAILED",
		RequestedByHash:      "sha256:requester",
		ApprovedByHash:       "sha256:approver",
		ResultId:             "admres-1",
		DownstreamService:    "control-plane-service",
		DownstreamRequestRef: "config-version:cfgv-1",
		FailureClass:         "DOWNSTREAM_UNAVAILABLE",
		PublicError:          "stable public error",
		PayloadHash:          "sha256:payload",
	}}
	command, err := BuildAppendCommand(adminMessage(event))
	if err != nil {
		t.Fatalf("build failed command: %v", err)
	}
	if command.Action != "FAIL_ADMIN_OPERATION" || command.Outcome != "FAILED" {
		t.Fatalf("unexpected failed command: %+v", command)
	}
	if strings.Contains(command.AttributesJSON, "stable public error") {
		t.Fatalf("attributes should not persist public error text: %s", command.AttributesJSON)
	}
	if !strings.Contains(command.AttributesJSON, "DOWNSTREAM_UNAVAILABLE") {
		t.Fatalf("attributes should keep low-sensitive failure class: %s", command.AttributesJSON)
	}
}

func TestBuildAppendCommandRejectsMalformedAndUnsupported(t *testing.T) {
	message := adminMessage(submittedEvent())
	message.Value = []byte("not-protobuf")
	if _, err := BuildAppendCommand(message); err == nil {
		t.Fatalf("malformed protobuf should fail")
	}
	event := submittedEvent()
	event.Payload = nil
	if _, err := BuildAppendCommand(adminMessage(event)); err == nil {
		t.Fatalf("unsupported payload should fail")
	}
	event = submittedEvent()
	event.Producer = "other-service"
	if _, err := BuildAppendCommand(adminMessage(event)); err == nil {
		t.Fatalf("unexpected producer should fail")
	}
	event = submittedEvent()
	event.AggregateType = "other_aggregate"
	if _, err := BuildAppendCommand(adminMessage(event)); err == nil {
		t.Fatalf("unexpected aggregate type should fail")
	}
	event = submittedEvent()
	event.GetOperationSubmitted().TenantId = "other-tenant"
	if _, err := BuildAppendCommand(adminMessage(event)); err == nil {
		t.Fatalf("payload tenant mismatch should fail")
	}
}

func TestWorkerCommitsOnlyAfterProjectionSuccess(t *testing.T) {
	consumer := &fakeConsumer{message: adminMessage(submittedEvent())}
	projector := &fakeProjector{}
	worker := NewWorker(consumer, projector)
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !consumer.committed || projector.command.SourceEventID != "admin-event-1" {
		t.Fatalf("worker did not project then commit: committed=%v command=%+v", consumer.committed, projector.command)
	}

	consumer = &fakeConsumer{message: adminMessage(submittedEvent())}
	projector = &fakeProjector{err: types.NewDBWriteFailed("append failed")}
	worker = NewWorker(consumer, projector)
	if err := worker.RunOnce(context.Background()); err == nil {
		t.Fatalf("projection failure should fail")
	}
	if consumer.committed {
		t.Fatalf("worker committed failed projection")
	}
}

type fakeConsumer struct {
	message   types.AdminEventMessage
	committed bool
}

func (consumer *fakeConsumer) Fetch(context.Context) (types.AdminEventMessage, error) {
	return consumer.message, nil
}

func (consumer *fakeConsumer) Commit(_ context.Context, _ types.AdminEventMessage) error {
	consumer.committed = true
	return nil
}

type fakeProjector struct {
	command types.AppendAuditRecordCommand
	err     error
}

func (projector *fakeProjector) Execute(_ context.Context, command types.AppendAuditRecordCommand) (types.AuditRecord, error) {
	projector.command = command
	if projector.err != nil {
		return types.AuditRecord{}, projector.err
	}
	return types.AuditRecord{AuditID: "aud-1", SourceEventID: command.SourceEventID}, nil
}

func adminMessage(event *admineventsv1.AdminEvent) types.AdminEventMessage {
	value, _ := proto.Marshal(event)
	return types.AdminEventMessage{
		Topic:     TopicAdminEvents,
		Partition: 0,
		Offset:    12,
		Key:       []byte(event.GetPartitionKey()),
		Value:     value,
	}
}

func submittedEvent() *admineventsv1.AdminEvent {
	return &admineventsv1.AdminEvent{
		EventId:          "admin-event-1",
		EventType:        "admin.operation.submitted.v1",
		EventVersion:     1,
		TenantId:         "tenant-admin",
		AggregateType:    "admin_operation",
		AggregateId:      "admop-1",
		AggregateVersion: 1,
		PartitionKey:     "tenant-admin:admop-1",
		TraceId:          "trace-1",
		CorrelationId:    "corr-1",
		CausationId:      "cause-1",
		Producer:         sourceService,
		OccurredAt:       timestamppb.New(time.Date(2026, 6, 21, 8, 0, 0, 0, time.UTC)),
		Payload: &admineventsv1.AdminEvent_OperationSubmitted{OperationSubmitted: &admineventsv1.AdminOperationSubmittedV1{
			TenantId:             "tenant-admin",
			OperationId:          "admop-1",
			OperationType:        "AUDIT_EXPORT_REQUEST",
			TargetRefHash:        "sha256:target",
			RiskLevel:            "HIGH",
			Status:               "SUBMITTED",
			RequestedByHash:      "sha256:requester",
			PayloadSchemaVersion: "admin.audit_export_request.v1",
			PayloadHash:          "sha256:payload",
			ReasonRef:            "reason:ticket-1",
		}},
	}
}
