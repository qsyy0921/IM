package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	controlplanev1 "github.com/qsyy0921/IM/api/proto/nexusim/controlplane/v1"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type ControlPlaneConfigPublishExecutor struct {
	client  controlplanev1.ControlPlaneServiceClient
	timeout time.Duration
}

type configPublishPayload struct {
	Environment       string `json:"environment"`
	ConfigKind        string `json:"config_kind"`
	BundleKey         string `json:"bundle_key"`
	Version           string `json:"version"`
	SchemaVersion     string `json:"schema_version"`
	PayloadJSON       string `json:"payload_json"`
	EffectiveAtUnixMS int64  `json:"effective_at_unix_ms"`
	ExpiresAtUnixMS   int64  `json:"expires_at_unix_ms"`
}

type configRollbackPayload struct {
	Environment   string `json:"environment"`
	ConfigKind    string `json:"config_kind"`
	BundleKey     string `json:"bundle_key"`
	TargetVersion string `json:"target_version"`
}

type tenantQuotaChangePayload struct {
	Environment       string  `json:"environment"`
	BundleKey         string  `json:"bundle_key"`
	Version           string  `json:"version"`
	ConfigVersion     string  `json:"config_version"`
	TenantRef         string  `json:"tenant_ref"`
	QuotaRPS          float64 `json:"quota_rps"`
	QuotaBurst        int64   `json:"quota_burst"`
	EffectiveAtUnixMS int64   `json:"effective_at_unix_ms"`
	ExpiresAtUnixMS   int64   `json:"expires_at_unix_ms"`
}

type policyRuleChangePayload struct {
	Environment       string `json:"environment"`
	BundleKey         string `json:"bundle_key"`
	Version           string `json:"version"`
	ConfigVersion     string `json:"config_version"`
	PolicyRuleRef     string `json:"policy_rule_ref"`
	EffectiveAtUnixMS int64  `json:"effective_at_unix_ms"`
	ExpiresAtUnixMS   int64  `json:"expires_at_unix_ms"`
}

func NewControlPlaneConfigPublishExecutor(
	client controlplanev1.ControlPlaneServiceClient,
	timeout time.Duration,
) ControlPlaneConfigPublishExecutor {
	if timeout <= 0 {
		timeout = time.Second
	}
	return ControlPlaneConfigPublishExecutor{client: client, timeout: timeout}
}

func DialControlPlaneConfigPublishExecutor(
	_ context.Context,
	addr string,
	timeout time.Duration,
) (ControlPlaneConfigPublishExecutor, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ControlPlaneConfigPublishExecutor{}, nil, errors.New("control-plane-service address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return ControlPlaneConfigPublishExecutor{}, nil, err
	}
	return NewControlPlaneConfigPublishExecutor(controlplanev1.NewControlPlaneServiceClient(conn), timeout), conn.Close, nil
}

