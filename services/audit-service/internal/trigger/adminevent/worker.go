package adminevent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	admineventsv1 "github.com/qsyy0921/IM/schemas/kafka/admin/v1"
	"github.com/qsyy0921/IM/services/audit-service/internal/types"
	"google.golang.org/protobuf/proto"
)

const (
	TopicAdminEvents            = "im.admin.events"
	auditStreamAdmin            = "admin"
	sourceService               = "admin-service"
	aggregateTypeAdminOperation = "admin_operation"
)

type Consumer interface {
	Fetch(context.Context) (types.AdminEventMessage, error)
	Commit(context.Context, types.AdminEventMessage) error
}

type Projector interface {
	Execute(context.Context, types.AppendAuditRecordCommand) (types.AuditRecord, error)
}

type Worker struct {
	consumer  Consumer
	projector Projector
	config    Config
	metrics   workerMetrics
}

type Config struct {
	ErrorBackoff time.Duration
	Logf         func(format string, args ...any)
}

type workerMetrics struct {
	totalErrors        atomic.Uint64
	consecutiveErrors  atomic.Uint64
	lastErrorAtMS      atomic.Int64
	lastSuccessAtMS    atomic.Int64
	lastCommitAtMS     atomic.Int64
	lastErrorBackoffMS atomic.Int64
}

type Snapshot struct {
	TotalErrors        uint64
	ConsecutiveErrors  uint64
	LastErrorAtMS      int64
	LastSuccessAtMS    int64
	LastCommitAtMS     int64
	LastErrorBackoffMS int64
}

func NewWorker(consumer Consumer, projector Projector, configs ...Config) *Worker {
	var config Config
	if len(configs) > 0 {
		config = configs[0]
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	return &Worker{consumer: consumer, projector: projector, config: config}
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		err := worker.RunOnce(ctx)
		if err == nil {
			worker.recordSuccess()
			continue
		}
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		if worker.config.Logf != nil {
			worker.config.Logf("audit-service admin event worker retrying after error: %v", err)
		}
		worker.recordError()
		worker.metrics.lastErrorBackoffMS.Store(worker.config.ErrorBackoff.Milliseconds())
		if err := waitForInterval(ctx, worker.config.ErrorBackoff); err != nil {
			return err
		}
	}
}

func (worker *Worker) RunOnce(ctx context.Context) error {
	if worker == nil || worker.consumer == nil {
		return errors.New("audit admin event consumer is not configured")
	}
	if worker.projector == nil {
		return errors.New("audit admin event projector is not configured")
	}
	message, err := worker.consumer.Fetch(ctx)
	if err != nil {
		return err
	}
	command, err := BuildAppendCommand(message)
	if err != nil {
		return err
	}
	if _, err := worker.projector.Execute(ctx, command); err != nil {
		return err
	}
	if err := worker.consumer.Commit(ctx, message); err != nil {
		return err
	}
	worker.metrics.lastCommitAtMS.Store(time.Now().UnixMilli())
	return nil
}

func (worker *Worker) Snapshot() Snapshot {
	return Snapshot{
		TotalErrors:        worker.metrics.totalErrors.Load(),
		ConsecutiveErrors:  worker.metrics.consecutiveErrors.Load(),
		LastErrorAtMS:      worker.metrics.lastErrorAtMS.Load(),
		LastSuccessAtMS:    worker.metrics.lastSuccessAtMS.Load(),
		LastCommitAtMS:     worker.metrics.lastCommitAtMS.Load(),
		LastErrorBackoffMS: worker.metrics.lastErrorBackoffMS.Load(),
	}
}

