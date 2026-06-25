package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/qsyy0921/IM/services/workflow-service/internal/domain"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

type RandomIDGenerator struct{}

func NewRandomIDGenerator() RandomIDGenerator {
	return RandomIDGenerator{}
}

func (RandomIDGenerator) NewWorkflowID() string {
	return "wf_" + randomHex(16)
}

func (RandomIDGenerator) NewStepID() string {
	return "wfs_" + randomHex(16)
}

func (RandomIDGenerator) NewDecisionID() string {
	return "wfd_" + randomHex(16)
}

type CreateWorkflowUseCase struct {
	repository WorkflowRepository
	ids        IDGenerator
}

type CreateWorkflowResult struct {
	Workflow types.Workflow
	Replayed bool
}

func NewCreateWorkflowUseCase(repository WorkflowRepository, ids IDGenerator) CreateWorkflowUseCase {
	return CreateWorkflowUseCase{repository: repository, ids: ids}
}

func (useCase CreateWorkflowUseCase) Execute(ctx context.Context, command types.CreateWorkflowCommand) (CreateWorkflowResult, error) {
	if useCase.repository == nil || useCase.ids == nil {
		return CreateWorkflowResult{}, types.NewUnavailable("workflow create dependencies are not configured")
	}
	prepared, err := domain.PrepareWorkflow(command, useCase.ids.NewWorkflowID(), useCase.ids.NewStepID(), time.Now().UTC())
	if err != nil {
		return CreateWorkflowResult{}, err
	}
	workflow, replayed, err := useCase.repository.CreateWorkflow(ctx, prepared)
	if err != nil {
		return CreateWorkflowResult{}, err
	}
	return CreateWorkflowResult{Workflow: workflow, Replayed: replayed}, nil
}

type RecordWorkflowDecisionUseCase struct {
	repository WorkflowRepository
	ids        IDGenerator
}

type RecordWorkflowDecisionResult struct {
	Workflow types.Workflow
	Decision types.WorkflowDecision
	Replayed bool
}

func NewRecordWorkflowDecisionUseCase(repository WorkflowRepository, ids IDGenerator) RecordWorkflowDecisionUseCase {
	return RecordWorkflowDecisionUseCase{repository: repository, ids: ids}
}

func (useCase RecordWorkflowDecisionUseCase) Execute(ctx context.Context, command types.RecordWorkflowDecisionCommand) (RecordWorkflowDecisionResult, error) {
	if useCase.repository == nil || useCase.ids == nil {
		return RecordWorkflowDecisionResult{}, types.NewUnavailable("workflow decision dependencies are not configured")
	}
	prepared, err := domain.PrepareDecision(command, useCase.ids.NewDecisionID(), time.Now().UTC())
	if err != nil {
		return RecordWorkflowDecisionResult{}, err
	}
	workflow, decision, replayed, err := useCase.repository.RecordWorkflowDecision(ctx, prepared)
	if err != nil {
		return RecordWorkflowDecisionResult{}, err
	}
	return RecordWorkflowDecisionResult{Workflow: workflow, Decision: decision, Replayed: replayed}, nil
}

type GetWorkflowUseCase struct {
	repository WorkflowRepository
}

func NewGetWorkflowUseCase(repository WorkflowRepository) GetWorkflowUseCase {
	return GetWorkflowUseCase{repository: repository}
}

func (useCase GetWorkflowUseCase) Execute(ctx context.Context, command types.GetWorkflowCommand) (types.Workflow, []types.WorkflowDecision, error) {
	if useCase.repository == nil {
		return types.Workflow{}, nil, types.NewUnavailable("workflow repository is not configured")
	}
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return types.Workflow{}, nil, err
	}
	return useCase.repository.GetWorkflow(ctx, normalized)
}

type ListWorkflowsUseCase struct {
	repository WorkflowRepository
}

func NewListWorkflowsUseCase(repository WorkflowRepository) ListWorkflowsUseCase {
	return ListWorkflowsUseCase{repository: repository}
}

func (useCase ListWorkflowsUseCase) Execute(ctx context.Context, command types.ListWorkflowsCommand) ([]types.Workflow, error) {
	if useCase.repository == nil {
		return nil, types.NewUnavailable("workflow repository is not configured")
	}
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	return useCase.repository.ListWorkflows(ctx, normalized)
}

type ListWorkflowCompensationInstructionsUseCase struct {
	repository WorkflowRepository
}

func NewListWorkflowCompensationInstructionsUseCase(repository WorkflowRepository) ListWorkflowCompensationInstructionsUseCase {
	return ListWorkflowCompensationInstructionsUseCase{repository: repository}
}

func (useCase ListWorkflowCompensationInstructionsUseCase) Execute(
	ctx context.Context,
	command types.ListWorkflowCompensationInstructionsCommand,
) ([]types.WorkflowCompensationInstruction, error) {
	if useCase.repository == nil {
		return nil, types.NewUnavailable("workflow repository is not configured")
	}
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	return useCase.repository.ListWorkflowCompensationInstructions(ctx, normalized)
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
