package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/qsyy0921/IM/services/audit-service/internal/domain"
	"github.com/qsyy0921/IM/services/audit-service/internal/types"
)

type AppendAuditRecordUseCase struct {
	repository Repository
	auditIDs   AuditIDGenerator
}

func NewAppendAuditRecordUseCase(repository Repository, auditIDs AuditIDGenerator) *AppendAuditRecordUseCase {
	return &AppendAuditRecordUseCase{repository: repository, auditIDs: auditIDs}
}

func (useCase *AppendAuditRecordUseCase) Execute(
	ctx context.Context,
	command types.AppendAuditRecordCommand,
) (types.AuditRecord, error) {
	if useCase.repository == nil {
		return types.AuditRecord{}, types.NewDBWriteFailed("audit repository is not configured")
	}
	if useCase.auditIDs == nil {
		return types.AuditRecord{}, types.NewDBWriteFailed("audit id generator is not configured")
	}
	prepared, err := domain.PrepareRecord(command)
	if err != nil {
		return types.AuditRecord{}, err
	}
	auditID, err := useCase.auditIDs.NewAuditID()
	if err != nil {
		return types.AuditRecord{}, err
	}
	return useCase.repository.AppendAuditRecord(ctx, prepared, auditID)
}

type QueryAuditRecordsUseCase struct {
	repository Repository
}

func NewQueryAuditRecordsUseCase(repository Repository) *QueryAuditRecordsUseCase {
	return &QueryAuditRecordsUseCase{repository: repository}
}

func (useCase *QueryAuditRecordsUseCase) Execute(
	ctx context.Context,
	command types.QueryAuditRecordsCommand,
) (types.QueryAuditRecordsResult, error) {
	if err := command.Validate(); err != nil {
		return types.QueryAuditRecordsResult{}, err
	}
	if useCase.repository == nil {
		return types.QueryAuditRecordsResult{}, types.NewDBReadFailed("audit repository is not configured")
	}
	limit := command.EffectiveLimit()
	records, err := useCase.repository.QueryAuditRecords(ctx, command, limit+1)
	if err != nil {
		return types.QueryAuditRecordsResult{}, err
	}
	result := types.QueryAuditRecordsResult{Records: records}
	if len(result.Records) > limit {
		result.NextCursor = result.Records[limit-1].AuditID
		result.Records = result.Records[:limit]
	}
	return result, nil
}

type VerifyAuditProofUseCase struct {
	repository Repository
}

func NewVerifyAuditProofUseCase(repository Repository) *VerifyAuditProofUseCase {
	return &VerifyAuditProofUseCase{repository: repository}
}

func (useCase *VerifyAuditProofUseCase) Execute(
	ctx context.Context,
	command types.VerifyAuditProofCommand,
) (types.AuditProofVerification, error) {
	if err := command.Validate(); err != nil {
		return types.AuditProofVerification{}, err
	}
	if useCase.repository == nil {
		return types.AuditProofVerification{}, types.NewDBReadFailed("audit repository is not configured")
	}
	return useCase.repository.VerifyAuditProof(ctx, command.AuthContext.TenantID, command.AuditID)
}

type RandomAuditIDGenerator struct{}

func NewRandomAuditIDGenerator() RandomAuditIDGenerator {
	return RandomAuditIDGenerator{}
}

func (RandomAuditIDGenerator) NewAuditID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", types.NewDBWriteFailed("audit id generation failed")
	}
	return "aud_" + hex.EncodeToString(raw[:]), nil
}