func BuildAppendCommand(message types.AdminEventMessage) (types.AppendAuditRecordCommand, error) {
	var event admineventsv1.AdminEvent
	if err := proto.Unmarshal(message.Value, &event); err != nil {
		return types.AppendAuditRecordCommand{}, err
	}
	if strings.TrimSpace(event.GetEventId()) == "" ||
		strings.TrimSpace(event.GetEventType()) == "" ||
		strings.TrimSpace(event.GetTenantId()) == "" ||
		strings.TrimSpace(event.GetAggregateType()) != aggregateTypeAdminOperation ||
		strings.TrimSpace(event.GetAggregateId()) == "" ||
		strings.TrimSpace(event.GetProducer()) != sourceService ||
		event.GetOccurredAt() == nil {
		return types.AppendAuditRecordCommand{}, types.NewInvalidArgument("admin event envelope is incomplete")
	}
	base := adminPayloadBase{
		eventType:        event.GetEventType(),
		aggregateVersion: event.GetAggregateVersion(),
		partitionKey:     event.GetPartitionKey(),
	}
	switch payload := event.GetPayload().(type) {
	case *admineventsv1.AdminEvent_OperationSubmitted:
		fillSubmitted(&base, payload.OperationSubmitted)
	case *admineventsv1.AdminEvent_OperationApproved:
		fillApproved(&base, payload.OperationApproved)
	case *admineventsv1.AdminEvent_OperationRejected:
		fillRejected(&base, payload.OperationRejected)
	case *admineventsv1.AdminEvent_OperationExecuted:
		fillExecuted(&base, payload.OperationExecuted)
	case *admineventsv1.AdminEvent_OperationFailed:
		fillFailed(&base, payload.OperationFailed)
	case *admineventsv1.AdminEvent_OperationCompensationRequested:
		fillCompensationRequested(&base, payload.OperationCompensationRequested)
	default:
		return types.AppendAuditRecordCommand{}, types.NewInvalidArgument("unsupported admin event payload")
	}
	if err := base.validate(event.GetTenantId(), event.GetAggregateId()); err != nil {
		return types.AppendAuditRecordCommand{}, err
	}
	attributesJSON, err := base.attributesJSON()
	if err != nil {
		return types.AppendAuditRecordCommand{}, err
	}
	return types.AppendAuditRecordCommand{
		AuthContext: types.AuthContext{
			TenantID:  types.TenantID(event.GetTenantId()),
			UserID:    "audit-admin-event-consumer",
			DeviceID:  "kafka",
			TraceID:   event.GetTraceId(),
			RequestID: event.GetCorrelationId(),
		},
		AuditStream:    auditStreamAdmin,
		SourceService:  sourceService,
		SourceEventID:  event.GetEventId(),
		RecordType:     "ADMIN_OPERATION",
		ActorRef:       base.actorRef(),
		SubjectRef:     base.targetRefHash,
		ResourceRef:    "admin_operation:" + event.GetAggregateId(),
		Action:         base.action,
		Outcome:        base.outcome,
		ReasonCode:     base.reasonCode,
		RiskLevel:      base.riskLevel,
		OccurredAt:     event.GetOccurredAt().AsTime(),
		AttributesJSON: attributesJSON,
		IdempotencyKey: "admin-event:" + event.GetEventId(),
		CorrelationID:  event.GetCorrelationId(),
		CausationID:    event.GetCausationId(),
		TraceID:        event.GetTraceId(),
	}, nil
}

type adminPayloadBase struct {
	eventType                   string
	aggregateVersion            int64
	partitionKey                string
	payloadTenantID             string
	operationID                 string
	operationType               string
	targetRefHash               string
	riskLevel                   string
	status                      string
	requestedByHash             string
	approvedByHash              string
	payloadHash                 string
	payloadSchemaVersion        string
	reasonRef                   string
	approvalID                  string
	decision                    string
	resultID                    string
	downstreamService           string
	downstreamRequestRef        string
	failureClass                string
	compensationRequestedByHash string
	compensationReasonRef       string
	action                      string
	outcome                     string
	reasonCode                  string
}

func (payload adminPayloadBase) validate(tenantID string, aggregateID string) error {
	if payload.payloadTenantID != tenantID ||
		payload.operationID != aggregateID ||
		strings.TrimSpace(tenantID) == "" ||
		strings.TrimSpace(payload.operationType) == "" ||
		strings.TrimSpace(payload.targetRefHash) == "" ||
		strings.TrimSpace(payload.riskLevel) == "" ||
		strings.TrimSpace(payload.status) == "" ||
		strings.TrimSpace(payload.requestedByHash) == "" ||
		strings.TrimSpace(payload.payloadHash) == "" ||
		strings.TrimSpace(payload.action) == "" ||
		strings.TrimSpace(payload.outcome) == "" {
		return types.NewInvalidArgument("admin event payload is incomplete")
	}
	return nil
}

