package app

import (
	"context"

	"github.com/qsyy0921/IM/services/audit-service/internal/domain"
	"github.com/qsyy0921/IM/services/audit-service/internal/types"
)

type Repository interface {
	AppendAuditRecord(ctx context.Context, prepared domain.PreparedRecord, auditID string) (types.AuditRecord, error)
	QueryAuditRecords(ctx context.Context, command types.QueryAuditRecordsCommand, fetchLimit int) ([]types.AuditRecord, error)
	CreateAuditExport(ctx context.Context, prepared domain.PreparedExport, exportID string) (types.AuditExportJob, error)
	GetAuditExport(ctx context.Context, tenantID types.TenantID, exportID string) (types.AuditExportJob, error)
	VerifyAuditProof(ctx context.Context, tenantID types.TenantID, auditID string) (types.AuditProofVerification, error)
}

type AuditIDGenerator interface {
	NewAuditID() (string, error)
	NewAuditExportID() (string, error)
}
