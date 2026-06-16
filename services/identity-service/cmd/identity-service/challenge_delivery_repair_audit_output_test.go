package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/postgres"
)

func TestWriteChallengeDeliveryRepairAuditOutput(t *testing.T) {
	deadLetteredAt := time.Date(2026, 6, 16, 9, 10, 0, 0, time.UTC)
	repairedAt := time.Date(2026, 6, 16, 9, 15, 0, 0, time.UTC)
	outputPath := filepath.Join(t.TempDir(), "identity-challenge-delivery-repair-audit.json")

	err := writeChallengeDeliveryRepairAuditOutput(outputPath, []postgresinfra.ChallengeDeliveryRepairAuditRow{
		{
			DeliveryID:                      42,
			TenantID:                        "tenant-a",
			UserID:                          "user-a",
			ChallengeID:                     "challenge-a",
			Mode:                            "redrive-active-pending",
			Outcome:                         "MUTATED",
			Operator:                        "local-operator",
			Reason:                          "provider recovered",
			DryRun:                          false,
			PreviousDeliveryStatus:          "DLQ",
			PreviousChallengeStatus:         "EXPIRED",
			PreviousChallengeDeliveryStatus: "FAILED",
			PreviousRetryCount:              5,
			PreviousLastError:               "challenge delivery unavailable",
			PreviousFailureClass:            "DELIVERY_FAILED",
			PreviousDeadLetteredAt:          &deadLetteredAt,
			NewDeliveryStatus:               "PENDING",
			NewChallengeStatus:              "ACTIVE",
			NewChallengeDeliveryStatus:      "PENDING",
			RepairedAt:                      repairedAt,
		},
	})
	if err != nil {
		t.Fatalf("write challenge delivery repair audit output: %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read challenge delivery repair audit output: %v", err)
	}
	var output struct {
		Rows []struct {
			DeliveryID                      int64  `json:"delivery_id"`
			TenantID                        string `json:"tenant_id"`
			UserID                          string `json:"user_id"`
			ChallengeID                     string `json:"challenge_id"`
			Mode                            string `json:"mode"`
			Outcome                         string `json:"outcome"`
			PreviousDeliveryStatus          string `json:"previous_delivery_status"`
			PreviousChallengeStatus         string `json:"previous_challenge_status"`
			PreviousChallengeDeliveryStatus string `json:"previous_challenge_delivery_status"`
			PreviousRetryCount              int    `json:"previous_retry_count"`
			PreviousLastError               string `json:"previous_last_error"`
			PreviousFailureClass            string `json:"previous_failure_class"`
			PreviousDeadLetteredAt          string `json:"previous_dead_lettered_at"`
			NewDeliveryStatus               string `json:"new_delivery_status"`
			NewChallengeStatus              string `json:"new_challenge_status"`
			NewChallengeDeliveryStatus      string `json:"new_challenge_delivery_status"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("decode challenge delivery repair audit output: %v", err)
	}
	if len(output.Rows) != 1 {
		t.Fatalf("unexpected row count: %+v", output)
	}
	row := output.Rows[0]
	if row.DeliveryID != 42 ||
		row.TenantID != "tenant-a" ||
		row.UserID != "user-a" ||
		row.ChallengeID != "challenge-a" ||
		row.Mode != "redrive-active-pending" ||
		row.Outcome != "MUTATED" ||
		row.PreviousDeliveryStatus != "DLQ" ||
		row.PreviousChallengeStatus != "EXPIRED" ||
		row.PreviousChallengeDeliveryStatus != "FAILED" ||
		row.PreviousRetryCount != 5 ||
		row.PreviousLastError != "challenge delivery unavailable" ||
		row.PreviousFailureClass != "DELIVERY_FAILED" ||
		row.PreviousDeadLetteredAt == "" ||
		row.NewDeliveryStatus != "PENDING" ||
		row.NewChallengeStatus != "ACTIVE" ||
		row.NewChallengeDeliveryStatus != "PENDING" {
		t.Fatalf("unexpected repair audit output row: %+v", row)
	}
}
