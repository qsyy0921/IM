package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/postgres"
)

func TestWriteContactRequestReviewAuditOutputOmitsReasonText(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "review-audit.json")
	rows := []postgresinfra.ContactRequestReviewAuditRow{{
		AuditID:        7,
		TenantID:       "tenant-contacts",
		RequestID:      "request-1",
		PreviousStatus: "REVIEW_REQUIRED",
		NextStatus:     "PENDING",
		Decision:       "APPROVE",
		Operator:       "operator-1",
		ReasonPresent:  true,
		SourceType:     "QR_CODE",
		RiskLevel:      "HIGH",
		ReviewRequired: true,
		ReviewedAt:     time.Unix(100, 0).UTC(),
	}}

	if err := writeContactRequestReviewAuditOutput(outputPath, rows); err != nil {
		t.Fatalf("write review audit output: %v", err)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read review audit output: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		`"count": 1`,
		`"request_id": "request-1"`,
		`"reason_present": true`,
		`"source_type": "QR_CODE"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected output to contain %s, got %s", want, text)
		}
	}
	if strings.Contains(text, `"reason":`) || strings.Contains(text, "internal source risk reviewed") {
		t.Fatalf("expected output to omit review reason field, got %s", text)
	}
}
