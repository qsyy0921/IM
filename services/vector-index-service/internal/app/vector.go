package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/domain"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type RandomIDGenerator struct{}

func NewRandomIDGenerator() RandomIDGenerator {
	return RandomIDGenerator{}
}

func (RandomIDGenerator) NewVectorItemID() string {
	return "vitem_" + randomHex(16)
}

func (RandomIDGenerator) NewJobID() string {
	return "vjob_" + randomHex(16)
}

func (RandomIDGenerator) NewTombstoneID() string {
	return "vtomb_" + randomHex(16)
}

type UpsertVectorItemUseCase struct {
	repository VectorRepository
	ids        IDGenerator
}

type UpsertVectorItemResult struct {
	Item     types.VectorItem
	Job      types.VectorIndexJob
	Replayed bool
}

func NewUpsertVectorItemUseCase(repository VectorRepository, ids IDGenerator) UpsertVectorItemUseCase {
	return UpsertVectorItemUseCase{repository: repository, ids: ids}
}

func (useCase UpsertVectorItemUseCase) Execute(ctx context.Context, command types.UpsertVectorItemCommand) (UpsertVectorItemResult, error) {
	if useCase.repository == nil || useCase.ids == nil {
		return UpsertVectorItemResult{}, types.NewUnavailable("vector upsert dependencies are not configured")
	}
	prepared, err := domain.PrepareUpsert(command, useCase.ids.NewVectorItemID(), useCase.ids.NewJobID(), time.Now().UTC())
	if err != nil {
		return UpsertVectorItemResult{}, err
	}
	item, job, replayed, err := useCase.repository.UpsertVectorItem(ctx, prepared)
	if err != nil {
		return UpsertVectorItemResult{}, err
	}
	return UpsertVectorItemResult{Item: item, Job: job, Replayed: replayed}, nil
}

type TombstoneVectorItemUseCase struct {
	repository VectorRepository
	ids        IDGenerator
}

type TombstoneVectorItemResult struct {
	Item        types.VectorItem
	Job         types.VectorIndexJob
	TombstoneID string
	Replayed    bool
}

func NewTombstoneVectorItemUseCase(repository VectorRepository, ids IDGenerator) TombstoneVectorItemUseCase {
	return TombstoneVectorItemUseCase{repository: repository, ids: ids}
}

func (useCase TombstoneVectorItemUseCase) Execute(ctx context.Context, command types.TombstoneVectorItemCommand) (TombstoneVectorItemResult, error) {
	if useCase.repository == nil || useCase.ids == nil {
		return TombstoneVectorItemResult{}, types.NewUnavailable("vector tombstone dependencies are not configured")
	}
	prepared, err := domain.PrepareTombstone(command, useCase.ids.NewTombstoneID(), useCase.ids.NewJobID(), time.Now().UTC())
	if err != nil {
		return TombstoneVectorItemResult{}, err
	}
	item, job, tombstoneID, replayed, err := useCase.repository.TombstoneVectorItem(ctx, prepared)
	if err != nil {
		return TombstoneVectorItemResult{}, err
	}
	return TombstoneVectorItemResult{Item: item, Job: job, TombstoneID: tombstoneID, Replayed: replayed}, nil
}

type SearchVectorsUseCase struct {
	repository VectorRepository
}

func NewSearchVectorsUseCase(repository VectorRepository) SearchVectorsUseCase {
	return SearchVectorsUseCase{repository: repository}
}

func (useCase SearchVectorsUseCase) Execute(ctx context.Context, command types.SearchVectorsCommand) ([]types.VectorSearchResult, error) {
	if useCase.repository == nil {
		return nil, types.NewUnavailable("vector search repository is not configured")
	}
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	return useCase.repository.SearchVectors(ctx, normalized)
}

type GetVectorIndexJobUseCase struct {
	repository VectorRepository
}

func NewGetVectorIndexJobUseCase(repository VectorRepository) GetVectorIndexJobUseCase {
	return GetVectorIndexJobUseCase{repository: repository}
}

func (useCase GetVectorIndexJobUseCase) Execute(ctx context.Context, command types.GetVectorIndexJobCommand) (types.VectorIndexJob, error) {
	if useCase.repository == nil {
		return types.VectorIndexJob{}, types.NewUnavailable("vector job repository is not configured")
	}
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return types.VectorIndexJob{}, err
	}
	return useCase.repository.GetVectorIndexJob(ctx, normalized)
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
