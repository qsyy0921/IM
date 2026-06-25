package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

const (
	defaultExternalCallbackDeliveryLimit      = 50
	defaultExternalCallbackDeliveryLease      = 30 * time.Second
	defaultExternalCallbackDeliveryRetryDelay = time.Second
)

func (repository *Repository) RegisterExternalCallbackDelivery(
	ctx context.Context,
	delivery types.WorkflowExternalCallbackDelivery,
) (types.WorkflowExternalCallbackDelivery, bool, error) {
	if repository.pool == nil {
		return types.WorkflowExternalCallbackDelivery{}, false, types.NewDBWriteFailed("workflow repository is not configured")
	}
	delivery = delivery.Normalized()
	if err := delivery.Validate(); err != nil {
		return types.WorkflowExternalCallbackDelivery{}, false, err
	}
	now := time.Now().UTC()
	if delivery.AvailableAt.IsZero() {
		delivery.AvailableAt = now
	}
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = now
	}
	if delivery.UpdatedAt.IsZero() {
		delivery.UpdatedAt = now
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, false, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	workflow, err := getWorkflowForUpdate(ctx, tx, delivery.TenantID, delivery.WorkflowID)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, false, err
	}
	if err := verifyExternalCallbackWorkflowBinding(workflow, delivery); err != nil {
		return types.WorkflowExternalCallbackDelivery{}, false, err
	}
	inserted, err := insertExternalCallbackDelivery(ctx, tx, delivery)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, false, err
	}
	if !inserted {
		existing, err := getExternalCallbackDeliveryForUpdate(ctx, tx, delivery.TenantID, delivery.DeliveryID)
		if err != nil {
			return types.WorkflowExternalCallbackDelivery{}, false, err
		}
		if !sameExternalCallbackDeliveryPlan(existing, delivery) {
			return types.WorkflowExternalCallbackDelivery{}, false, types.NewAlreadyExists("workflow external callback delivery already exists")
		}
		if err := tx.Commit(ctx); err != nil {
			return types.WorkflowExternalCallbackDelivery{}, false, types.NewDBWriteFailed(err.Error())
		}
		return existing, true, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return types.WorkflowExternalCallbackDelivery{}, false, types.NewDBWriteFailed(err.Error())
	}
	return delivery, false, nil
}