func (executor ControlPlaneConfigPublishExecutor) Execute(
	ctx context.Context,
	operation types.AdminOperation,
) (types.OperationExecutionResult, error) {
	if executor.client == nil {
		return types.OperationExecutionResult{}, types.NewUnavailable("control-plane executor is not configured")
	}
	switch operation.OperationType {
	case "CONFIG_ROLLBACK":
		return executor.executeRollback(ctx, operation)
	case "TENANT_QUOTA_CHANGE":
		return executor.executeTenantQuotaChange(ctx, operation)
	case "POLICY_RULE_CHANGE":
		return executor.executePolicyRuleChange(ctx, operation)
	}
	payload, err := parseConfigPublishPayload(operation.PayloadJSON)
	if err != nil {
		return types.OperationExecutionResult{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, executor.timeout)
	defer cancel()
	response, err := executor.client.PublishConfigVersion(callCtx, &controlplanev1.PublishConfigVersionRequest{
		AuthContext: &controlplanev1.AuthContext{
			TenantId:    string(operation.TenantID),
			UserId:      operation.RequestedBy,
			ServiceName: "admin-service",
			InstanceRef: "operation-worker",
			TraceId:     operation.TraceID,
			RequestId:   operation.OperationID,
		},
		Environment:       payload.Environment,
		ConfigKind:        payload.ConfigKind,
		BundleKey:         payload.BundleKey,
		Version:           payload.Version,
		SchemaVersion:     payload.SchemaVersion,
		PayloadJson:       payload.PayloadJSON,
		EffectiveAtUnixMs: payload.EffectiveAtUnixMS,
		ExpiresAtUnixMs:   payload.ExpiresAtUnixMS,
		ApprovalRef:       "admin-operation:" + operation.OperationID,
		OperatorRef:       operation.RequestedBy,
		ReasonRef:         operation.ReasonRef,
		IdempotencyKey:    "admin-control-plane:" + operation.OperationID,
		CorrelationId:     operation.CorrelationID,
		CausationId:       firstNonEmpty(operation.CausationID, operation.OperationID),
		TraceId:           operation.TraceID,
	})
	if err != nil {
		return types.OperationExecutionResult{}, mapControlPlaneError(err)
	}
	version := response.GetVersion()
	if version == nil || strings.TrimSpace(version.GetVersion()) == "" {
		return types.OperationExecutionResult{}, types.NewUnavailable("control-plane response is incomplete")
	}
	return types.OperationExecutionResult{
		DownstreamService: "control-plane-service",
		DownstreamRequestRef: fmt.Sprintf(
			"config:%s:%s:%s:%s",
			payload.Environment,
			payload.ConfigKind,
			payload.BundleKey,
			payload.Version,
		),
		Status: types.OperationStatusSucceeded,
	}, nil
}

func (executor ControlPlaneConfigPublishExecutor) executeTenantQuotaChange(
	ctx context.Context,
	operation types.AdminOperation,
) (types.OperationExecutionResult, error) {
	payload, err := parseTenantQuotaChangePayload(operation.PayloadJSON)
	if err != nil {
		return types.OperationExecutionResult{}, err
	}
	payloadJSON, err := tenantQuotaPayloadJSON(payload)
	if err != nil {
		return types.OperationExecutionResult{}, err
	}
	version := firstNonEmpty(payload.Version, payload.ConfigVersion)
	callCtx, cancel := context.WithTimeout(ctx, executor.timeout)
	defer cancel()
	response, err := executor.client.PublishConfigVersion(callCtx, &controlplanev1.PublishConfigVersionRequest{
		AuthContext: &controlplanev1.AuthContext{
			TenantId:    string(operation.TenantID),
			UserId:      operation.RequestedBy,
			ServiceName: "admin-service",
			InstanceRef: "operation-worker",
			TraceId:     operation.TraceID,
			RequestId:   operation.OperationID,
		},
		Environment:       payload.Environment,
		ConfigKind:        "API_GATEWAY_TENANT_QUOTA",
		BundleKey:         payload.BundleKey,
		Version:           version,
		SchemaVersion:     "quota-v1",
		PayloadJson:       payloadJSON,
		EffectiveAtUnixMs: payload.EffectiveAtUnixMS,
		ExpiresAtUnixMs:   payload.ExpiresAtUnixMS,
		ApprovalRef:       "admin-operation:" + operation.OperationID,
		OperatorRef:       operation.RequestedBy,
		ReasonRef:         operation.ReasonRef,
		IdempotencyKey:    "admin-control-plane-tenant-quota:" + operation.OperationID,
		CorrelationId:     operation.CorrelationID,
		CausationId:       firstNonEmpty(operation.CausationID, operation.OperationID),
		TraceId:           operation.TraceID,
	})
	if err != nil {
		return types.OperationExecutionResult{}, mapControlPlaneError(err)
	}
	published := response.GetVersion()
	if published == nil || strings.TrimSpace(published.GetVersion()) == "" {
		return types.OperationExecutionResult{}, types.NewUnavailable("control-plane response is incomplete")
	}
	return types.OperationExecutionResult{
		DownstreamService: "control-plane-service",
		DownstreamRequestRef: fmt.Sprintf(
			"tenant-quota:%s:%s:%s:%s",
			payload.Environment,
			payload.BundleKey,
			payload.TenantRef,
			version,
		),
		Status: types.OperationStatusSucceeded,
	}, nil
}

func (executor ControlPlaneConfigPublishExecutor) executePolicyRuleChange(
	ctx context.Context,
	operation types.AdminOperation,
) (types.OperationExecutionResult, error) {
	payload, err := parsePolicyRuleChangePayload(operation.PayloadJSON)
	if err != nil {
		return types.OperationExecutionResult{}, err
	}
	payloadJSON, err := policyRulePayloadJSON(payload)
	if err != nil {
		return types.OperationExecutionResult{}, err
	}
	version := firstNonEmpty(payload.Version, payload.ConfigVersion)
	callCtx, cancel := context.WithTimeout(ctx, executor.timeout)
	defer cancel()
	response, err := executor.client.PublishConfigVersion(callCtx, &controlplanev1.PublishConfigVersionRequest{
		AuthContext: &controlplanev1.AuthContext{
			TenantId:    string(operation.TenantID),
			UserId:      operation.RequestedBy,
			ServiceName: "admin-service",
			InstanceRef: "operation-worker",
			TraceId:     operation.TraceID,
			RequestId:   operation.OperationID,
		},
		Environment:       payload.Environment,
		ConfigKind:        "POLICY_RULESET_REF",
		BundleKey:         payload.BundleKey,
		Version:           version,
		SchemaVersion:     "policy-ruleset-ref-v1",
		PayloadJson:       payloadJSON,
		EffectiveAtUnixMs: payload.EffectiveAtUnixMS,
		ExpiresAtUnixMs:   payload.ExpiresAtUnixMS,
		ApprovalRef:       "admin-operation:" + operation.OperationID,
		OperatorRef:       operation.RequestedBy,
		ReasonRef:         operation.ReasonRef,
		IdempotencyKey:    "admin-control-plane-policy-rule:" + operation.OperationID,
		CorrelationId:     operation.CorrelationID,
		CausationId:       firstNonEmpty(operation.CausationID, operation.OperationID),
		TraceId:           operation.TraceID,
	})
	if err != nil {
		return types.OperationExecutionResult{}, mapControlPlaneError(err)
	}
	published := response.GetVersion()
	if published == nil || strings.TrimSpace(published.GetVersion()) == "" {
		return types.OperationExecutionResult{}, types.NewUnavailable("control-plane response is incomplete")
	}
	return types.OperationExecutionResult{
		DownstreamService: "control-plane-service",
		DownstreamRequestRef: fmt.Sprintf(
			"policy-ruleset-ref:%s:%s:%s:%s",
			payload.Environment,
			payload.BundleKey,
			payload.PolicyRuleRef,
			version,
		),
		Status: types.OperationStatusSucceeded,
	}, nil
}

func (executor ControlPlaneConfigPublishExecutor) executeRollback(
	ctx context.Context,
	operation types.AdminOperation,
) (types.OperationExecutionResult, error) {
	payload, err := parseConfigRollbackPayload(operation.PayloadJSON)
	if err != nil {
		return types.OperationExecutionResult{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, executor.timeout)
	defer cancel()
	response, err := executor.client.RollbackConfigVersion(callCtx, &controlplanev1.RollbackConfigVersionRequest{
		AuthContext: &controlplanev1.AuthContext{
			TenantId:    string(operation.TenantID),
			UserId:      operation.RequestedBy,
			ServiceName: "admin-service",
			InstanceRef: "operation-worker",
			TraceId:     operation.TraceID,
			RequestId:   operation.OperationID,
		},
		Environment:    payload.Environment,
		ConfigKind:     payload.ConfigKind,
		BundleKey:      payload.BundleKey,
		TargetVersion:  payload.TargetVersion,
		ApprovalRef:    "admin-operation:" + operation.OperationID,
		OperatorRef:    operation.RequestedBy,
		ReasonRef:      operation.ReasonRef,
		IdempotencyKey: "admin-control-plane-rollback:" + operation.OperationID,
		CorrelationId:  operation.CorrelationID,
		CausationId:    firstNonEmpty(operation.CausationID, operation.OperationID),
		TraceId:        operation.TraceID,
	})
	if err != nil {
		return types.OperationExecutionResult{}, mapControlPlaneError(err)
	}
	version := response.GetVersion()
	if version == nil || strings.TrimSpace(version.GetVersion()) == "" {
		return types.OperationExecutionResult{}, types.NewUnavailable("control-plane response is incomplete")
	}
	return types.OperationExecutionResult{
		DownstreamService: "control-plane-service",
		DownstreamRequestRef: fmt.Sprintf(
			"config-rollback:%s:%s:%s:%s",
			payload.Environment,
			payload.ConfigKind,
			payload.BundleKey,
			payload.TargetVersion,
		),
		Status: types.OperationStatusSucceeded,
	}, nil
}

func parsePolicyRuleChangePayload(raw string) (policyRuleChangePayload, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return policyRuleChangePayload{}, types.NewInvalidArgument("policy rule payload is required")
	}
	var payload policyRuleChangePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return policyRuleChangePayload{}, types.NewInvalidArgument("policy rule payload is malformed")
	}
	payload.Environment = strings.TrimSpace(payload.Environment)
	payload.BundleKey = strings.TrimSpace(payload.BundleKey)
	payload.Version = strings.TrimSpace(payload.Version)
	payload.ConfigVersion = strings.TrimSpace(payload.ConfigVersion)
	payload.PolicyRuleRef = strings.TrimSpace(payload.PolicyRuleRef)
	version := firstNonEmpty(payload.Version, payload.ConfigVersion)
	if payload.Environment == "" ||
		payload.BundleKey == "" ||
		version == "" ||
		payload.PolicyRuleRef == "" {
		return policyRuleChangePayload{}, types.NewInvalidArgument("policy rule payload is incomplete")
	}
	if payload.EffectiveAtUnixMS <= 0 {
		return policyRuleChangePayload{}, types.NewInvalidArgument("policy rule effective_at is required")
	}
	if payload.ExpiresAtUnixMS > 0 && payload.ExpiresAtUnixMS <= payload.EffectiveAtUnixMS {
		return policyRuleChangePayload{}, types.NewInvalidArgument("policy rule expires_at must be after effective_at")
	}
	return payload, nil
}

