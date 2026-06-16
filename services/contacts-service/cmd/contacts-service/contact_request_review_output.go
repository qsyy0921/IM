package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

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
	output := contactRequestReviewOutput{
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
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
