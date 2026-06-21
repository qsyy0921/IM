package rpc

import (
	"context"
	"errors"
	"testing"
	"time"

	controlplanev1 "github.com/qsyy0921/IM/api/proto/nexusim/controlplane/v1"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestControlPlaneConfigPublishExecutorPublishesConfigVersion(t *testing.T) {
	client := &fakeControlPlaneClient{
		response: &controlplanev1.PublishConfigVersionResponse{
			Version: &controlplanev1.ConfigVersion{
				Version: "v20260621",
			},
		},
	}
	executor := NewControlPlaneConfigPublishExecutor(client, time.Second)

	result, err := executor.Execute(context.Background(), types.AdminOperation{
		TenantID:             "tenant-admin",
		OperationID:          "admop_config_publish",
		OperationType:        "CONFIG_PUBLISH",
		PayloadSchemaVersion: "admin.config_publish.v1",
		PayloadJSON: `{
			"environment":"local",
			"config_kind":"quota",
			"bundle_key":"api-gateway.default",
			"version":"v20260621",
			"schema_version":"control-plane.quota.v1",
			"payload_json":"{\"limit\":100}",
			"effective_at_unix_ms":1000,
			"expires_at_unix_ms":2000
		}`,
		ReasonRef:     "reason:change-ticket-1",
		RequestedBy:   "admin:operator-1",
		CorrelationID: "corr-1",
		CausationID:   "cause-1",
		TraceID:       "trace-1",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.DownstreamService != "control-plane-service" ||
		result.DownstreamRequestRef != "config:local:quota:api-gateway.default:v20260621" ||
		result.Status != types.OperationStatusSucceeded {
		t.Fatalf("unexpected result: %+v", result)
	}
	request := client.request
	if request.GetAuthContext().GetTenantId() != "tenant-admin" ||
		request.GetAuthContext().GetServiceName() != "admin-service" ||
		request.GetAuthContext().GetInstanceRef() != "operation-worker" ||
		request.GetAuthContext().GetRequestId() != "admop_config_publish" {
		t.Fatalf("unexpected auth context: %+v", request.GetAuthContext())
	}
	if request.GetEnvironment() != "local" ||
		request.GetConfigKind() != "quota" ||
		request.GetBundleKey() != "api-gateway.default" ||
		request.GetVersion() != "v20260621" ||
		request.GetSchemaVersion() != "control-plane.quota.v1" ||
		request.GetPayloadJson() != `{"limit":100}` {
		t.Fatalf("unexpected publish request: %+v", request)
	}
	if request.GetIdempotencyKey() != "admin-control-plane:admop_config_publish" ||
		request.GetApprovalRef() != "admin-operation:admop_config_publish" ||
		request.GetReasonRef() != "reason:change-ticket-1" {
		t.Fatalf("unexpected refs: %+v", request)
	}
}

func TestControlPlaneConfigPublishExecutorRejectsMalformedPayload(t *testing.T) {
	client := &fakeControlPlaneClient{}
	executor := NewControlPlaneConfigPublishExecutor(client, time.Second)

	_, err := executor.Execute(context.Background(), types.AdminOperation{
		OperationID: "admop_config_bad",
		PayloadJSON: `{"environment":"local","payload_json":"not-json"}`,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if client.request != nil {
		t.Fatalf("malformed payload should not call control-plane")
	}
}

func TestControlPlaneConfigPublishExecutorRollsBackConfigVersion(t *testing.T) {
	client := &fakeControlPlaneClient{
		rollbackResponse: &controlplanev1.RollbackConfigVersionResponse{
			Version: &controlplanev1.ConfigVersion{Version: "v20260620"},
		},
	}
	executor := NewControlPlaneConfigPublishExecutor(client, time.Second)

	result, err := executor.Execute(context.Background(), types.AdminOperation{
		TenantID:      "tenant-admin",
		OperationID:   "admop_config_rollback",
		OperationType: "CONFIG_ROLLBACK",
		PayloadJSON: `{
			"environment":"local",
			"config_kind":"quota",
			"bundle_key":"api-gateway.default",
			"target_version":"v20260620"
		}`,
		ReasonRef:     "reason:rollback-ticket-1",
		RequestedBy:   "admin:operator-1",
		CorrelationID: "corr-rollback",
		CausationID:   "cause-rollback",
		TraceID:       "trace-rollback",
	})
	if err != nil {
		t.Fatalf("execute rollback: %v", err)
	}
	if result.DownstreamService != "control-plane-service" ||
		result.DownstreamRequestRef != "config-rollback:local:quota:api-gateway.default:v20260620" ||
		result.Status != types.OperationStatusSucceeded {
		t.Fatalf("unexpected result: %+v", result)
	}
	request := client.rollbackRequest
	if request.GetTargetVersion() != "v20260620" ||
		request.GetIdempotencyKey() != "admin-control-plane-rollback:admop_config_rollback" ||
		request.GetApprovalRef() != "admin-operation:admop_config_rollback" ||
		request.GetReasonRef() != "reason:rollback-ticket-1" {
		t.Fatalf("unexpected rollback request: %+v", request)
	}
	if client.request != nil {
		t.Fatal("rollback operation should not call PublishConfigVersion")
	}
}

func TestControlPlaneConfigPublishExecutorChangesTenantQuota(t *testing.T) {
	client := &fakeControlPlaneClient{
		response: &controlplanev1.PublishConfigVersionResponse{
			Version: &controlplanev1.ConfigVersion{Version: "quota-v1.tenant-a"},
		},
	}
	executor := NewControlPlaneConfigPublishExecutor(client, time.Second)

	result, err := executor.Execute(context.Background(), types.AdminOperation{
		TenantID:      "tenant-admin",
		OperationID:   "admop_tenant_quota",
		OperationType: "TENANT_QUOTA_CHANGE",
		PayloadJSON: `{
			"environment":"local",
			"bundle_key":"api-gateway/default",
			"config_version":"quota-v1.tenant-a",
			"tenant_ref":"tenant-a",
			"quota_rps":42,
			"quota_burst":84,
			"effective_at_unix_ms":1000
		}`,
		ReasonRef:     "reason:quota-ticket-1",
		RequestedBy:   "admin:operator-1",
		CorrelationID: "corr-quota",
		CausationID:   "cause-quota",
		TraceID:       "trace-quota",
	})
	if err != nil {
		t.Fatalf("execute tenant quota: %v", err)
	}
	if result.DownstreamService != "control-plane-service" ||
		result.DownstreamRequestRef != "tenant-quota:local:api-gateway/default:tenant-a:quota-v1.tenant-a" ||
		result.Status != types.OperationStatusSucceeded {
		t.Fatalf("unexpected result: %+v", result)
	}
	request := client.request
	if request.GetConfigKind() != "API_GATEWAY_TENANT_QUOTA" ||
		request.GetBundleKey() != "api-gateway/default" ||
		request.GetVersion() != "quota-v1.tenant-a" ||
		request.GetSchemaVersion() != "quota-v1" ||
		request.GetPayloadJson() != `{"plans":{"tenant-a":{"burst":84,"requests_per_second":42}}}` ||
		request.GetEffectiveAtUnixMs() != 1000 ||
		request.GetIdempotencyKey() != "admin-control-plane-tenant-quota:admop_tenant_quota" ||
		request.GetApprovalRef() != "admin-operation:admop_tenant_quota" {
		t.Fatalf("unexpected tenant quota publish request: %+v", request)
	}
	if client.rollbackRequest != nil {
		t.Fatal("tenant quota operation should not call RollbackConfigVersion")
	}
}

func TestControlPlaneConfigPublishExecutorPublishesPolicyRulesetRef(t *testing.T) {
	client := &fakeControlPlaneClient{
		response: &controlplanev1.PublishConfigVersionResponse{
			Version: &controlplanev1.ConfigVersion{Version: "policy-v1"},
		},
	}
	executor := NewControlPlaneConfigPublishExecutor(client, time.Second)

	result, err := executor.Execute(context.Background(), types.AdminOperation{
		TenantID:      "tenant-admin",
		OperationID:   "admop_policy_rule",
		OperationType: "POLICY_RULE_CHANGE",
		PayloadJSON: `{
			"environment":"local",
			"bundle_key":"policy/default",
			"config_version":"policy-v1",
			"policy_rule_ref":"policy-ruleset-v1",
			"effective_at_unix_ms":1000
		}`,
		ReasonRef:     "reason:policy-ticket-1",
		RequestedBy:   "admin:operator-1",
		CorrelationID: "corr-policy",
		CausationID:   "cause-policy",
		TraceID:       "trace-policy",
	})
	if err != nil {
		t.Fatalf("execute policy rule change: %v", err)
	}
	if result.DownstreamService != "control-plane-service" ||
		result.DownstreamRequestRef != "policy-ruleset-ref:local:policy/default:policy-ruleset-v1:policy-v1" ||
		result.Status != types.OperationStatusSucceeded {
		t.Fatalf("unexpected result: %+v", result)
	}
	request := client.request
	if request.GetConfigKind() != "POLICY_RULESET_REF" ||
		request.GetBundleKey() != "policy/default" ||
		request.GetVersion() != "policy-v1" ||
		request.GetSchemaVersion() != "policy-ruleset-ref-v1" ||
		request.GetPayloadJson() != `{"policy_ruleset_ref":"policy-ruleset-v1"}` ||
		request.GetEffectiveAtUnixMs() != 1000 ||
		request.GetIdempotencyKey() != "admin-control-plane-policy-rule:admop_policy_rule" ||
		request.GetApprovalRef() != "admin-operation:admop_policy_rule" {
		t.Fatalf("unexpected policy rule publish request: %+v", request)
	}
	if client.rollbackRequest != nil {
		t.Fatal("policy rule operation should not call RollbackConfigVersion")
	}
}

func TestControlPlaneConfigPublishExecutorRejectsMalformedTenantQuotaPayload(t *testing.T) {
	client := &fakeControlPlaneClient{}
	executor := NewControlPlaneConfigPublishExecutor(client, time.Second)

	_, err := executor.Execute(context.Background(), types.AdminOperation{
		OperationID:   "admop_tenant_quota_bad",
		OperationType: "TENANT_QUOTA_CHANGE",
		PayloadJSON:   `{"environment":"local","bundle_key":"api-gateway/default","tenant_ref":"tenant-a","quota_rps":0,"quota_burst":84,"config_version":"quota-v1"}`,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if client.request != nil {
		t.Fatalf("malformed tenant quota payload should not call control-plane")
	}
}

func TestControlPlaneConfigPublishExecutorRejectsMalformedPolicyRulePayload(t *testing.T) {
	client := &fakeControlPlaneClient{}
	executor := NewControlPlaneConfigPublishExecutor(client, time.Second)

	_, err := executor.Execute(context.Background(), types.AdminOperation{
		OperationID:   "admop_policy_rule_bad",
		OperationType: "POLICY_RULE_CHANGE",
		PayloadJSON:   `{"environment":"local","bundle_key":"policy/default","config_version":"policy-v1"}`,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if client.request != nil {
		t.Fatalf("malformed policy rule payload should not call control-plane")
	}
}

func TestControlPlaneConfigPublishExecutorRejectsTenantQuotaWithoutEffectiveAt(t *testing.T) {
	client := &fakeControlPlaneClient{}
	executor := NewControlPlaneConfigPublishExecutor(client, time.Second)

	_, err := executor.Execute(context.Background(), types.AdminOperation{
		OperationID:   "admop_tenant_quota_missing_effective_at",
		OperationType: "TENANT_QUOTA_CHANGE",
		PayloadJSON:   `{"environment":"local","bundle_key":"api-gateway/default","tenant_ref":"tenant-a","quota_rps":42,"quota_burst":84,"config_version":"quota-v1"}`,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if client.request != nil {
		t.Fatalf("tenant quota payload without effective_at should not call control-plane")
	}
}

func TestControlPlaneConfigPublishExecutorRejectsMalformedRollbackPayload(t *testing.T) {
	client := &fakeControlPlaneClient{}
	executor := NewControlPlaneConfigPublishExecutor(client, time.Second)

	_, err := executor.Execute(context.Background(), types.AdminOperation{
		OperationID:   "admop_config_rollback_bad",
		OperationType: "CONFIG_ROLLBACK",
		PayloadJSON:   `{"environment":"local"}`,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if client.rollbackRequest != nil {
		t.Fatalf("malformed payload should not call control-plane")
	}
}

func TestControlPlaneConfigPublishExecutorMapsPublicErrors(t *testing.T) {
	executor := NewControlPlaneConfigPublishExecutor(&fakeControlPlaneClient{
		err: status.Error(codes.PermissionDenied, "raw policy body"),
	}, time.Second)

	_, err := executor.Execute(context.Background(), types.AdminOperation{
		OperationID: "admop_config_denied",
		PayloadJSON: `{
			"environment":"local",
			"config_kind":"quota",
			"bundle_key":"api-gateway.default",
			"version":"v1",
			"schema_version":"control-plane.quota.v1",
			"payload_json":"{}"
		}`,
	})
	if !errors.Is(err, types.ErrPermissionDenied) || err.Error() != "control-plane permission denied" {
		t.Fatalf("unexpected error: %v", err)
	}
}

type fakeControlPlaneClient struct {
	request          *controlplanev1.PublishConfigVersionRequest
	response         *controlplanev1.PublishConfigVersionResponse
	rollbackRequest  *controlplanev1.RollbackConfigVersionRequest
	rollbackResponse *controlplanev1.RollbackConfigVersionResponse
	err              error
}

func (client *fakeControlPlaneClient) PublishConfigVersion(
	_ context.Context,
	request *controlplanev1.PublishConfigVersionRequest,
	_ ...grpc.CallOption,
) (*controlplanev1.PublishConfigVersionResponse, error) {
	client.request = request
	if client.err != nil {
		return nil, client.err
	}
	return client.response, nil
}

func (client *fakeControlPlaneClient) RollbackConfigVersion(
	_ context.Context,
	request *controlplanev1.RollbackConfigVersionRequest,
	_ ...grpc.CallOption,
) (*controlplanev1.RollbackConfigVersionResponse, error) {
	client.rollbackRequest = request
	if client.err != nil {
		return nil, client.err
	}
	return client.rollbackResponse, nil
}

func (client *fakeControlPlaneClient) GetConfigSnapshot(
	context.Context,
	*controlplanev1.GetConfigSnapshotRequest,
	...grpc.CallOption,
) (*controlplanev1.GetConfigSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeControlPlaneClient) AckAppliedConfigVersion(
	context.Context,
	*controlplanev1.AckAppliedConfigVersionRequest,
	...grpc.CallOption,
) (*controlplanev1.AckAppliedConfigVersionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