func (repository *Repository) ClaimReadyExternalCallbackDeliveries(
	ctx context.Context,
	now time.Time,
	limit int,
	leaseDuration time.Duration,
) ([]types.WorkflowExternalCallbackDelivery, error) {
	if repository.pool == nil {
		return nil, types.NewDBWriteFailed("workflow repository is not configured")
	}
	if limit <= 0 {
		limit = defaultExternalCallbackDeliveryLimit
	}
	if leaseDuration <= 0 {
		leaseDuration = defaultExternalCallbackDeliveryLease
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	leasedUntil := now.Add(leaseDuration)

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
WITH candidates AS (
    SELECT delivery.tenant_id, delivery.delivery_id
    FROM workflow_external_callback_deliveries AS delivery
    JOIN workflow_requests AS workflow
      ON workflow.tenant_id = delivery.tenant_id
     AND workflow.workflow_id = delivery.workflow_id
    WHERE workflow.status = $5
      AND (
        (delivery.status IN ($2, $3) AND delivery.available_at <= $6)
        OR (delivery.status = $4 AND delivery.leased_until IS NOT NULL AND delivery.leased_until <= $6)
      )
    ORDER BY delivery.available_at, delivery.created_at, delivery.delivery_id
    LIMIT $1
    FOR UPDATE OF delivery SKIP LOCKED
)
UPDATE workflow_external_callback_deliveries AS delivery
SET status = $4,
    attempt_count = delivery.attempt_count + 1,
    leased_until = $7,
    last_attempt_at = $6,
    updated_at = $6
FROM candidates
WHERE delivery.tenant_id = candidates.tenant_id
  AND delivery.delivery_id = candidates.delivery_id
RETURNING `+selectExternalCallbackDeliveryColumns("delivery"),
		limit,
		types.WorkflowExternalCallbackDeliveryStatusPending,
		types.WorkflowExternalCallbackDeliveryStatusRetryPending,
		types.WorkflowExternalCallbackDeliveryStatusInFlight,
		types.WorkflowStatusWaitingDecision,
		now,
		leasedUntil)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	deliveries := make([]types.WorkflowExternalCallbackDelivery, 0, limit)
	for rows.Next() {
		delivery, err := scanExternalCallbackDelivery(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return deliveries, nil
}

func (repository *Repository) MarkExternalCallbackDeliveryDelivered(
	ctx context.Context,
	delivery types.WorkflowExternalCallbackDelivery,
	result types.WorkflowExternalCallbackDeliveryResult,
) (types.WorkflowExternalCallbackDelivery, error) {
	if repository.pool == nil {
		return types.WorkflowExternalCallbackDelivery{}, types.NewDBWriteFailed("workflow repository is not configured")
	}
	result.DeliveryResultRef = strings.TrimSpace(result.DeliveryResultRef)
	if result.DeliveryResultRef == "" || containsSensitiveExternalCallbackValue(result.DeliveryResultRef) {
		return types.WorkflowExternalCallbackDelivery{}, types.NewInvalidArgument("delivery result ref must be low-sensitive")
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	locked, err := getExternalCallbackDeliveryForUpdate(ctx, tx, delivery.TenantID, delivery.DeliveryID)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, err
	}
	if locked.Status == types.WorkflowExternalCallbackDeliveryStatusDelivered {
		if err := tx.Commit(ctx); err != nil {
			return types.WorkflowExternalCallbackDelivery{}, types.NewDBWriteFailed(err.Error())
		}
		return locked, nil
	}
	if locked.Status != types.WorkflowExternalCallbackDeliveryStatusInFlight {
		return types.WorkflowExternalCallbackDelivery{}, types.NewFailedPrecondition("workflow external callback delivery is not in flight")
	}
	workflow, err := getWorkflowForUpdate(ctx, tx, locked.TenantID, locked.WorkflowID)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, err
	}
	now := time.Now().UTC()
	updated, err := updateExternalCallbackDeliveryDelivered(ctx, tx, locked, result.DeliveryResultRef, now)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, err
	}
	if err := insertExternalCallbackDeliveryOutbox(ctx, tx, workflow, updated, types.WorkflowEventExternalCallbackDelivered); err != nil {
		return types.WorkflowExternalCallbackDelivery{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.WorkflowExternalCallbackDelivery{}, types.NewDBWriteFailed(err.Error())
	}
	return updated, nil
}

func (repository *Repository) MarkExternalCallbackDeliveryFailed(
	ctx context.Context,
	delivery types.WorkflowExternalCallbackDelivery,
	result types.WorkflowExternalCallbackDeliveryResult,
	nextRetryAt time.Time,
) (types.WorkflowExternalCallbackDelivery, error) {
	if repository.pool == nil {
		return types.WorkflowExternalCallbackDelivery{}, types.NewDBWriteFailed("workflow repository is not configured")
	}
	result.FailureClass = strings.TrimSpace(result.FailureClass)
	if result.FailureClass == "" || containsSensitiveExternalCallbackValue(result.FailureClass) {
		return types.WorkflowExternalCallbackDelivery{}, types.NewInvalidArgument("failure class must be low-sensitive")
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	locked, err := getExternalCallbackDeliveryForUpdate(ctx, tx, delivery.TenantID, delivery.DeliveryID)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, err
	}
	if locked.Status == types.WorkflowExternalCallbackDeliveryStatusDelivered ||
		locked.Status == types.WorkflowExternalCallbackDeliveryStatusDLQ {
		if err := tx.Commit(ctx); err != nil {
			return types.WorkflowExternalCallbackDelivery{}, types.NewDBWriteFailed(err.Error())
		}
		return locked, nil
	}
	if locked.Status != types.WorkflowExternalCallbackDeliveryStatusInFlight {
		return types.WorkflowExternalCallbackDelivery{}, types.NewFailedPrecondition("workflow external callback delivery is not in flight")
	}
	workflow, err := getWorkflowForUpdate(ctx, tx, locked.TenantID, locked.WorkflowID)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, err
	}
	now := time.Now().UTC()
	status := types.WorkflowExternalCallbackDeliveryStatusRetryPending
	availableAt := nextRetryAt.UTC()
	if availableAt.IsZero() {
		availableAt = now.Add(defaultExternalCallbackDeliveryRetryDelay)
	}
	if locked.AttemptCount >= locked.MaxAttempts {
		status = types.WorkflowExternalCallbackDeliveryStatusDLQ
		availableAt = now
	}
	updated, err := updateExternalCallbackDeliveryFailed(ctx, tx, locked, status, result.FailureClass, availableAt, now)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, err
	}
	if updated.Status == types.WorkflowExternalCallbackDeliveryStatusDLQ {
		if err := insertExternalCallbackDeliveryOutbox(ctx, tx, workflow, updated, types.WorkflowEventExternalCallbackDLQ); err != nil {
			return types.WorkflowExternalCallbackDelivery{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.WorkflowExternalCallbackDelivery{}, types.NewDBWriteFailed(err.Error())
	}
	return updated, nil
}

func verifyExternalCallbackWorkflowBinding(workflow types.Workflow, delivery types.WorkflowExternalCallbackDelivery) error {
	if workflow.Status != types.WorkflowStatusWaitingDecision {
		return types.NewFailedPrecondition("workflow is not waiting for decision")
	}
	if workflow.WorkflowType != delivery.WorkflowType ||
		workflow.CurrentStepID != delivery.StepID ||
		workflow.TargetService != delivery.TargetService ||
		workflow.TargetOperation != delivery.TargetOperation ||
		workflow.TargetRefHash != delivery.TargetRefHash ||
		workflow.PayloadSchemaVersion != delivery.PayloadSchemaVersion ||
		workflow.PayloadRefHash != delivery.PayloadRefHash ||
		workflow.ApprovalPolicyRef != delivery.ApprovalPolicyRef {
		return types.NewFailedPrecondition("workflow external callback delivery binding mismatch")
	}
	return nil
}

func insertExternalCallbackDelivery(
	ctx context.Context,
	tx pgx.Tx,
	delivery types.WorkflowExternalCallbackDelivery,
) (bool, error) {
	tag, err := tx.Exec(ctx, `
INSERT INTO workflow_external_callback_deliveries (
    tenant_id, workflow_id, delivery_id, delivery_plan_sha256, source_decision_manifest_sha256,
    step_id, workflow_type, target_service, target_operation, target_ref_hash,
    payload_schema_version, payload_ref_hash, approval_policy_ref, decision_policy_ref,
    callback_provider_ref, callback_endpoint_ref, delivery_queue_ref, retry_policy_ref,
    backoff_policy_ref, callback_timeout_policy_ref, callback_payload_schema_version,
    callback_payload_ref_hash, status, attempt_count, max_attempts, available_at,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13, $14,
    $15, $16, $17, $18,
    $19, $20, $21,
    $22, $23, $24, $25, $26,
    $27, $28
)
ON CONFLICT (tenant_id, delivery_id) DO NOTHING
`, string(delivery.TenantID), delivery.WorkflowID, delivery.DeliveryID,
		delivery.DeliveryPlanSha256, delivery.SourceDecisionManifestSha256,
		delivery.StepID, delivery.WorkflowType, delivery.TargetService,
		delivery.TargetOperation, delivery.TargetRefHash,
		delivery.PayloadSchemaVersion, delivery.PayloadRefHash,
		delivery.ApprovalPolicyRef, delivery.DecisionPolicyRef,
		delivery.CallbackProviderRef, delivery.CallbackEndpointRef,
		delivery.DeliveryQueueRef, delivery.RetryPolicyRef,
		delivery.BackoffPolicyRef, delivery.CallbackTimeoutPolicyRef,
		delivery.CallbackPayloadSchemaVersion, delivery.CallbackPayloadRefHash,
		delivery.Status, delivery.AttemptCount, delivery.MaxAttempts,
		delivery.AvailableAt, delivery.CreatedAt, delivery.UpdatedAt)
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	return tag.RowsAffected() == 1, nil
}

func getExternalCallbackDeliveryForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	deliveryID string,
) (types.WorkflowExternalCallbackDelivery, error) {
	row := tx.QueryRow(ctx, `
SELECT `+selectExternalCallbackDeliveryColumns("")+`
FROM workflow_external_callback_deliveries
WHERE tenant_id = $1 AND delivery_id = $2
FOR UPDATE
`, string(tenantID), deliveryID)
	delivery, err := scanExternalCallbackDelivery(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.WorkflowExternalCallbackDelivery{}, types.NewNotFound("workflow external callback delivery not found")
		}
		return types.WorkflowExternalCallbackDelivery{}, types.NewDBReadFailed(err.Error())
	}
	return delivery, nil
}

func updateExternalCallbackDeliveryDelivered(
	ctx context.Context,
	tx pgx.Tx,
	delivery types.WorkflowExternalCallbackDelivery,
	resultRef string,
	now time.Time,
) (types.WorkflowExternalCallbackDelivery, error) {
	row := tx.QueryRow(ctx, `
UPDATE workflow_external_callback_deliveries
SET status = $3,
    leased_until = NULL,
    delivered_at = $4,
    last_delivery_result_ref = $5,
    updated_at = $4
WHERE tenant_id = $1 AND delivery_id = $2
RETURNING `+selectExternalCallbackDeliveryColumns("workflow_external_callback_deliveries"),
		string(delivery.TenantID), delivery.DeliveryID,
		types.WorkflowExternalCallbackDeliveryStatusDelivered, now, resultRef)
	updated, err := scanExternalCallbackDelivery(row)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, types.NewDBWriteFailed(err.Error())
	}
	return updated, nil
}

func updateExternalCallbackDeliveryFailed(
	ctx context.Context,
	tx pgx.Tx,
	delivery types.WorkflowExternalCallbackDelivery,
	status string,
	failureClass string,
	availableAt time.Time,
	now time.Time,
) (types.WorkflowExternalCallbackDelivery, error) {
	row := tx.QueryRow(ctx, `
UPDATE workflow_external_callback_deliveries
SET status = $3,
    leased_until = NULL,
    available_at = $4,
    last_failure_class = $5,
    updated_at = $6
WHERE tenant_id = $1 AND delivery_id = $2
RETURNING `+selectExternalCallbackDeliveryColumns("workflow_external_callback_deliveries"),
		string(delivery.TenantID), delivery.DeliveryID, status, availableAt, failureClass, now)
	updated, err := scanExternalCallbackDelivery(row)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, types.NewDBWriteFailed(err.Error())
	}
	return updated, nil
}

func insertExternalCallbackDeliveryOutbox(
	ctx context.Context,
	tx pgx.Tx,
	workflow types.Workflow,
	delivery types.WorkflowExternalCallbackDelivery,
	eventType string,
) error {
	payload := workflowPayload(workflow)
	payload["delivery_id"] = delivery.DeliveryID
	payload["delivery_status"] = delivery.Status
	payload["delivery_plan_sha256"] = delivery.DeliveryPlanSha256
	payload["callback_provider_ref"] = delivery.CallbackProviderRef
	payload["callback_endpoint_ref"] = delivery.CallbackEndpointRef
	payload["delivery_queue_ref"] = delivery.DeliveryQueueRef
	payload["attempt_count"] = delivery.AttemptCount
	payload["max_attempts"] = delivery.MaxAttempts
	payload["last_failure_class"] = delivery.LastFailureClass
	payload["last_delivery_result_ref"] = delivery.LastDeliveryResultRef
	return insertOutbox(ctx, tx, "evt_"+delivery.DeliveryID+"_"+delivery.Status, workflow, eventType, payload)
}

func sameExternalCallbackDeliveryPlan(
	left types.WorkflowExternalCallbackDelivery,
	right types.WorkflowExternalCallbackDelivery,
) bool {
	return left.WorkflowID == right.WorkflowID &&
		left.DeliveryPlanSha256 == right.DeliveryPlanSha256 &&
		left.StepID == right.StepID &&
		left.WorkflowType == right.WorkflowType &&
		left.TargetService == right.TargetService &&
		left.TargetOperation == right.TargetOperation &&
		left.TargetRefHash == right.TargetRefHash &&
		left.PayloadSchemaVersion == right.PayloadSchemaVersion &&
		left.PayloadRefHash == right.PayloadRefHash &&
		left.ApprovalPolicyRef == right.ApprovalPolicyRef &&
		left.DecisionPolicyRef == right.DecisionPolicyRef &&
		left.CallbackProviderRef == right.CallbackProviderRef &&
		left.CallbackEndpointRef == right.CallbackEndpointRef &&
		left.DeliveryQueueRef == right.DeliveryQueueRef &&
		left.RetryPolicyRef == right.RetryPolicyRef &&
		left.BackoffPolicyRef == right.BackoffPolicyRef &&
		left.CallbackTimeoutPolicyRef == right.CallbackTimeoutPolicyRef &&
		left.CallbackPayloadSchemaVersion == right.CallbackPayloadSchemaVersion &&
		left.CallbackPayloadRefHash == right.CallbackPayloadRefHash &&
		left.MaxAttempts == right.MaxAttempts
}

func selectExternalCallbackDeliveryColumns(alias string) string {
	prefix := ""
	if strings.TrimSpace(alias) != "" {
		prefix = alias + "."
	}
	return prefix + `tenant_id, ` + prefix + `workflow_id, ` + prefix + `delivery_id,
       ` + prefix + `delivery_plan_sha256, ` + prefix + `source_decision_manifest_sha256,
       ` + prefix + `step_id, ` + prefix + `workflow_type, ` + prefix + `target_service,
       ` + prefix + `target_operation, ` + prefix + `target_ref_hash,
       ` + prefix + `payload_schema_version, ` + prefix + `payload_ref_hash,
       ` + prefix + `approval_policy_ref, ` + prefix + `decision_policy_ref,
       ` + prefix + `callback_provider_ref, ` + prefix + `callback_endpoint_ref,
       ` + prefix + `delivery_queue_ref, ` + prefix + `retry_policy_ref,
       ` + prefix + `backoff_policy_ref, ` + prefix + `callback_timeout_policy_ref,
       ` + prefix + `callback_payload_schema_version, ` + prefix + `callback_payload_ref_hash,
       ` + prefix + `status, ` + prefix + `attempt_count, ` + prefix + `max_attempts,
       ` + prefix + `available_at, ` + prefix + `leased_until, ` + prefix + `last_attempt_at,
       ` + prefix + `delivered_at, ` + prefix + `last_failure_class,
       ` + prefix + `last_delivery_result_ref, ` + prefix + `created_at, ` + prefix + `updated_at`
}

func scanExternalCallbackDelivery(row scanner) (types.WorkflowExternalCallbackDelivery, error) {
	var delivery types.WorkflowExternalCallbackDelivery
	var leasedUntil *time.Time
	var lastAttemptAt *time.Time
	var deliveredAt *time.Time
	err := row.Scan(
		&delivery.TenantID, &delivery.WorkflowID, &delivery.DeliveryID,
		&delivery.DeliveryPlanSha256, &delivery.SourceDecisionManifestSha256,
		&delivery.StepID, &delivery.WorkflowType, &delivery.TargetService,
		&delivery.TargetOperation, &delivery.TargetRefHash,
		&delivery.PayloadSchemaVersion, &delivery.PayloadRefHash,
		&delivery.ApprovalPolicyRef, &delivery.DecisionPolicyRef,
		&delivery.CallbackProviderRef, &delivery.CallbackEndpointRef,
		&delivery.DeliveryQueueRef, &delivery.RetryPolicyRef,
		&delivery.BackoffPolicyRef, &delivery.CallbackTimeoutPolicyRef,
		&delivery.CallbackPayloadSchemaVersion, &delivery.CallbackPayloadRefHash,
		&delivery.Status, &delivery.AttemptCount, &delivery.MaxAttempts,
		&delivery.AvailableAt, &leasedUntil, &lastAttemptAt,
		&deliveredAt, &delivery.LastFailureClass,
		&delivery.LastDeliveryResultRef, &delivery.CreatedAt, &delivery.UpdatedAt,
	)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, err
	}
	if leasedUntil != nil {
		delivery.LeasedUntil = *leasedUntil
	}
	if lastAttemptAt != nil {
		delivery.LastAttemptAt = *lastAttemptAt
	}
	if deliveredAt != nil {
		delivery.DeliveredAt = *deliveredAt
	}
	return delivery, nil
}

func containsSensitiveExternalCallbackValue(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, marker := range []string{"secret", "token", "api_key", "apikey", "password", "private://", "raw:", "dsn=", "postgres://", "http://", "https://", "provider_body"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
