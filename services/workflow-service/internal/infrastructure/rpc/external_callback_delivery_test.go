package rpc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

func TestLoadExternalCallbackRedrivePlanAcceptsLowSensitivePlan(t *testing.T) {
	path := writeExternalCallbackRedrivePlanFixture(t, map[string]any{})
	plan, err := LoadExternalCallbackRedrivePlan(path, types.TenantID("tenant-workflow"))
	if err != nil {
		t.Fatalf("load redrive plan: %v", err)
	}
	if plan.TenantID != "tenant-workflow" ||
		plan.WorkflowID != "wf_callback_redrive" ||
		plan.SourceDeliveryPlanSha256 != "sha256:delivery-plan" ||
		plan.DeliveryStatus != types.WorkflowExternalCallbackDeliveryStatusDLQ ||
		plan.RedriveReasonRef != "reason-sha256:callback-redrive" {
		t.Fatalf("unexpected redrive plan: %+v", plan)
	}
}

func TestLoadExternalCallbackRedrivePlanRejectsUnsafeBoundary(t *testing.T) {
	path := writeExternalCallbackRedrivePlanFixture(t, map[string]any{
		"redrive_contract": map[string]any{
			"owner":                         "workflow-service.external-callback-delivery",
			"redrive_queue_ref":             "queue:workflow-callback-redrive",
			"redrive_reason_ref":            "reason-sha256:callback-redrive",
			"operator_review_ref":           "review:callback-redrive",
			"redrive_plan_calls_provider":   true,
			"redrive_plan_records_decision": false,
			"redrive_plan_executes_target":  false,
		},
	})
	if _, err := LoadExternalCallbackRedrivePlan(path, types.TenantID("tenant-workflow")); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected unsafe contract to fail, got %v", err)
	}
}

func TestLoadExternalCallbackRedrivePlanRejectsSourceDeliveryPlanHashMismatch(t *testing.T) {
	path := writeExternalCallbackRedrivePlanFixture(t, map[string]any{
		"redrive_source": map[string]any{
			"delivery_status":             "DLQ",
			"attempt_number":              3,
			"max_attempts":                3,
			"source_delivery_plan_sha256": "sha256:different-delivery-plan",
			"delivery_attempt_ref":        "attempt:callback-3",
			"failure_class_ref":           "failure:retry-exhausted",
			"redrive_policy_ref":          "workflow.external-callback-redrive.v1",
		},
	})
	if _, err := LoadExternalCallbackRedrivePlan(path, types.TenantID("tenant-workflow")); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected source delivery plan mismatch to fail, got %v", err)
	}
}

func writeExternalCallbackRedrivePlanFixture(t *testing.T, overrides map[string]any) string {
	t.Helper()
	plan := map[string]any{
		"schema_version":                "nexusim.workflow.external_callback_redrive_plan.v1",
		"redrive_plan_id":               "workflow-external-callback-redrive-plan-1",
		"source_delivery_status_sha256": "sha256:delivery-status",
		"source_delivery_plan_sha256":   "sha256:delivery-plan",
		"workflow_binding": map[string]any{
			"workflow_id":                     "wf_callback_redrive",
			"step_id":                         "wfs_callback_redrive",
			"expected_workflow_type":          "REPAIR_APPROVAL",
			"expected_status":                 "WAITING_DECISION",
			"expected_target_service":         "action-executor",
			"expected_target_operation":       "PROVIDER_REPLAY_REQUEST",
			"expected_target_ref_hash":        "sha256:target",
			"expected_payload_schema_version": "admin.provider_replay_request.v1",
			"expected_payload_ref_hash":       "sha256:payload",
			"expected_approval_policy_ref":    "admin.workflow.provider_replay.v1",
			"decision_policy_ref":             "workflow.external-decision.v1",
		},
		"redrive_source": map[string]any{
			"delivery_status":             "DLQ",
			"attempt_number":              3,
			"max_attempts":                3,
			"source_delivery_plan_sha256": "sha256:delivery-plan",
			"delivery_attempt_ref":        "attempt:callback-3",
			"failure_class_ref":           "failure:retry-exhausted",
			"redrive_policy_ref":          "workflow.external-callback-redrive.v1",
		},
		"redrive_contract": map[string]any{
			"owner":                         "workflow-service.external-callback-delivery",
			"redrive_queue_ref":             "queue:workflow-callback-redrive",
			"redrive_reason_ref":            "reason-sha256:callback-redrive",
			"operator_review_ref":           "review:callback-redrive",
			"redrive_plan_calls_provider":   false,
			"redrive_plan_records_decision": false,
			"redrive_plan_executes_target":  false,
		},
		"no_direct_execution":     true,
		"no_decision_recorded":    true,
		"does_not_call_provider":  true,
		"does_not_execute_target": true,
	}
	mergeFixtureOverrides(plan, overrides)
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal redrive plan fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "workflow-external-callback-redrive-plan.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write redrive plan fixture: %v", err)
	}
	return path
}

func mergeFixtureOverrides(target map[string]any, overrides map[string]any) {
	for key, value := range overrides {
		if nestedTarget, ok := target[key].(map[string]any); ok {
			if nestedOverride, ok := value.(map[string]any); ok {
				mergeFixtureOverrides(nestedTarget, nestedOverride)
				continue
			}
		}
		target[key] = value
	}
}
