package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func TestWriteContactRequestReviewOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "review", "result.json")
	result := types.ReviewContactRequestResult{
		RequestID:      "request-1",
		TenantID:       "tenant-1",
		SenderUserID:   "alice",
		ReceiverUserID: "bob",
		PreviousStatus: types.ContactRequestStatusReviewRequired,
		Status:         types.ContactRequestStatusPending,
		Decision:       types.ContactRequestReviewDecisionApprove,
		RiskLevel:      types.ContactRequestRiskLevelHigh,
		ReviewRequired: true,
	}
	if err := writeContactRequestReviewOutput(outputPath, result, "operator-1", true); err != nil {
		t.Fatalf("writeContactRequestReviewOutput() error = %v", err)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var output contactRequestReviewOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if output.RequestID != "request-1" ||
		output.PreviousStatus != "REVIEW_REQUIRED" ||
		output.Status != "PENDING" ||
		output.Decision != "APPROVE" ||
		output.RiskLevel != "HIGH" ||
		!output.ReviewRequired ||
		output.Operator != "operator-1" ||
		!output.ReasonPresent {
		t.Fatalf("unexpected contact request review output: %+v", output)
	}
	if strings.Contains(string(raw), "source risk rejected") {
		t.Fatalf("review output must not expose raw reason")
	}
}
