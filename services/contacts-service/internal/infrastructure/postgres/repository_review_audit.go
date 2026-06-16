package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

const defaultContactRequestReviewAuditLimit = 20
const maxContactRequestReviewAuditLimit = 200

type ContactRequestReviewAuditOptions struct {
	TenantID   string
	RequestID  string
	Operator   string
	Decision   string
	NextStatus string
	RiskLevel  string
	Limit      int
}

type ContactRequestReviewAuditRow struct {
	AuditID        int64
	TenantID       string
	RequestID      string
	PreviousStatus string
	NextStatus     string
	Decision       string
	Operator       string
	ReasonPresent  bool
	RiskLevel      string
	ReviewRequired bool
	ReviewedAt     time.Time
}

func (r *Repository) AuditContactRequestReviews(
	ctx context.Context,
	options ContactRequestReviewAuditOptions,
) ([]ContactRequestReviewAuditRow, error) {
	if r == nil || r.pool == nil {
		return nil, types.NewDBReadFailed("contacts repository is not configured")
	}
	options = normalizeContactRequestReviewAuditOptions(options)
	if options.TenantID == "" {
		return nil, types.NewInvalidArgument("tenant_id is required")
	}
	if options.Decision != "" && types.NormalizeContactRequestReviewDecision(types.ContactRequestReviewDecision(options.Decision)) == "" {
		return nil, types.NewInvalidArgument("decision is invalid")
	}
	if options.RiskLevel != "" && types.NormalizeContactRequestRiskLevel(types.ContactRequestRiskLevel(options.RiskLevel)) == "" {
		return nil, types.NewInvalidArgument("risk_level is invalid")
	}
	if options.NextStatus != "" && !isReviewAuditContactRequestStatus(options.NextStatus) {
		return nil, types.NewInvalidArgument("next_status is invalid")
	}
	limit := options.Limit
	if limit <= 0 {
		limit = defaultContactRequestReviewAuditLimit
	}
	if limit > maxContactRequestReviewAuditLimit {
		limit = maxContactRequestReviewAuditLimit
	}

	rows, err := r.pool.Query(ctx, `
SELECT audit_id,
       tenant_id,
       request_id,
       previous_status,
       next_status,
       decision,
       operator,
       reason <> '' AS reason_present,
       risk_level,
       review_required,
       reviewed_at
FROM contact_request_review_audit
WHERE tenant_id = $1
  AND ($2 = '' OR request_id = $2)
  AND ($3 = '' OR operator = $3)
  AND ($4 = '' OR decision = $4)
  AND ($5 = '' OR next_status = $5)
  AND ($6 = '' OR risk_level = $6)
ORDER BY reviewed_at DESC, audit_id DESC
LIMIT $7
`, options.TenantID, options.RequestID, options.Operator, options.Decision, options.NextStatus, options.RiskLevel, limit)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	auditRows := make([]ContactRequestReviewAuditRow, 0)
	for rows.Next() {
		var row ContactRequestReviewAuditRow
		if err := rows.Scan(
			&row.AuditID,
			&row.TenantID,
			&row.RequestID,
			&row.PreviousStatus,
			&row.NextStatus,
			&row.Decision,
			&row.Operator,
			&row.ReasonPresent,
			&row.RiskLevel,
			&row.ReviewRequired,
			&row.ReviewedAt,
		); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		auditRows = append(auditRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return auditRows, nil
}

func normalizeContactRequestReviewAuditOptions(options ContactRequestReviewAuditOptions) ContactRequestReviewAuditOptions {
	options.TenantID = strings.TrimSpace(options.TenantID)
	options.RequestID = strings.TrimSpace(options.RequestID)
	options.Operator = strings.TrimSpace(options.Operator)
	options.Decision = strings.ToUpper(strings.TrimSpace(options.Decision))
	options.NextStatus = strings.ToUpper(strings.TrimSpace(options.NextStatus))
	options.RiskLevel = strings.ToUpper(strings.TrimSpace(options.RiskLevel))
	return options
}

func isReviewAuditContactRequestStatus(status string) bool {
	switch types.ContactRequestStatus(status) {
	case types.ContactRequestStatusPending,
		types.ContactRequestStatusReviewRequired,
		types.ContactRequestStatusAccepted,
		types.ContactRequestStatusDeclined,
		types.ContactRequestStatusCanceled,
		types.ContactRequestStatusExpired:
		return true
	default:
		return false
	}
}
