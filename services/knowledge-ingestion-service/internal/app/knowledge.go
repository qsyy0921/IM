package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/domain"
	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/types"
)

type CreateKnowledgeSourceResult struct {
	Source   types.KnowledgeSource
	Replayed bool
}

type SubmitIngestionJobResult struct {
	Job        types.KnowledgeIngestionJob
	Replayed   bool
	DocumentID string
	ChunkCount int
}

type CreateKnowledgeSourceUseCase struct {
	repository Repository
	ids        IDGenerator
}

func NewCreateKnowledgeSourceUseCase(repository Repository, ids IDGenerator) *CreateKnowledgeSourceUseCase {
	return &CreateKnowledgeSourceUseCase{repository: repository, ids: ids}
}

func (useCase *CreateKnowledgeSourceUseCase) Execute(
	ctx context.Context,
	command types.CreateKnowledgeSourceCommand,
) (CreateKnowledgeSourceResult, error) {
	if useCase.repository == nil {
		return CreateKnowledgeSourceResult{}, types.NewUnavailable("knowledge repository is not configured")
	}
	prepared, err := domain.PrepareKnowledgeSource(command, useCase.ids.NewSourceID(), time.Now())
	if err != nil {
		return CreateKnowledgeSourceResult{}, err
	}
	source, replayed, err := useCase.repository.CreateKnowledgeSource(ctx, prepared)
	if err != nil {
		return CreateKnowledgeSourceResult{}, err
	}
	return CreateKnowledgeSourceResult{Source: source, Replayed: replayed}, nil
}

type SubmitIngestionJobUseCase struct {
	repository Repository
	ids        IDGenerator
}

func NewSubmitIngestionJobUseCase(repository Repository, ids IDGenerator) *SubmitIngestionJobUseCase {
	return &SubmitIngestionJobUseCase{repository: repository, ids: ids}
}

func (useCase *SubmitIngestionJobUseCase) Execute(
	ctx context.Context,
	command types.SubmitIngestionJobCommand,
) (SubmitIngestionJobResult, error) {
	if useCase.repository == nil {
		return SubmitIngestionJobResult{}, types.NewUnavailable("knowledge repository is not configured")
	}
	chunkIDs := make([]string, 0, len(command.Chunks))
	for index := range command.Chunks {
		chunkIDs = append(chunkIDs, useCase.ids.NewChunkID(index))
	}
	prepared, err := domain.PrepareIngestionJob(command, useCase.ids.NewJobID(), useCase.ids.NewDocumentID(), chunkIDs, time.Now())
	if err != nil {
		return SubmitIngestionJobResult{}, err
	}
	job, replayed, err := useCase.repository.SubmitIngestionJob(ctx, prepared)
	if err != nil {
		return SubmitIngestionJobResult{}, err
	}
	return SubmitIngestionJobResult{
		Job:        job,
		Replayed:   replayed,
		DocumentID: job.DocumentID,
		ChunkCount: job.ChunkCount,
	}, nil
}

type GetIngestionJobUseCase struct {
	repository Repository
}

func NewGetIngestionJobUseCase(repository Repository) *GetIngestionJobUseCase {
	return &GetIngestionJobUseCase{repository: repository}
}

func (useCase *GetIngestionJobUseCase) Execute(
	ctx context.Context,
	command types.GetIngestionJobCommand,
) (types.KnowledgeIngestionJob, error) {
	if useCase.repository == nil {
		return types.KnowledgeIngestionJob{}, types.NewUnavailable("knowledge repository is not configured")
	}
	if err := command.Validate(); err != nil {
		return types.KnowledgeIngestionJob{}, err
	}
	return useCase.repository.GetIngestionJob(ctx, command.AuthContext.TenantID, command.JobID)
}

type ListKnowledgeChunksUseCase struct {
	repository Repository
}

func NewListKnowledgeChunksUseCase(repository Repository) *ListKnowledgeChunksUseCase {
	return &ListKnowledgeChunksUseCase{repository: repository}
}

func (useCase *ListKnowledgeChunksUseCase) Execute(
	ctx context.Context,
	command types.ListKnowledgeChunksCommand,
) ([]types.KnowledgeChunk, string, error) {
	if useCase.repository == nil {
		return nil, "", types.NewUnavailable("knowledge repository is not configured")
	}
	if err := command.Validate(); err != nil {
		return nil, "", err
	}
	if command.PageSize == 0 {
		command.PageSize = 50
	}
	return useCase.repository.ListKnowledgeChunks(ctx, command)
}

type RandomIDGenerator struct{}

func NewRandomIDGenerator() RandomIDGenerator {
	return RandomIDGenerator{}
}

func (RandomIDGenerator) NewSourceID() string {
	return "ksrc_" + randomHex()
}

func (RandomIDGenerator) NewJobID() string {
	return "kjob_" + randomHex()
}

func (RandomIDGenerator) NewDocumentID() string {
	return "kdoc_" + randomHex()
}

func (RandomIDGenerator) NewChunkID(index int) string {
	return fmt.Sprintf("kchk_%s_%03d", randomHex(), index)
}

func randomHex() string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(value[:])
}
