package main

import (
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type contactRequestReviewOutput struct {
	GeneratedAt    string `json:"generated_at"`
	TenantID       string `json:"tenant_id"`
	RequestID      string `json:"request_id"`
	SenderUserID   string `json:"sender_user_id"`
	ReceiverUserID string `json:"receiver_user_id"`
	PreviousStatus string `json:"previous_status"`
	Status         string `json:"status"`
	Decision       string `json:"decision"`
	RiskLevel      string `json:"risk_level"`
	ReviewRequired bool   `json:"review_required"`
	Operator       string `json:"operator"`
	ReasonPresent  bool   `json:"reason_present"`
}

func writeContactRequestReviewOutput(path string, result types.ReviewContactRequestResult, operator string, reasonPresent bool) error {
	return writeJSONFile(path, contactRequestReviewOutput{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		TenantID:       string(result.TenantID),
		RequestID:      result.RequestID,
		SenderUserID:   string(result.SenderUserID),
		ReceiverUserID: string(result.ReceiverUserID),
		PreviousStatus: string(result.PreviousStatus),
		Status:         string(result.Status),
		Decision:       string(result.Decision),
		RiskLevel:      string(result.RiskLevel),
		ReviewRequired: result.ReviewRequired,
		Operator:       operator,
		ReasonPresent:  reasonPresent,
	})
}

type contactRequestReviewAuditOutput struct {
	GeneratedAt string                               `json:"generated_at"`
	Count       int                                  `json:"count"`
	Rows        []contactRequestReviewAuditOutputRow `json:"rows"`
}

type contactRequestReviewAuditOutputRow struct {
	AuditID        int64  `json:"audit_id"`
	TenantID       string `json:"tenant_id"`
	RequestID      string `json:"request_id"`
	PreviousStatus string `json:"previous_status"`
	NextStatus     string `json:"next_status"`
	Decision       string `json:"decision"`
	Operator       string `json:"operator"`
	ReasonPresent  bool   `json:"reason_present"`
	SourceType     string `json:"source_type"`
	RiskLevel      string `json:"risk_level"`
	ReviewRequired bool   `json:"review_required"`
	ReviewedAt     string `json:"reviewed_at"`
}

func writeContactRequestReviewAuditOutput(path string, rows []postgresinfra.ContactRequestReviewAuditRow) error {
	outputRows := make([]contactRequestReviewAuditOutputRow, 0, len(rows))
	for _, row := range rows {
		outputRows = append(outputRows, contactRequestReviewAuditOutputRow{
			AuditID:        row.AuditID,
			TenantID:       row.TenantID,
			RequestID:      row.RequestID,
			PreviousStatus: row.PreviousStatus,
			NextStatus:     row.NextStatus,
			Decision:       row.Decision,
			Operator:       row.Operator,
			ReasonPresent:  row.ReasonPresent,
			SourceType:     row.SourceType,
			RiskLevel:      row.RiskLevel,
			ReviewRequired: row.ReviewRequired,
			ReviewedAt:     row.ReviewedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return writeJSONFile(path, contactRequestReviewAuditOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Count:       len(outputRows),
		Rows:        outputRows,
	})
}
