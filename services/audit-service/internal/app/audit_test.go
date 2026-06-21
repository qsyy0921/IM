package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/audit-service/internal/domain"
	"github.com/qsyy0921/IM/services/audit-service/internal/types"
)

func TestAppendAuditRecordRejectsDisallowedAttributes(t *testing.T) {
	_, err := NewAppendAuditRecordUseCase(&fakeRepository{}, fixedAuditIDGenerator("aud_1")).Execute(
		context.Background(),
		validAppendCommand(`{"raw_prompt":"do not store"}`),
	)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestAppendAuditRecordPreparesRecord(t *testing.T) {
	repository := &fakeRepository{}
	record, err := NewAppendAuditRecordUseCase(repository, fixedAuditIDGenerator("aud_1")).Execute(
		context.Background(),
		validAppendCommand(`{"proposal_id":"proposal-1"}`),
	)
	if err != nil {
		t.Fatalf("append audit record: %v", err)
	}
	if record.AuditID != "aud_1" ||
		repository.prepared.CanonicalJSONHash == "" ||
		repository.prepared.CommandHash == "" {
		t.Fatalf("record was not prepared: record=%+v prepared=%+v", record, repository.prepared)
	}
}

func TestQueryAuditRecordsReturnsCursor(t *testing.T) {
	repository := &fakeRepository{
		queryRecords: []types.AuditRecord{
			{AuditID: "a"},
			{AuditID: "b"},
			{AuditID: "c"},
		},
	}
	result, err := NewQueryAuditRecordsUseCase(repository).Execute(context.Background(), types.QueryAuditRecordsCommand{
		AuthContext: validAuth(),
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("query audit records: %v", err)
	}
	if len(result.Records) != 2 || result.NextCursor != "b" || repository.fetchLimit != 3 {
		t.Fatalf("unexpected query result=%+v fetchLimit=%d", result, repository.fetchLimit)
	}
}

func TestCreateAuditExportPreparesJob(t *testing.T) {
	repository := &fakeRepository{}
	job, err := NewCreateAuditExportUseCase(repository, fixedAuditIDGenerator("aud_1")).Execute(
		context.Background(),
		types.CreateAuditExportCommand{
			AuthContext:      validAuth(),
			FilterHash:       "filter-hash-1",
			RedactionProfile: "ops-redacted",
			RequestedByRef:   "admin:operator-1",
			IdempotencyKey:   "idem-export-1",
			TraceID:          "trace-export",
		},
	)
	if err != nil {
		t.Fatalf("create audit export: %v", err)
	}
	if job.ExportID != "audexp_1" ||
		job.Status != types.AuditExportStatusPending ||
		repository.preparedExport.Command.AuditStream != types.DefaultAuditStream ||
		repository.preparedExport.CommandHash == "" {
		t.Fatalf("export job was not prepared: job=%+v prepared=%+v", job, repository.preparedExport)
	}
}

func validAppendCommand(attributes string) types.AppendAuditRecordCommand {
	return types.AppendAuditRecordCommand{
		AuthContext:    validAuth(),
		SourceService:  "agent-service",
		SourceEventID:  "proposal-1",
		RecordType:     "AGENT_PROPOSAL",
		Action:         "CREATE_PROPOSAL",
		Outcome:        "SUCCEEDED",
		OccurredAt:     time.Unix(100, 0).UTC(),
		AttributesJSON: attributes,
		IdempotencyKey: "idem-1",
	}
}

func validAuth() types.AuthContext {
	return types.AuthContext{
		TenantID: "tenant-1",
		UserID:   "user-1",
		DeviceID: "device-1",
	}
}

type fixedAuditIDGenerator string

func (generator fixedAuditIDGenerator) NewAuditID() (string, error) {
	return string(generator), nil
}

func (generator fixedAuditIDGenerator) NewAuditExportID() (string, error) {
	return "audexp" + strings.TrimPrefix(string(generator), "aud"), nil
}

type fakeRepository struct {
	prepared       domain.PreparedRecord
	preparedExport domain.PreparedExport
	queryRecords   []types.AuditRecord
	fetchLimit     int
}

func (repository *fakeRepository) AppendAuditRecord(
	_ context.Context,
	prepared domain.PreparedRecord,
	auditID string,
) (types.AuditRecord, error) {
	repository.prepared = prepared
	return types.AuditRecord{
		TenantID:          prepared.Command.AuthContext.TenantID,
		AuditID:           auditID,
		AuditStream:       prepared.Command.AuditStream,
		SourceService:     prepared.Command.SourceService,
		SourceEventID:     prepared.Command.SourceEventID,
		RecordType:        prepared.Command.RecordType,
		AttributesJSON:    prepared.Command.AttributesJSON,
		CanonicalJSONHash: prepared.CanonicalJSONHash,
		CommandHash:       prepared.CommandHash,
	}, nil
}

func (repository *fakeRepository) QueryAuditRecords(
	_ context.Context,
	_ types.QueryAuditRecordsCommand,
	fetchLimit int,
) ([]types.AuditRecord, error) {
	repository.fetchLimit = fetchLimit
	return repository.queryRecords, nil
}

func (repository *fakeRepository) CreateAuditExport(
	_ context.Context,
	prepared domain.PreparedExport,
	exportID string,
) (types.AuditExportJob, error) {
	repository.preparedExport = prepared
	return types.AuditExportJob{
		TenantID:         prepared.Command.AuthContext.TenantID,
		ExportID:         exportID,
		Status:           types.AuditExportStatusPending,
		AuditStream:      prepared.Command.AuditStream,
		FilterHash:       prepared.Command.FilterHash,
		RedactionProfile: prepared.Command.RedactionProfile,
		RequestedByRef:   prepared.Command.RequestedByRef,
		IdempotencyKey:   prepared.Command.IdempotencyKey,
		CommandHash:      prepared.CommandHash,
	}, nil
}

func (repository *fakeRepository) GetAuditExport(
	_ context.Context,
	_ types.TenantID,
	exportID string,
) (types.AuditExportJob, error) {
	return types.AuditExportJob{ExportID: exportID, Status: types.AuditExportStatusPending}, nil
}

func (repository *fakeRepository) VerifyAuditProof(
	_ context.Context,
	_ types.TenantID,
	auditID string,
) (types.AuditProofVerification, error) {
	return types.AuditProofVerification{AuditID: auditID, Valid: true}, nil
}
