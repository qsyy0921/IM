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