func policyRulePayloadJSON(payload policyRuleChangePayload) (string, error) {
	encoded, err := json.Marshal(map[string]any{
		"policy_ruleset_ref": payload.PolicyRuleRef,
	})
	if err != nil {
		return "", types.NewInvalidArgument("policy rule payload is invalid")
	}
	return string(encoded), nil
}

func parseConfigPublishPayload(raw string) (configPublishPayload, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return configPublishPayload{}, types.NewInvalidArgument("config publish payload is required")
	}
	var payload configPublishPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return configPublishPayload{}, types.NewInvalidArgument("config publish payload is malformed")
	}
	payload.Environment = strings.TrimSpace(payload.Environment)
	payload.ConfigKind = strings.TrimSpace(payload.ConfigKind)
	payload.BundleKey = strings.TrimSpace(payload.BundleKey)
	payload.Version = strings.TrimSpace(payload.Version)
	payload.SchemaVersion = strings.TrimSpace(payload.SchemaVersion)
	payload.PayloadJSON = strings.TrimSpace(payload.PayloadJSON)
	if payload.Environment == "" ||
		payload.ConfigKind == "" ||
		payload.BundleKey == "" ||
		payload.Version == "" ||
		payload.SchemaVersion == "" ||
		payload.PayloadJSON == "" {
		return configPublishPayload{}, types.NewInvalidArgument("config publish payload is incomplete")
	}
	if !json.Valid([]byte(payload.PayloadJSON)) {
		return configPublishPayload{}, types.NewInvalidArgument("config publish payload_json is malformed")
	}
	return payload, nil
}