func (payload adminPayloadBase) actorRef() string {
	for _, value := range []string{
		payload.compensationRequestedByHash,
		payload.approvedByHash,
		payload.requestedByHash,
	} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (payload adminPayloadBase) attributesJSON() (string, error) {
	attributes := map[string]any{
		"event_type":        payload.eventType,
		"aggregate_version": payload.aggregateVersion,
		"operation_id":      payload.operationID,
		"operation_type":    payload.operationType,
		"target_ref_hash":   payload.targetRefHash,
		"risk_level":        payload.riskLevel,
		"status":            payload.status,
		"requested_by_hash": payload.requestedByHash,
		"payload_hash":      payload.payloadHash,
	}
	addIfPresent(attributes, "partition_key", payload.partitionKey)
	addIfPresent(attributes, "approved_by_hash", payload.approvedByHash)
	addIfPresent(attributes, "payload_schema_version", payload.payloadSchemaVersion)
	addIfPresent(attributes, "reason_ref", payload.reasonRef)
	addIfPresent(attributes, "approval_id", payload.approvalID)
	addIfPresent(attributes, "decision", payload.decision)
	addIfPresent(attributes, "result_id", payload.resultID)
	addIfPresent(attributes, "downstream_service", payload.downstreamService)
	addIfPresent(attributes, "downstream_request_ref", payload.downstreamRequestRef)
	addIfPresent(attributes, "failure_class", payload.failureClass)
	addIfPresent(attributes, "compensation_requested_by_hash", payload.compensationRequestedByHash)
	addIfPresent(attributes, "compensation_reason_ref", payload.compensationReasonRef)
	encoded, err := json.Marshal(attributes)
	if err != nil {
		return "", types.NewInvalidArgument("admin event attributes are invalid")
	}
	return string(encoded), nil
}

func addIfPresent(attributes map[string]any, key string, value string) {
	if strings.TrimSpace(value) != "" {
		attributes[key] = strings.TrimSpace(value)
	}
}

func fillSubmitted(base *adminPayloadBase, payload *admineventsv1.AdminOperationSubmittedV1) {
	if payload == nil {
		return
	}
	base.payloadTenantID = payload.GetTenantId()
	base.operationID = payload.GetOperationId()
	base.operationType = payload.GetOperationType()
	base.targetRefHash = payload.GetTargetRefHash()
	base.riskLevel = payload.GetRiskLevel()
	base.status = payload.GetStatus()
	base.requestedByHash = payload.GetRequestedByHash()
	base.payloadHash = payload.GetPayloadHash()
	base.payloadSchemaVersion = payload.GetPayloadSchemaVersion()
	base.reasonRef = payload.GetReasonRef()
	base.action = "SUBMIT_ADMIN_OPERATION"
	base.outcome = payload.GetStatus()
	base.reasonCode = "ADMIN_OPERATION_SUBMITTED"
}

func fillApproved(base *adminPayloadBase, payload *admineventsv1.AdminOperationApprovedV1) {
	if payload == nil {
		return
	}
	fillDecision(base, payload.GetTenantId(), payload.GetOperationId(), payload.GetOperationType(), payload.GetTargetRefHash(), payload.GetRiskLevel(), payload.GetStatus(), payload.GetRequestedByHash(), payload.GetApprovedByHash(), payload.GetApprovalId(), payload.GetDecision(), payload.GetPayloadHash())
	base.action = "APPROVE_ADMIN_OPERATION"
	base.reasonCode = "ADMIN_OPERATION_APPROVED"
}

func fillRejected(base *adminPayloadBase, payload *admineventsv1.AdminOperationRejectedV1) {
	if payload == nil {
		return
	}
	fillDecision(base, payload.GetTenantId(), payload.GetOperationId(), payload.GetOperationType(), payload.GetTargetRefHash(), payload.GetRiskLevel(), payload.GetStatus(), payload.GetRequestedByHash(), payload.GetApprovedByHash(), payload.GetApprovalId(), payload.GetDecision(), payload.GetPayloadHash())
	base.action = "REJECT_ADMIN_OPERATION"
	base.reasonCode = "ADMIN_OPERATION_REJECTED"
}

func fillDecision(base *adminPayloadBase, tenantID string, operationID string, operationType string, targetRefHash string, riskLevel string, status string, requestedByHash string, approvedByHash string, approvalID string, decision string, payloadHash string) {
	base.payloadTenantID = tenantID
	base.operationID = operationID
	base.operationType = operationType
	base.targetRefHash = targetRefHash
	base.riskLevel = riskLevel
	base.status = status
	base.requestedByHash = requestedByHash
	base.approvedByHash = approvedByHash
	base.approvalID = approvalID
	base.decision = decision
	base.payloadHash = payloadHash
	base.outcome = status
}

func fillExecuted(base *adminPayloadBase, payload *admineventsv1.AdminOperationExecutedV1) {
	if payload == nil {
		return
	}
	fillResult(base, payload.GetTenantId(), payload.GetOperationId(), payload.GetOperationType(), payload.GetTargetRefHash(), payload.GetRiskLevel(), payload.GetStatus(), payload.GetRequestedByHash(), payload.GetApprovedByHash(), payload.GetResultId(), payload.GetDownstreamService(), payload.GetDownstreamRequestRef(), payload.GetPayloadHash())
	base.action = "EXECUTE_ADMIN_OPERATION"
	base.reasonCode = "ADMIN_OPERATION_EXECUTED"
}

func fillFailed(base *adminPayloadBase, payload *admineventsv1.AdminOperationFailedV1) {
	if payload == nil {
		return
	}
	fillResult(base, payload.GetTenantId(), payload.GetOperationId(), payload.GetOperationType(), payload.GetTargetRefHash(), payload.GetRiskLevel(), payload.GetStatus(), payload.GetRequestedByHash(), payload.GetApprovedByHash(), payload.GetResultId(), payload.GetDownstreamService(), payload.GetDownstreamRequestRef(), payload.GetPayloadHash())
	base.failureClass = payload.GetFailureClass()
	base.action = "FAIL_ADMIN_OPERATION"
	base.reasonCode = "ADMIN_OPERATION_FAILED"
}

func fillResult(base *adminPayloadBase, tenantID string, operationID string, operationType string, targetRefHash string, riskLevel string, status string, requestedByHash string, approvedByHash string, resultID string, downstreamService string, downstreamRequestRef string, payloadHash string) {
	base.payloadTenantID = tenantID
	base.operationID = operationID
	base.operationType = operationType
	base.targetRefHash = targetRefHash
	base.riskLevel = riskLevel
	base.status = status
	base.requestedByHash = requestedByHash
	base.approvedByHash = approvedByHash
	base.resultID = resultID
	base.downstreamService = downstreamService
	base.downstreamRequestRef = downstreamRequestRef
	base.payloadHash = payloadHash
	base.outcome = status
}

func fillCompensationRequested(base *adminPayloadBase, payload *admineventsv1.AdminOperationCompensationRequestedV1) {
	if payload == nil {
		return
	}
	base.payloadTenantID = payload.GetTenantId()
	base.operationID = payload.GetOperationId()
	base.operationType = payload.GetOperationType()
	base.targetRefHash = payload.GetTargetRefHash()
	base.riskLevel = payload.GetRiskLevel()
	base.status = payload.GetStatus()
	base.requestedByHash = payload.GetRequestedByHash()
	base.approvedByHash = payload.GetApprovedByHash()
	base.compensationRequestedByHash = payload.GetCompensationRequestedByHash()
	base.compensationReasonRef = payload.GetCompensationReasonRef()
	base.payloadHash = payload.GetPayloadHash()
	base.action = "REQUEST_ADMIN_COMPENSATION"
	base.outcome = payload.GetStatus()
	base.reasonCode = "ADMIN_OPERATION_COMPENSATION_REQUESTED"
}

func (worker *Worker) recordError() {
	worker.metrics.totalErrors.Add(1)
	worker.metrics.consecutiveErrors.Add(1)
	worker.metrics.lastErrorAtMS.Store(time.Now().UnixMilli())
}

func (worker *Worker) recordSuccess() {
	worker.metrics.consecutiveErrors.Store(0)
	worker.metrics.lastSuccessAtMS.Store(time.Now().UnixMilli())
}

func waitForInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
