package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	postgresinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/postgres"
)

func TestWriteSessionMFAProofAuditOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "nested", "session-mfa-proof-audit.json")
	stats := postgresinfra.SessionMFAProofAuditStats{
		InvalidTotal:         10,
		UnknownMethod:        1,
		EmptyMethodWithProof: 2,
		TOTPMissingProof:     3,
		RecoveryInvalidProof: 4,
	}

	if err := writeSessionMFAProofAuditOutput(outputPath, stats); err != nil {
		t.Fatalf("writeSessionMFAProofAuditOutput() error = %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var output sessionMFAProofAuditOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if output.GeneratedAt == "" {
		t.Fatal("generated_at is empty")
	}
	if output.InvalidTotal != stats.InvalidTotal ||
		output.UnknownMethod != stats.UnknownMethod ||
		output.EmptyMethodWithProof != stats.EmptyMethodWithProof ||
		output.TOTPMissingProof != stats.TOTPMissingProof ||
		output.RecoveryInvalidProof != stats.RecoveryInvalidProof {
		t.Fatalf("unexpected output: %+v", output)
	}
}
