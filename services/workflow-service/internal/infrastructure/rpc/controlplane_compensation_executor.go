package rpc

import (
	"context"
	"crypto/sha256"
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
	client   controlplanev1.ControlPlaneServiceClient
	timeout  time.Duration
	resolver ControlPlaneRollbackInstructionResolver
}

type ControlPlaneRollbackInstructionResolver interface {
	ResolveControlPlaneRollbackInstruction(
		context.Context,
		types.WorkflowCompensation,
	) (types.WorkflowCompensationInstruction, bool, error)
}

type ControlPlaneRollbackInstruction struct {
	InstructionID  string `json:"instruction_id"`
	WorkflowID     string `json:"workflow_id"`
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
		client:   client,
		timeout:  timeout,
		resolver: newStaticControlPlaneRollbackInstructionResolver(instructions),
	}
}

func NewControlPlaneCompensationExecutorWithResolver(
	client controlplanev1.ControlPlaneServiceClient,
	timeout time.Duration,
	resolver ControlPlaneRollbackInstructionResolver,
) ControlPlaneCompensationExecutor {
	if timeout <= 0 {
		timeout = time.Second
	}
	return ControlPlaneCompensationExecutor{
		client:   client,
		timeout:  timeout,
		resolver: resolver,
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

func DialControlPlaneCompensationExecutorWithResolver(
	_ context.Context,
	addr string,
	timeout time.Duration,
	resolver ControlPlaneRollbackInstructionResolver,
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
	return NewControlPlaneCompensationExecutorWithResolver(controlplanev1.NewControlPlaneServiceClient(conn), timeout, resolver), conn.Close, nil
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
	instruction.InstructionID = strings.TrimSpace(instruction.InstructionID)
	instruction.WorkflowID = strings.TrimSpace(instruction.WorkflowID)
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

func ControlPlaneRollbackInstructionsForTenant(
	tenantID types.TenantID,
	instructions []ControlPlaneRollbackInstruction,
) ([]types.WorkflowCompensationInstruction, error) {
	result := make([]types.WorkflowCompensationInstruction, 0, len(instructions))
	for index := range instructions {
		if err := instructions[index].NormalizeAndValidate(); err != nil {
			return nil, err
		}
		instructionID := instructions[index].InstructionID
		if instructionID == "" {
			instructionID = deterministicControlPlaneInstructionID(instructions[index].PayloadRefHash)
		}
		result = append(result, types.WorkflowCompensationInstruction{
			TenantID:        tenantID,
			InstructionID:   instructionID,
			WorkflowID:      instructions[index].WorkflowID,
			PayloadRefHash:  instructions[index].PayloadRefHash,
			TargetService:   "control-plane-service",
			TargetOperation: "CONFIG_ROLLBACK",
			InstructionType: types.WorkflowCompensationInstructionTypeControlPlaneRollback,
			Environment:     instructions[index].Environment,
			ConfigKind:      instructions[index].ConfigKind,
			BundleKey:       instructions[index].BundleKey,
			TargetVersion:   instructions[index].TargetVersion,
			OperatorRef:     instructions[index].OperatorRef,
			ReasonRef:       instructions[index].ReasonRef,
			Status:          types.WorkflowCompensationInstructionStatusActive,
		})
	}
	return result, nil
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
	if executor.resolver == nil {
		return types.WorkflowCompensationExecutionResult{}, types.NewUnavailable("control-plane compensation instruction resolver is not configured")
	}
	instruction, ok, err := executor.resolver.ResolveControlPlaneRollbackInstruction(ctx, compensation)
	if err != nil {
		return types.WorkflowCompensationExecutionResult{}, err
	}
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

type staticControlPlaneRollbackInstructionResolver struct {
	instructions map[string]types.WorkflowCompensationInstruction
}

func newStaticControlPlaneRollbackInstructionResolver(
	instructions []ControlPlaneRollbackInstruction,
) staticControlPlaneRollbackInstructionResolver {
	workflowInstructions, _ := ControlPlaneRollbackInstructionsForTenant("", instructions)
	indexed := make(map[string]types.WorkflowCompensationInstruction, len(workflowInstructions))
	for _, instruction := range workflowInstructions {
		indexed[instruction.PayloadRefHash] = instruction
	}
	return staticControlPlaneRollbackInstructionResolver{instructions: indexed}
}

func (resolver staticControlPlaneRollbackInstructionResolver) ResolveControlPlaneRollbackInstruction(
	_ context.Context,
	compensation types.WorkflowCompensation,
) (types.WorkflowCompensationInstruction, bool, error) {
	instruction, ok := resolver.instructions[compensation.PayloadRefHash]
	return instruction, ok, nil
}

func deterministicControlPlaneInstructionID(payloadRefHash string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(payloadRefHash)))
	return "wfci_" + fmt.Sprintf("%x", sum[:8])
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