func parseTenantQuotaChangePayload(raw string) (tenantQuotaChangePayload, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tenantQuotaChangePayload{}, types.NewInvalidArgument("tenant quota payload is required")
	}
	var payload tenantQuotaChangePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return tenantQuotaChangePayload{}, types.NewInvalidArgument("tenant quota payload is malformed")
	}
	payload.Environment = strings.TrimSpace(payload.Environment)
	payload.BundleKey = strings.TrimSpace(payload.BundleKey)
	payload.Version = strings.TrimSpace(payload.Version)
	payload.ConfigVersion = strings.TrimSpace(payload.ConfigVersion)
	payload.TenantRef = strings.TrimSpace(payload.TenantRef)
	version := firstNonEmpty(payload.Version, payload.ConfigVersion)
	if payload.Environment == "" ||
		payload.BundleKey == "" ||
		version == "" ||
		payload.TenantRef == "" {
		return tenantQuotaChangePayload{}, types.NewInvalidArgument("tenant quota payload is incomplete")
	}
	if payload.QuotaRPS <= 0 || payload.QuotaBurst <= 0 {
		return tenantQuotaChangePayload{}, types.NewInvalidArgument("tenant quota values must be positive")
	}
	if payload.EffectiveAtUnixMS <= 0 {
		return tenantQuotaChangePayload{}, types.NewInvalidArgument("tenant quota effective_at is required")
	}
	if payload.ExpiresAtUnixMS > 0 && payload.ExpiresAtUnixMS <= payload.EffectiveAtUnixMS {
		return tenantQuotaChangePayload{}, types.NewInvalidArgument("tenant quota expires_at must be after effective_at")
	}
	return payload, nil
}

