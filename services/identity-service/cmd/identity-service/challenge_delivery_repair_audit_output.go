package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/postgres"
)

type challengeDeliveryRepairAuditOutput struct {
	GeneratedAt string                                  `json:"generated_at"`
	Filters     map[string]string                       `json:"filters,omitempty"`
	Rows        []challengeDeliveryRepairAuditOutputRow `json:"rows"`
}

type challengeDeliveryRepairAuditOutputRow struct {
	DeliveryID                      int64  `json:"delivery_id"`
	TenantID                        string `json:"tenant_id"`
	UserID                          string `json:"user_id"`
	ChallengeID                     string `json:"challenge_id"`
	Mode                            string `json:"mode"`
	Outcome                         string `json:"outcome"`
	SkipReason                      string `json:"skip_reason,omitempty"`
	Operator                        string `json:"operator,omitempty"`
	Reason                          string `json:"reason"`
	DryRun                          bool   `json:"dry_run"`
	PreviousDeliveryStatus          string `json:"previous_delivery_status"`
	PreviousChallengeStatus         string `json:"previous_challenge_status"`
	PreviousChallengeDeliveryStatus string `json:"previous_challenge_delivery_status"`
	PreviousRetryCount              int    `json:"previous_retry_count"`
	PreviousLastError               string `json:"previous_last_error,omitempty"`
	PreviousFailureClass            string `json:"previous_failure_class,omitempty"`
	PreviousDeadLetteredAt          string `json:"previous_dead_lettered_at,omitempty"`
	NewDeliveryStatus               string `json:"new_delivery_status"`
	NewChallengeStatus              string `json:"new_challenge_status"`
	NewChallengeDeliveryStatus      string `json:"new_challenge_delivery_status"`
	NewFailureClass                 string `json:"new_failure_class,omitempty"`
	RepairedAt                      string `json:"repaired_at"`
}

func writeChallengeDeliveryRepairAuditOutput(path string, rows []postgresinfra.ChallengeDeliveryRepairAuditRow, filters map[string]string) error {
	output := challengeDeliveryRepairAuditOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Filters:     compactOperatorCleanupFilters(filters),
		Rows:        make([]challengeDeliveryRepairAuditOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		outputRow := challengeDeliveryRepairAuditOutputRow{
			DeliveryID:                      row.DeliveryID,
			TenantID:                        row.TenantID,
			UserID:                          row.UserID,
			ChallengeID:                     row.ChallengeID,
			Mode:                            row.Mode,
			Outcome:                         row.Outcome,
			SkipReason:                      row.SkipReason,
			Operator:                        row.Operator,
			Reason:                          row.Reason,
			DryRun:                          row.DryRun,
			PreviousDeliveryStatus:          row.PreviousDeliveryStatus,
			PreviousChallengeStatus:         row.PreviousChallengeStatus,
			PreviousChallengeDeliveryStatus: row.PreviousChallengeDeliveryStatus,
			PreviousRetryCount:              row.PreviousRetryCount,
			PreviousLastError:               row.PreviousLastError,
			PreviousFailureClass:            row.PreviousFailureClass,
			NewDeliveryStatus:               row.NewDeliveryStatus,
			NewChallengeStatus:              row.NewChallengeStatus,
			NewChallengeDeliveryStatus:      row.NewChallengeDeliveryStatus,
			NewFailureClass:                 row.NewFailureClass,
			RepairedAt:                      row.RepairedAt.UTC().Format(time.RFC3339Nano),
		}
		if row.PreviousDeadLetteredAt != nil {
			outputRow.PreviousDeadLetteredAt = row.PreviousDeadLetteredAt.UTC().Format(time.RFC3339Nano)
		}
		output.Rows = append(output.Rows, outputRow)
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
