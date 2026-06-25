package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

func TestValidateWorkflowMode(t *testing.T) {
	for _, mode := range []string{
		"noop",
		"grpc",
		"timer-worker",
		"compensation-worker",
		"compensation-executor",
		"compensation-instruction-import",
		"external-callback-delivery-import",
		"external-callback-delivery-redrive",
		"external-callback-delivery-worker",
	} {
		if err := validateWorkflowMode(mode); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
	if err := validateWorkflowMode("bad-mode"); err == nil {
		t.Fatal("expected unsupported mode to fail")
	}
}

func TestValidateWorkflowDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validateWorkflowDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty debug listener should be allowed: %v", err)
	}
	if err := validateWorkflowDebugListenerConfig("127.0.0.1:11934", false); err != nil {
		t.Fatalf("loopback debug listener should be allowed: %v", err)
	}
	if err := validateWorkflowDebugListenerConfig("172.30.80.37:11934", false); err != nil {
		t.Fatalf("private debug listener should be allowed: %v", err)
	}
}

func TestValidateWorkflowDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateWorkflowDebugListenerConfig("0.0.0.0:11934", false); err == nil {
		t.Fatal("public debug listener should require explicit override")
	}
}

func TestValidateWorkflowDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateWorkflowDebugListenerConfig("0.0.0.0:11934", true); err != nil {
		t.Fatalf("explicit public debug listener opt-in should be allowed: %v", err)
	}
}

func TestWriteExternalCallbackRedriveExecutionSummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redrive-summary.json")
	plan := types.WorkflowExternalCallbackRedrivePlan{
		TenantID:                   "tenant-workflow",
		RedrivePlanID:              "redrive-plan-1",
		RedrivePlanSha256:          "sha256:redrive-plan-1",
		SourceDeliveryStatusSha256: "sha256:delivery-status-1",
		SourceDeliveryPlanSha256:   "sha256:delivery-plan-1",
		WorkflowID:                 "workflow-1",
		StepID:                     "step-1",
		WorkflowType:               types.WorkflowTypeActionApproval,
		TargetService:              "external-crm",
		TargetOperation:            "SYNC_APPROVAL_CALLBACK",
		TargetRefHash:              "sha256:target-1",
		PayloadSchemaVersion:       "external.callback_request.v1",
		PayloadRefHash:             "sha256:payload-1",
		ApprovalPolicyRef:          "workflow.external_callback.v1",
		DecisionPolicyRef:          "workflow.external_callback.decision.v1",
		DeliveryStatus:             types.WorkflowExternalCallbackDeliveryStatusDLQ,
		AttemptNumber:              3,
		MaxAttempts:                3,
		DeliveryAttemptRef:         "attempt:workflow-1",
		FailureClassRef:            "failure:retry-exhausted",
		RedrivePolicyRef:           "workflow.external-callback-redrive.v1",
		RedriveQueueRef:            "queue:workflow-callback-redrive",
		RedriveReasonRef:           "reason-sha256:callback-redrive",
	}
	redriven := types.WorkflowExternalCallbackDelivery{
		TenantID:              "tenant-workflow",
		WorkflowID:            "workflow-1",
		DeliveryID:            "delivery-1",
		StepID:                "step-1",
		TargetService:         "external-crm",
		TargetOperation:       "SYNC_APPROVAL_CALLBACK",
		TargetRefHash:         "sha256:target-1",
		PayloadSchemaVersion:  "external.callback_request.v1",
		PayloadRefHash:        "sha256:payload-1",
		ApprovalPolicyRef:     "workflow.external_callback.v1",
		DecisionPolicyRef:     "workflow.external_callback.decision.v1",
		Status:                types.WorkflowExternalCallbackDeliveryStatusPending,
		RedriveCount:          1,
		LastRedrivePlanSha256: "sha256:redrive-plan-1",
		LastRedriveReasonRef:  "reason-sha256:callback-redrive",
	}

	if err := writeExternalCallbackRedriveExecutionSummary(path, plan, redriven); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var summary externalCallbackRedriveExecutionSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.SchemaVersion != "nexusim.workflow.external_callback_redrive_execution_summary.v1" ||
		summary.Mode != "external-callback-delivery-redrive" ||
		summary.RedrivePlanID != plan.RedrivePlanID ||
		summary.RedrivePlanSha256 != plan.RedrivePlanSha256 ||
		summary.WorkflowID != redriven.WorkflowID ||
		summary.DeliveryID != redriven.DeliveryID ||
		summary.DeliveryStatus != types.WorkflowExternalCallbackDeliveryStatusPending ||
		summary.RedriveCount != 1 ||
		!summary.ExecutedRedrive ||
		summary.RecordsDecision ||
		summary.CallsProvider ||
		summary.ExecutesTarget ||
		!summary.MutatesDeliveryFact {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	for _, forbidden := range []string{path, "password", "provider_body", "raw:"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("summary leaked forbidden content %q: %s", forbidden, raw)
		}
	}
}
