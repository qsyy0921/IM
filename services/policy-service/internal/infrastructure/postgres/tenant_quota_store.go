package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type TenantQuotaStore struct {
	pool *pgxpool.Pool
}

type TenantQuotaAuditOptions struct {
	TenantID string
	Action   string
	Enabled  *bool
	Limit    int
}

type TenantQuotaSetOptions struct {
	TenantID          string
	Action            string
	MaxDecisions      int
	WindowSeconds     int
	PermissionVersion int64
	Classification    string
	Reason            string
	Enabled           bool
	Source            string
}

type TenantQuotaRow struct {
	TenantID          string
	Action            string
	MaxDecisions      int
	WindowSeconds     int
	PermissionVersion int64
	Classification    string
	Reason            string
	Enabled           bool
	Source            string
	UpdatedAt         time.Time
}

func NewTenantQuotaStore(pool *pgxpool.Pool) *TenantQuotaStore {
	return &TenantQuotaStore{pool: pool}
}

func (store *TenantQuotaStore) AuditTenantQuotas(ctx context.Context, options TenantQuotaAuditOptions) ([]TenantQuotaRow, error) {
	if store == nil || store.pool == nil {
		return nil, errors.New("policy tenant quota store is not configured")
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	var args []any
	clauses := make([]string, 0, 3)
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "tenant_id = $"+strconv.Itoa(len(args)))
	}
	if action := normalizePolicyMessageAction(options.Action); action != "" {
		args = append(args, action)
		clauses = append(clauses, "action = $"+strconv.Itoa(len(args)))
	} else if strings.TrimSpace(options.Action) != "" {
		return nil, types.NewInvalidArgument("unsupported policy tenant quota action")
	}
	if options.Enabled != nil {
		args = append(args, *options.Enabled)
		clauses = append(clauses, "enabled = $"+strconv.Itoa(len(args)))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	rows, err := store.pool.Query(ctx, `
SELECT
    tenant_id,
    action,
    max_decisions,
    window_seconds,
    permission_version,
    classification,
    reason,
    enabled,
    source,
    updated_at
FROM policy_tenant_message_action_quotas
`+where+`
ORDER BY updated_at DESC, tenant_id, action
LIMIT $`+strconv.Itoa(len(args)), args...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if isUndefinedTable(err) {
		return nil, nil
	}
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	result := make([]TenantQuotaRow, 0, limit)
	for rows.Next() {
		var row TenantQuotaRow
		if err := rows.Scan(
			&row.TenantID,
			&row.Action,
			&row.MaxDecisions,
			&row.WindowSeconds,
			&row.PermissionVersion,
			&row.Classification,
			&row.Reason,
			&row.Enabled,
			&row.Source,
			&row.UpdatedAt,
		); err != nil {
			return nil, types.NewDBWriteFailed(err.Error())
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func (store *TenantQuotaStore) SetTenantQuota(ctx context.Context, options TenantQuotaSetOptions) (TenantQuotaRow, error) {
	if store == nil || store.pool == nil {
		return TenantQuotaRow{}, errors.New("policy tenant quota store is not configured")
	}
	tenantID := strings.TrimSpace(options.TenantID)
	action := normalizePolicyMessageAction(options.Action)
	classification := strings.TrimSpace(options.Classification)
	source := strings.TrimSpace(options.Source)
	if source == "" {
		source = "manual"
	}
	if tenantID == "" {
		return TenantQuotaRow{}, types.NewInvalidArgument("tenant_id is required")
	}
	if action == "" {
		return TenantQuotaRow{}, types.NewInvalidArgument("unsupported policy tenant quota action")
	}
	if options.MaxDecisions <= 0 {
		return TenantQuotaRow{}, types.NewInvalidArgument("max_decisions must be positive")
	}
	if options.WindowSeconds <= 0 {
		return TenantQuotaRow{}, types.NewInvalidArgument("window_seconds must be positive")
	}
	if options.PermissionVersion <= 0 {
		return TenantQuotaRow{}, types.NewInvalidArgument("permission_version must be positive")
	}
	if classification == "" {
		return TenantQuotaRow{}, types.NewInvalidArgument("classification is required")
	}
	if source == "" {
		return TenantQuotaRow{}, types.NewInvalidArgument("source is required")
	}

	var row TenantQuotaRow
	err := store.pool.QueryRow(ctx, `
INSERT INTO policy_tenant_message_action_quotas (
    tenant_id,
    action,
    max_decisions,
    window_seconds,
    permission_version,
    classification,
    reason,
    enabled,
    source,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
ON CONFLICT (tenant_id, action) DO UPDATE
SET max_decisions = EXCLUDED.max_decisions,
    window_seconds = EXCLUDED.window_seconds,
    permission_version = EXCLUDED.permission_version,
    classification = EXCLUDED.classification,
    reason = EXCLUDED.reason,
    enabled = EXCLUDED.enabled,
    source = EXCLUDED.source,
    updated_at = now()
RETURNING tenant_id, action, max_decisions, window_seconds, permission_version, classification, reason, enabled, source, updated_at
`,
		tenantID,
		action,
		options.MaxDecisions,
		options.WindowSeconds,
		options.PermissionVersion,
		classification,
		strings.TrimSpace(options.Reason),
		options.Enabled,
		source,
	).Scan(
		&row.TenantID,
		&row.Action,
		&row.MaxDecisions,
		&row.WindowSeconds,
		&row.PermissionVersion,
		&row.Classification,
		&row.Reason,
		&row.Enabled,
		&row.Source,
		&row.UpdatedAt,
	)
	if err != nil {
		return TenantQuotaRow{}, types.NewDBWriteFailed(err.Error())
	}
	return row, nil
}
