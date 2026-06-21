package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	controlplanev1 "github.com/qsyy0921/IM/api/proto/nexusim/controlplane/v1"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type ControlPlaneCompensationExecutor struct {
	client       controlplanev1.ControlPlaneServiceClient
	timeout      time.Duration
	instructions map[string]ControlPlaneRollbackInstruction
}

type ControlPlaneRollbackInstruction struct {
	PayloadRefHash string `json:"payload_ref_hash"`
	Environment    string `json:"environment"`
	ConfigKind     string `json:"config_kind"`
	BundleKey      string `json:"bundle_key"`
	TargetVersion  string `json:"target_version"`
	OperatorRef    string `json:"operator_ref"`
	ReasonRef      string `json:"reason_ref"`
}

type controlPlaneInstructionFile struct {
	Instructions []ControlPlaneRollbackInstruction `json:"instructions"`
}

func NewControlPlaneCompensationExecutor(
	client controlplanev1.ControlPlaneServiceClient,
	timeout time.Duration,
	instructions []ControlPlaneRollbackInstruction,
) ControlPlaneCompensationExecutor {
	if timeout <= 0 {
		timeout = time.Second
	}
	return ControlPlaneCompensationExecutor{
		client:       client,
		timeout:      timeout,
		instructions: indexControlPlaneInstructions(instructions),
	}
}

func DialControlPlaneCompensationExecutor(
	_ context.Context,
	addr string,
	timeout time.Duration,
	instructions []ControlPlaneRollbackInstruction,
) (ControlPlaneCompensationExecutor, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ControlPlaneCompensationExecutor{}, nil, errors.New("control-plane-service address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return ControlPlaneCompensationExecutor{}, nil, err
	}
	return NewControlPlaneCompensationExecutor(controlplanev1.NewControlPlaneServiceClient(conn), timeout, instructions), conn.Close, nil
}

func LoadControlPlaneRollbackInstructions(path string) ([]ControlPlaneRollbackInstruction, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("control-plane compensation instruction file is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file controlPlaneInstructionFile
	if err := json.Unmarshal(content, &file); err != nil {
		return nil, err
	}
	for index := range file.Instructions {
		if err := file.Instructions[index].NormalizeAndValidate(); err != nil {
			return nil, err
		}
	}
	return file.Instructions, nil
}

func (instruction *ControlPlaneRollbackInstruction) NormalizeAndValidate() error {
	instruction.PayloadRefHash = strings.TrimSpace(instruction.PayloadRefHash)
	instruction.Environment = strings.TrimSpace(instruction.Environment)
	instruction.ConfigKind = strings.TrimSpace(instruction.ConfigKind)
	instruction.BundleKey = strings.TrimSpace(instruction.BundleKey)
	instruction.TargetVersion = strings.TrimSpace(instruction.TargetVersion)
	instruction.OperatorRef = strings.TrimSpace(instruction.OperatorRef)
	instruction.ReasonRef = strings.TrimSpace(instruction.ReasonRef)
	if instruction.PayloadRefHash == "" ||
		instruction.Environment == "" ||
		instruction.ConfigKind == "" ||
		instruction.BundleKey == "" ||
		instruction.TargetVersion == "" ||
		instruction.OperatorRef == "" ||
		instruction.ReasonRef == "" {
		return types.NewInvalidArgument("control-plane compensation instruction is incomplete")
	}
	return nil
}

func (executor ControlPlaneCompensationExecutor) ExecuteCompensation(
	ctx context.Context,
	compensation types.WorkflowCompensation,
) (types.WorkflowCompensationExecutionResult, error) {
	if executor.client == nil {
		return types.WorkflowCompensationExecutionResult{}, types.NewUnavailable("control-plane compensation executor is not configured")
	}
	if compensation.TargetService != "control-plane-service" || compensation.TargetOperation != "CONFIG_ROLLBACK" {
		return unsupportedCompensationResult(compensation), nil
	}
	instruction, ok := executor.instructions[compensation.PayloadRefHash]
	if !ok {
		return types.WorkflowCompensationExecutionResult{
			DownstreamService:    "control-plane-service",
			DownstreamRequestRef: "compensation:" + compensation.CompensationID,
			Status:               types.WorkflowCompensationStatusFailed,
			FailureClass:         "COMPENSATION_INSTRUCTION_NOT_FOUND",
			PublicError:          "compensation instruction not found",
		}, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, executor.timeout)
	defer cancel()
	response, err := executor.client.RollbackConfigVersion(callCtx, &controlplanev1.RollbackConfigVersionRequest{
		AuthContext: &controlplanev1.AuthContext{
			TenantId:    string(compensation.TenantID),
			ServiceName: "workflow-service",
			InstanceRef: "compensation-executor",
			TraceId:     "workflow:" + compensation.WorkflowID,
			RequestId:   compensation.CompensationID,
		},
		Environment:    instruction.Environment,
		ConfigKind:     instruction.ConfigKind,
		BundleKey:      instruction.BundleKey,
		TargetVersion:  instruction.TargetVersion,
		ApprovalRef:    "workflow:" + compensation.WorkflowID,
		OperatorRef:    instruction.OperatorRef,
		ReasonRef:      instruction.ReasonRef,
		IdempotencyKey: "workflow-compensation:" + compensation.CompensationID,
		CorrelationId:  compensation.WorkflowID,
		CausationId:    compensation.CompensationID,
		TraceId:        "workflow:" + compensation.WorkflowID,
	})
	if err != nil {
		return types.WorkflowCompensationExecutionResult{}, mapControlPlaneError(err)
	}
	version := response.GetVersion()
	if version == nil || strings.TrimSpace(version.GetVersion()) == "" {
		return types.WorkflowCompensationExecutionResult{}, types.NewUnavailable("control-plane compensation response is incomplete")
	}
	return types.WorkflowCompensationExecutionResult{
		DownstreamService: "control-plane-service",
		DownstreamRequestRef: fmt.Sprintf(
			"config-rollback:%s:%s:%s:%s",
			instruction.Environment,
			instruction.ConfigKind,
			instruction.BundleKey,
			instruction.TargetVersion,
		),
		Status: types.WorkflowCompensationStatusSucceeded,
	}, nil
}

func unsupportedCompensationResult(compensation types.WorkflowCompensation) types.WorkflowCompensationExecutionResult {
	return types.WorkflowCompensationExecutionResult{
		DownstreamService:    compensation.TargetService,
		DownstreamRequestRef: "compensation:" + compensation.CompensationID,
		Status:               types.WorkflowCompensationStatusFailed,
		FailureClass:         "UNSUPPORTED_COMPENSATION_TARGET",
		PublicError:          "compensation target is unsupported",
	}
}

func indexControlPlaneInstructions(instructions []ControlPlaneRollbackInstruction) map[string]ControlPlaneRollbackInstruction {
	indexed := make(map[string]ControlPlaneRollbackInstruction, len(instructions))
	for _, instruction := range instructions {
		indexed[instruction.PayloadRefHash] = instruction
	}
	return indexed
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
