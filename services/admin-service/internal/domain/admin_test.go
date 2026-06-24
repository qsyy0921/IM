package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

func TestPrepareCreateRejectsSensitivePayload(t *testing.T) {
	_, err := PrepareCreate(validCreateCommand(`{"token":"secret"}`), "op_test_1", time.Now())
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument for sensitive payload, got %v", err)
	}
}

func TestPrepareCreateRequiresEvidenceForHighRisk(t *testing.T) {
	command := validCreateCommand(`{"target_user_ref":"user:123"}`)
	command.RiskLevel = types.RiskLevelHigh
	command.EvidenceRefs = nil
	if _, err := PrepareCreate(command, "op_test_1", time.Now()); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected high risk evidence requirement, got %v", err)
	}
}

func TestValidateApprovalTransitionEnforcesSeparationOfDuty(t *testing.T) {
	operation := OperationFromPrepared(mustPrepareCreate(t, validCreateCommand(`{"target_user_ref":"user:123"}`)))
	operation.RiskLevel = types.RiskLevelHigh
	approval := types.AdminApproval{ApproverRef: operation.RequestedBy, Decision: types.DecisionApprove}
	if err := ValidateApprovalTransition(operation, approval); !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected separation of duty denial, got %v", err)
	}
}

func TestNormalizePayloadCanonicalizesJSON(t *testing.T) {
	payload, err := NormalizePayload(`{"quota_burst":20,"quota_rps":10}`)
	if err != nil {
		t.Fatalf("normalize payload: %v", err)
	}
	if !strings.Contains(payload, "quota_burst") || !strings.Contains(payload, "quota_rps") {
		t.Fatalf("unexpected payload: %s", payload)
	}
}

func TestPrepareCreateAcceptsConfigPublishPayload(t *testing.T) {
	command := validCreateCommand(`{
		"environment":"local",
		"config_kind":"API_GATEWAY_TENANT_QUOTA",
		"bundle_key":"api-gateway/default",
		"version":"quota-v1",
		"schema_version":"quota-v1",
		"effective_at_unix_ms":1000,
		"payload_json":"{\"plans\":{\"tenant-free\":{\"requests_per_second\":20,\"burst\":40}}}"
	}`)
	command.OperationType = "CONFIG_PUBLISH"
	command.PayloadSchemaVersion = "admin.config_publish.v1"

	prepared, err := PrepareCreate(command, "op_config_publish", time.Now())
	if err != nil {
		t.Fatalf("prepare create: %v", err)
	}
	if !strings.Contains(prepared.PayloadJSON, "payload_json") ||
		!strings.Contains(prepared.PayloadJSON, "API_GATEWAY_TENANT_QUOTA") {
		t.Fatalf("unexpected payload: %s", prepared.PayloadJSON)
	}
}

