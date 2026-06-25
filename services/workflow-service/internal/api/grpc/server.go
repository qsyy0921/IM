package grpc

import (
	"context"
	"errors"
	"time"

	workflowv1 "github.com/qsyy0921/IM/api/proto/nexusim/workflow/v1"
	"github.com/qsyy0921/IM/services/workflow-service/internal/app"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CreateWorkflowExecutor interface {
	Execute(context.Context, types.CreateWorkflowCommand) (app.CreateWorkflowResult, error)
}

type RecordWorkflowDecisionExecutor interface {
	Execute(context.Context, types.RecordWorkflowDecisionCommand) (app.RecordWorkflowDecisionResult, error)
}

type GetWorkflowExecutor interface {
	Execute(context.Context, types.GetWorkflowCommand) (types.Workflow, []types.WorkflowDecision, error)
}

type ListWorkflowsExecutor interface {
	Execute(context.Context, types.ListWorkflowsCommand) ([]types.Workflow, error)
}

type ListWorkflowCompensationsExecutor interface {
	Execute(context.Context, types.ListWorkflowCompensationsCommand) ([]types.WorkflowCompensation, error)
}

type ListWorkflowCompensationInstructionsExecutor interface {
	Execute(context.Context, types.ListWorkflowCompensationInstructionsCommand) ([]types.WorkflowCompensationInstruction, error)
}

type Server struct {
	workflowv1.UnimplementedWorkflowServiceServer
	createWorkflow    CreateWorkflowExecutor
	recordDecision    RecordWorkflowDecisionExecutor
	getWorkflow       GetWorkflowExecutor
	listWorkflows     ListWorkflowsExecutor
	listCompensations ListWorkflowCompensationsExecutor
	listInstructions  ListWorkflowCompensationInstructionsExecutor
}

func NewServer(
	createWorkflow CreateWorkflowExecutor,
	recordDecision RecordWorkflowDecisionExecutor,
	getWorkflow GetWorkflowExecutor,
	listWorkflows ListWorkflowsExecutor,
	listCompensations ListWorkflowCompensationsExecutor,
	listInstructions ListWorkflowCompensationInstructionsExecutor,
) *Server {
	return &Server{
		createWorkflow:    createWorkflow,
		recordDecision:    recordDecision,
		getWorkflow:       getWorkflow,
		listWorkflows:     listWorkflows,
		listCompensations: listCompensations,
		listInstructions:  listInstructions,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	workflowv1.RegisterWorkflowServiceServer(registrar, server)
}

func (server *Server) CreateWorkflow(
	ctx context.Context,
	request *workflowv1.CreateWorkflowRequest,
) (*workflowv1.CreateWorkflowResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.createWorkflow.Execute(ctx, types.CreateWorkflowCommand{
		AuthContext:           auth,
		RequesterRef:          request.GetRequesterRef(),
		RequesterService:      request.GetRequesterService(),
		WorkflowType:          request.GetWorkflowType(),
		RiskLevel:             request.GetRiskLevel(),
		TargetRefHash:         request.GetTargetRefHash(),
		TargetService:         request.GetTargetService(),
		TargetOperation:       request.GetTargetOperation(),
		ApprovalPolicyRef:     request.GetApprovalPolicyRef(),
		TimeoutPolicyRef:      request.GetTimeoutPolicyRef(),
		CompensationPolicyRef: request.GetCompensationPolicyRef(),
		PayloadSchemaVersion:  request.GetPayloadSchemaVersion(),
		PayloadRefHash:        request.GetPayloadRefHash(),
		ReasonRef:             request.GetReasonRef(),
		EvidenceRefs:          request.GetEvidenceRefs(),
		IdempotencyKey:        request.GetIdempotencyKey(),
		CorrelationID:         request.GetCorrelationId(),
		CausationID:           request.GetCausationId(),
		TraceID:               request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &workflowv1.CreateWorkflowResponse{
		Workflow: workflowToProto(result.Workflow),
		Replayed: result.Replayed,
	}, nil
}

func (server *Server) RecordWorkflowDecision(
	ctx context.Context,
	request *workflowv1.RecordWorkflowDecisionRequest,
) (*workflowv1.RecordWorkflowDecisionResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.recordDecision.Execute(ctx, types.RecordWorkflowDecisionCommand{
		AuthContext:       auth,
		WorkflowID:        request.GetWorkflowId(),
		StepID:            request.GetStepId(),
		DecisionType:      request.GetDecisionType(),
		DeciderRef:        request.GetDeciderRef(),
		DecisionPolicyRef: request.GetDecisionPolicyRef(),
		ReasonRef:         request.GetReasonRef(),
		EvidenceRefs:      request.GetEvidenceRefs(),
		IdempotencyKey:    request.GetIdempotencyKey(),
		CorrelationID:     request.GetCorrelationId(),
		CausationID:       request.GetCausationId(),
		TraceID:           request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &workflowv1.RecordWorkflowDecisionResponse{
		Workflow: workflowToProto(result.Workflow),
		Decision: decisionToProto(result.Decision),
		Replayed: result.Replayed,
	}, nil
}

func (server *Server) GetWorkflow(
	ctx context.Context,
	request *workflowv1.GetWorkflowRequest,
) (*workflowv1.GetWorkflowResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	workflow, decisions, err := server.getWorkflow.Execute(ctx, types.GetWorkflowCommand{
		AuthContext: auth,
		WorkflowID:  request.GetWorkflowId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	response := &workflowv1.GetWorkflowResponse{Workflow: workflowToProto(workflow)}
	for _, decision := range decisions {
		response.Decisions = append(response.Decisions, decisionToProto(decision))
	}
	return response, nil
}

func (server *Server) ListWorkflows(
	ctx context.Context,
	request *workflowv1.ListWorkflowsRequest,
) (*workflowv1.ListWorkflowsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	workflows, err := server.listWorkflows.Execute(ctx, types.ListWorkflowsCommand{
		AuthContext:       auth,
		WorkflowType:      request.GetWorkflowType(),
		Status:            request.GetStatus(),
		TargetService:     request.GetTargetService(),
		TargetOperation:   request.GetTargetOperation(),
		ApprovalPolicyRef: request.GetApprovalPolicyRef(),
		PageSize:          int(request.GetPageSize()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	response := &workflowv1.ListWorkflowsResponse{}
	for _, workflow := range workflows {
		response.Workflows = append(response.Workflows, workflowToProto(workflow))
	}
	return response, nil
}

func (server *Server) ListWorkflowCompensationInstructions(
	ctx context.Context,
	request *workflowv1.ListWorkflowCompensationInstructionsRequest,
) (*workflowv1.ListWorkflowCompensationInstructionsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	instructions, err := server.listInstructions.Execute(ctx, types.ListWorkflowCompensationInstructionsCommand{
		AuthContext: auth,
		WorkflowID:  request.GetWorkflowId(),
		Status:      request.GetStatus(),
		PageSize:    int(request.GetPageSize()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	response := &workflowv1.ListWorkflowCompensationInstructionsResponse{}
	for _, instruction := range instructions {
		response.Instructions = append(response.Instructions, instructionToProto(instruction))
	}
	return response, nil
}

func (server *Server) ListWorkflowCompensations(
	ctx context.Context,
	request *workflowv1.ListWorkflowCompensationsRequest,
) (*workflowv1.ListWorkflowCompensationsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	compensations, err := server.listCompensations.Execute(ctx, types.ListWorkflowCompensationsCommand{
		AuthContext: auth,
		WorkflowID:  request.GetWorkflowId(),
		Status:      request.GetStatus(),
		PageSize:    int(request.GetPageSize()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	response := &workflowv1.ListWorkflowCompensationsResponse{}
	for _, compensation := range compensations {
		response.Compensations = append(response.Compensations, compensationToProto(compensation))
	}
	return response, nil
}

func authFromProto(ctx context.Context, auth *workflowv1.AuthContext) (types.AuthContext, bool) {
	if verified, ok := verifiedAuthFromContext(ctx); ok {
		return verified, true
	}
	if auth == nil {
		return types.AuthContext{}, false
	}
	return types.AuthContext{
		TenantID:    types.TenantID(auth.GetTenantId()),
		UserID:      auth.GetUserId(),
		ServiceName: auth.GetServiceName(),
		InstanceRef: auth.GetInstanceRef(),
		TraceID:     auth.GetTraceId(),
		RequestID:   auth.GetRequestId(),
	}, true
}

func instructionToProto(instruction types.WorkflowCompensationInstruction) *workflowv1.WorkflowCompensationInstruction {
	return &workflowv1.WorkflowCompensationInstruction{
		TenantId:        string(instruction.TenantID),
		InstructionId:   instruction.InstructionID,
		WorkflowId:      instruction.WorkflowID,
		PayloadRefHash:  instruction.PayloadRefHash,
		TargetService:   instruction.TargetService,
		TargetOperation: instruction.TargetOperation,
		InstructionType: instruction.InstructionType,
		Environment:     instruction.Environment,
		ConfigKind:      instruction.ConfigKind,
		BundleKey:       instruction.BundleKey,
		TargetVersion:   instruction.TargetVersion,
		OperatorRef:     instruction.OperatorRef,
		ReasonRef:       instruction.ReasonRef,
		Status:          instruction.Status,
		CreatedAtUnixMs: timeToUnixMillis(instruction.CreatedAt),
		UpdatedAtUnixMs: timeToUnixMillis(instruction.UpdatedAt),
	}
}

func compensationToProto(compensation types.WorkflowCompensation) *workflowv1.WorkflowCompensation {
	return &workflowv1.WorkflowCompensation{
		TenantId:              string(compensation.TenantID),
		WorkflowId:            compensation.WorkflowID,
		CompensationId:        compensation.CompensationID,
		SourceStepId:          compensation.SourceStepID,
		TargetService:         compensation.TargetService,
		TargetOperation:       compensation.TargetOperation,
		TargetRefHash:         compensation.TargetRefHash,
		PayloadSchemaVersion:  compensation.PayloadSchemaVersion,
		PayloadRefHash:        compensation.PayloadRefHash,
		CompensationPolicyRef: compensation.CompensationPolicyRef,
		ReasonRef:             compensation.ReasonRef,
		DownstreamService:     compensation.DownstreamService,
		DownstreamRequestRef:  compensation.DownstreamRequestRef,
		Status:                compensation.Status,
		FailureClass:          compensation.FailureClass,
		PublicError:           compensation.PublicError,
		CreatedAtUnixMs:       timeToUnixMillis(compensation.CreatedAt),
		UpdatedAtUnixMs:       timeToUnixMillis(compensation.UpdatedAt),
		CompletedAtUnixMs:     timeToUnixMillis(compensation.CompletedAt),
	}
}

func workflowToProto(workflow types.Workflow) *workflowv1.Workflow {
	return &workflowv1.Workflow{
		TenantId:              string(workflow.TenantID),
		WorkflowId:            workflow.WorkflowID,
		WorkflowType:          workflow.WorkflowType,
		RiskLevel:             workflow.RiskLevel,
		RequesterRef:          workflow.RequesterRef,
		RequesterService:      workflow.RequesterService,
		TargetService:         workflow.TargetService,
		TargetOperation:       workflow.TargetOperation,
		TargetRefHash:         workflow.TargetRefHash,
		PayloadSchemaVersion:  workflow.PayloadSchemaVersion,
		PayloadRefHash:        workflow.PayloadRefHash,
		ApprovalPolicyRef:     workflow.ApprovalPolicyRef,
		TimeoutPolicyRef:      workflow.TimeoutPolicyRef,
		CompensationPolicyRef: workflow.CompensationPolicyRef,
		ReasonRef:             workflow.ReasonRef,
		EvidenceRefs:          append([]string(nil), workflow.EvidenceRefs...),
		Status:                workflow.Status,
		CurrentStepId:         workflow.CurrentStepID,
		CorrelationId:         workflow.CorrelationID,
		CausationId:           workflow.CausationID,
		TraceId:               workflow.TraceID,
		CreatedAtUnixMs:       timeToUnixMillis(workflow.CreatedAt),
		UpdatedAtUnixMs:       timeToUnixMillis(workflow.UpdatedAt),
		CompletedAtUnixMs:     timeToUnixMillis(workflow.CompletedAt),
	}
}

func decisionToProto(decision types.WorkflowDecision) *workflowv1.WorkflowDecision {
	return &workflowv1.WorkflowDecision{
		TenantId:          string(decision.TenantID),
		WorkflowId:        decision.WorkflowID,
		DecisionId:        decision.DecisionID,
		StepId:            decision.StepID,
		DeciderRef:        decision.DeciderRef,
		DecisionType:      decision.DecisionType,
		DecisionPolicyRef: decision.DecisionPolicyRef,
		ReasonRef:         decision.ReasonRef,
		EvidenceRefs:      append([]string(nil), decision.EvidenceRefs...),
		CreatedAtUnixMs:   timeToUnixMillis(decision.CreatedAt),
	}
}

func timeToUnixMillis(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().UnixMilli()
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrNotFound):
		return status.Error(codes.NotFound, "workflow not found")
	case errors.Is(err, types.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, "workflow already exists")
	case errors.Is(err, types.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, "workflow precondition failed")
	case errors.Is(err, types.ErrUnavailable), errors.Is(err, types.ErrDBReadFailed), errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "workflow temporarily unavailable")
	default:
		return status.Error(codes.Internal, "workflow internal error")
	}
}
