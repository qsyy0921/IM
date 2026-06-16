package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

const (
	MessageComplianceExternalProofStatusVerified = "VERIFIED"
	MessageComplianceExternalProofStatusRevoked  = "REVOKED"
)

type MessageComplianceExternalProofMutationOptions struct {
	TenantID         string
	ExternalProofRef string
	Provider         string
	ProofHash        string
	OperatorID       string
	Now              time.Time
}

type MessageComplianceExternalProofResult struct {
	TenantID         string
	ExternalProofRef string
	Status           string
	Provider         string
	ProofHash        string
	VerifiedBy       string
	VerifiedAt       time.Time
	RevokedBy        string
	RevokedAt        *time.Time
	UpdatedAt        time.Time
}

type MessageComplianceExternalProofAuditOptions struct {
	TenantID         string
	ExternalProofRef string
	Status           string
	Provider         string
	UpdatedAfter     *time.Time
	UpdatedBefore    *time.Time
	Limit            int
}

type MessageComplianceExternalProofAuditRow = MessageComplianceExternalProofResult

func (r *MessageRepository) RegisterComplianceExternalProof(ctx context.Context, options MessageComplianceExternalProofMutationOptions) (MessageComplianceExternalProofResult, error) {
	if r.pool == nil {
		return MessageComplianceExternalProofResult{}, ErrRepositoryNotConfigured
	}
	options = normalizeComplianceExternalProofOptions(options, r.now())
	if err := validateComplianceExternalProofOptions(options, true, true); err != nil {
		return MessageComplianceExternalProofResult{}, err
	}

	row := r.pool.QueryRow(ctx, `
INSERT INTO message_compliance_external_proofs (
    tenant_id,
    external_proof_ref,
    status,
    provider,
    proof_hash,
    verified_by,
    verified_at,
    updated_at
) VALUES ($1, $2, 'VERIFIED', $3, $4, $5, $6, $6)
ON CONFLICT (tenant_id, external_proof_ref) DO UPDATE
SET status = 'VERIFIED',
    provider = EXCLUDED.provider,
    proof_hash = EXCLUDED.proof_hash,
    verified_by = EXCLUDED.verified_by,
    verified_at = EXCLUDED.verified_at,
    revoked_by = '',
    revoked_at = NULL,
    updated_at = EXCLUDED.updated_at
RETURNING tenant_id, external_proof_ref, status, provider, proof_hash, verified_by, verified_at, revoked_by, revoked_at, updated_at
`, options.TenantID, options.ExternalProofRef, options.Provider, options.ProofHash, options.OperatorID, options.Now)
	return scanComplianceExternalProofRow(row)
}

func (r *MessageRepository) RevokeComplianceExternalProof(ctx context.Context, options MessageComplianceExternalProofMutationOptions) (MessageComplianceExternalProofResult, error) {
	if r.pool == nil {
		return MessageComplianceExternalProofResult{}, ErrRepositoryNotConfigured
	}
	options = normalizeComplianceExternalProofOptions(options, r.now())
	if err := validateComplianceExternalProofOptions(options, false, false); err != nil {
		return MessageComplianceExternalProofResult{}, err
	}

	row := r.pool.QueryRow(ctx, `
UPDATE message_compliance_external_proofs
SET status = 'REVOKED',
    revoked_by = $3,
    revoked_at = $4,
    updated_at = $4
WHERE tenant_id = $1
  AND external_proof_ref = $2
  AND status = 'VERIFIED'
RETURNING tenant_id, external_proof_ref, status, provider, proof_hash, verified_by, verified_at, revoked_by, revoked_at, updated_at
`, options.TenantID, options.ExternalProofRef, options.OperatorID, options.Now)
	result, err := scanComplianceExternalProofRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MessageComplianceExternalProofResult{}, types.NewInvalidMessageState("verified compliance external proof not found")
		}
		return MessageComplianceExternalProofResult{}, err
	}
	return result, nil
}