func TestPrepareCreateRejectsSensitiveConfigPublishPayload(t *testing.T) {
	command := validCreateCommand(`{
		"environment":"local",
		"config_kind":"API_GATEWAY_TENANT_QUOTA",
		"bundle_key":"api-gateway/default",
		"version":"quota-v1",
		"schema_version":"quota-v1",
		"payload_json":"{\"secret\":\"do-not-store\"}"
	}`)
	command.OperationType = "CONFIG_PUBLISH"
	command.PayloadSchemaVersion = "admin.config_publish.v1"

	if _, err := PrepareCreate(command, "op_config_publish", time.Now()); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestPrepareCreateAcceptsConfigRollbackPayload(t *testing.T) {
	command := validCreateCommand(`{
		"environment":"local",
		"config_kind":"API_GATEWAY_TENANT_QUOTA",
		"bundle_key":"api-gateway/default",
		"target_version":"quota-v1"
	}`)
	command.OperationType = "CONFIG_ROLLBACK"
	command.PayloadSchemaVersion = "admin.config_rollback.v1"

	prepared, err := PrepareCreate(command, "op_config_rollback", time.Now())
	if err != nil {
		t.Fatalf("prepare create: %v", err)
	}
	if !strings.Contains(prepared.PayloadJSON, "target_version") {
		t.Fatalf("unexpected payload: %s", prepared.PayloadJSON)
	}
}

func TestPrepareCreateAcceptsTenantQuotaPayload(t *testing.T) {
	command := validCreateCommand(`{
		"environment":"local",
		"bundle_key":"api-gateway/default",
		"config_version":"quota-v1",
		"tenant_ref":"tenant-free",
		"quota_rps":20,
		"quota_burst":40,
		"effective_at_unix_ms":1000
	}`)
	command.OperationType = "TENANT_QUOTA_CHANGE"
	command.PayloadSchemaVersion = "admin.tenant_quota_change.v1"

	prepared, err := PrepareCreate(command, "op_tenant_quota", time.Now())
	if err != nil {
		t.Fatalf("prepare create: %v", err)
	}
	if !strings.Contains(prepared.PayloadJSON, "quota_rps") ||
		!strings.Contains(prepared.PayloadJSON, "tenant-free") {
		t.Fatalf("unexpected payload: %s", prepared.PayloadJSON)
	}
}

func TestPrepareCreateAcceptsProviderReplayRequestPayload(t *testing.T) {
	command := validCreateCommand(`{
		"provider_failure_ref_hash":"sha256:provider-failure",
		"source_execution_ref_hash":"sha256:execution",
		"source_result_ref_hash":"sha256:result",
		"replay_candidate_id":"provider-replay-candidate-1234",
		"redrive_entrypoint":"RedriveProviderFailure",
		"requires_fresh_proposal":true,
		"requires_fresh_approval":true,
		"requires_prepared_audit":true,
		"requires_new_input":true,
		"requires_reason_sha256":true,
		"source_dlq_immutable":true,
		"direct_execution_allowed":false
	}`)
	command.OperationType = "PROVIDER_REPLAY_REQUEST"
	command.PayloadSchemaVersion = "admin.provider_replay_request.v1"
	command.RiskLevel = types.RiskLevelHigh

	prepared, err := PrepareCreate(command, "op_provider_replay", time.Now())
	if err != nil {
		t.Fatalf("prepare create: %v", err)
	}
	if !strings.Contains(prepared.PayloadJSON, "RedriveProviderFailure") ||
		!strings.Contains(prepared.PayloadJSON, "provider_failure_ref_hash") {
		t.Fatalf("unexpected payload: %s", prepared.PayloadJSON)
	}
}

func TestPrepareCreateRejectsSensitiveProviderReplayPayload(t *testing.T) {
	command := validCreateCommand(`{
		"provider_failure_ref_hash":"sha256:provider-failure",
		"redrive_entrypoint":"RedriveProviderFailure",
		"message_body":"raw provider input"
	}`)
	command.OperationType = "PROVIDER_REPLAY_REQUEST"
	command.PayloadSchemaVersion = "admin.provider_replay_request.v1"
	command.RiskLevel = types.RiskLevelHigh

	if _, err := PrepareCreate(command, "op_provider_replay", time.Now()); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func validCreateCommand(payload string) types.CreateAdminOperationCommand {
	return types.CreateAdminOperationCommand{
		AuthContext:          types.AuthContext{TenantID: "tenant-admin-test", ServiceName: "admin-ui"},
		OperatorRef:          "admin:requester",
		OperatorRole:         "ADMIN",
		OperationType:        "USER_BAN",
		TargetRefHash:        "sha256:target-ref",
		RiskLevel:            types.RiskLevelMedium,
		PayloadSchemaVersion: "admin.user_ban.v1",
		OperationPayloadJSON: payload,
		IdempotencyKey:       "admin-create-idem",
		ReasonRef:            "reason:ticket-1",
		EvidenceRefs:         []string{"evidence:ticket-1"},
	}
}

func mustPrepareCreate(t *testing.T, command types.CreateAdminOperationCommand) PreparedOperation {
	t.Helper()
	prepared, err := PrepareCreate(command, "op_test_1", time.Now())
	if err != nil {
		t.Fatalf("prepare create: %v", err)
	}
	return prepared
}
