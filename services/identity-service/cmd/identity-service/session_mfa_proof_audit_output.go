package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/postgres"
)

type sessionMFAProofAuditOutput struct {
	GeneratedAt          string `json:"generated_at"`
	InvalidTotal         int64  `json:"invalid_total"`
	UnknownMethod        int64  `json:"unknown_method"`
	EmptyMethodWithProof int64  `json:"empty_method_with_proof"`
	TOTPMissingProof     int64  `json:"totp_missing_proof"`
	RecoveryInvalidProof int64  `json:"recovery_invalid_proof"`
}

func writeSessionMFAProofAuditOutput(path string, stats postgresinfra.SessionMFAProofAuditStats) error {
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
	return encoder.Encode(sessionMFAProofAuditOutput{
		GeneratedAt:          time.Now().UTC().Format(time.RFC3339Nano),
		InvalidTotal:         stats.InvalidTotal,
		UnknownMethod:        stats.UnknownMethod,
		EmptyMethodWithProof: stats.EmptyMethodWithProof,
		TOTPMissingProof:     stats.TOTPMissingProof,
		RecoveryInvalidProof: stats.RecoveryInvalidProof,
	})
}