func (r *MessageRepository) AuditComplianceExternalProofs(ctx context.Context, options MessageComplianceExternalProofAuditOptions) ([]MessageComplianceExternalProofAuditRow, error) {
	if r.pool == nil {
		return nil, ErrRepositoryNotConfigured
	}
	limit := options.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	status := strings.ToUpper(strings.TrimSpace(options.Status))
	if status != "" &&
		status != MessageComplianceExternalProofStatusVerified &&
		status != MessageComplianceExternalProofStatusRevoked {
		return nil, errors.New("unsupported message compliance external proof status")
	}

	args := []any{}
	clauses := []string{"1 = 1"}
	addClause := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, clause)
	}
	if value := strings.TrimSpace(options.TenantID); value != "" {
		addClause("tenant_id = $"+strconv.Itoa(len(args)+1), value)
	}
	if value := strings.TrimSpace(options.ExternalProofRef); value != "" {
		addClause("external_proof_ref = $"+strconv.Itoa(len(args)+1), value)
	}
	if status != "" {
		addClause("status = $"+strconv.Itoa(len(args)+1), status)
	}
	if value := strings.TrimSpace(options.Provider); value != "" {
		addClause("provider = $"+strconv.Itoa(len(args)+1), value)
	}
	if options.UpdatedAfter != nil {
		addClause("updated_at >= $"+strconv.Itoa(len(args)+1), options.UpdatedAfter.UTC())
	}
	if options.UpdatedBefore != nil {
		addClause("updated_at < $"+strconv.Itoa(len(args)+1), options.UpdatedBefore.UTC())
	}
	if options.UpdatedAfter != nil && options.UpdatedBefore != nil && !options.UpdatedAfter.Before(*options.UpdatedBefore) {
		return nil, types.NewInvalidArgument("updated_after must be before updated_before")
	}
	args = append(args, limit)
	rows, err := r.pool.Query(ctx, `
SELECT tenant_id, external_proof_ref, status, provider, proof_hash, verified_by, verified_at, revoked_by, revoked_at, updated_at
FROM message_compliance_external_proofs
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY updated_at DESC, id DESC
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()
	result := make([]MessageComplianceExternalProofAuditRow, 0, limit)
	for rows.Next() {
		row, err := scanComplianceExternalProofRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func lockVerifiedComplianceExternalProof(ctx context.Context, tx pgx.Tx, tenantID string, externalProofRef string) error {
	var proofRef string
	err := tx.QueryRow(ctx, `
SELECT external_proof_ref
FROM message_compliance_external_proofs
WHERE tenant_id = $1
  AND external_proof_ref = $2
  AND status = 'VERIFIED'
FOR UPDATE
`, strings.TrimSpace(tenantID), strings.TrimSpace(externalProofRef)).Scan(&proofRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.NewInvalidMessageState("verified compliance external proof not found")
	}
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func normalizeComplianceExternalProofOptions(options MessageComplianceExternalProofMutationOptions, fallbackNow time.Time) MessageComplianceExternalProofMutationOptions {
	options.TenantID = strings.TrimSpace(options.TenantID)
	options.ExternalProofRef = strings.TrimSpace(options.ExternalProofRef)
	options.Provider = strings.TrimSpace(options.Provider)
	options.ProofHash = strings.TrimSpace(options.ProofHash)
	options.OperatorID = strings.TrimSpace(options.OperatorID)
	if options.Now.IsZero() {
		options.Now = fallbackNow
	}
	return options
}

func validateComplianceExternalProofOptions(options MessageComplianceExternalProofMutationOptions, requireProvider bool, requireHash bool) error {
	if options.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if options.ExternalProofRef == "" {
		return errors.New("external_proof_ref is required")
	}
	if requireProvider && options.Provider == "" {
		return errors.New("provider is required")
	}
	if requireHash && options.ProofHash == "" {
		return errors.New("proof_hash is required")
	}
	if options.OperatorID == "" {
		return errors.New("operator_id is required")
	}
	return nil
}

type complianceExternalProofScanner interface {
	Scan(dest ...any) error
}

func scanComplianceExternalProofRow(scanner complianceExternalProofScanner) (MessageComplianceExternalProofResult, error) {
	var row MessageComplianceExternalProofResult
	if err := scanner.Scan(
		&row.TenantID,
		&row.ExternalProofRef,
		&row.Status,
		&row.Provider,
		&row.ProofHash,
		&row.VerifiedBy,
		&row.VerifiedAt,
		&row.RevokedBy,
		&row.RevokedAt,
		&row.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MessageComplianceExternalProofResult{}, pgx.ErrNoRows
		}
		return MessageComplianceExternalProofResult{}, types.NewDBWriteFailed(err.Error())
	}
	return row, nil
}
