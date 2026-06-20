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