func tenantQuotaPayloadJSON(payload tenantQuotaChangePayload) (string, error) {
	encoded, err := json.Marshal(map[string]any{
		"plans": map[string]any{
			payload.TenantRef: map[string]any{
				"requests_per_second": payload.QuotaRPS,
				"burst":               payload.QuotaBurst,
			},
		},
	})
	if err != nil {
		return "", types.NewInvalidArgument("tenant quota payload is invalid")
	}
	return string(encoded), nil
}

func parseConfigRollbackPayload(raw string) (configRollbackPayload, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return configRollbackPayload{}, types.NewInvalidArgument("config rollback payload is required")
	}
	var payload configRollbackPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return configRollbackPayload{}, types.NewInvalidArgument("config rollback payload is malformed")
	}
	payload.Environment = strings.TrimSpace(payload.Environment)
	payload.ConfigKind = strings.TrimSpace(payload.ConfigKind)
	payload.BundleKey = strings.TrimSpace(payload.BundleKey)
	payload.TargetVersion = strings.TrimSpace(payload.TargetVersion)
	if payload.Environment == "" ||
		payload.ConfigKind == "" ||
		payload.BundleKey == "" ||
		payload.TargetVersion == "" {
		return configRollbackPayload{}, types.NewInvalidArgument("config rollback payload is incomplete")
	}
	return payload, nil
}

func mapControlPlaneError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return types.NewUnavailable("control-plane temporarily unavailable")
	}
	st, ok := status.FromError(err)
	if !ok {
		return types.NewUnavailable("control-plane temporarily unavailable")
	}
	switch st.Code() {
	case codes.InvalidArgument:
		return types.NewInvalidArgument("control-plane request invalid")
	case codes.PermissionDenied:
		return types.NewPermissionDenied("control-plane permission denied")
	case codes.FailedPrecondition:
		return types.NewFailedPrecondition("control-plane precondition failed")
	case codes.AlreadyExists:
		return types.NewAlreadyExists("control-plane already exists")
	case codes.NotFound:
		return types.NewNotFound("control-plane target not found")
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.NewUnavailable("control-plane temporarily unavailable")
	default:
		return types.NewUnavailable("control-plane temporarily unavailable")
	}
}
