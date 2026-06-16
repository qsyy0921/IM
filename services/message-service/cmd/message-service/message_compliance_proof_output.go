package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

type messageComplianceProofOutput struct {
	GeneratedAt string                            `json:"generated_at"`
	Rows        []messageComplianceProofOutputRow `json:"rows"`
}

type messageComplianceProofMutationOutput struct {
	GeneratedAt string                          `json:"generated_at"`
	Row         messageComplianceProofOutputRow `json:"row"`
}

type messageComplianceProofOutputRow struct {
	TenantID         string `json:"tenant_id"`
	ExternalProofRef string `json:"external_proof_ref"`
	Status           string `json:"status"`
	Provider         string `json:"provider"`
	ProofHash        string `json:"proof_hash"`
	VerifiedBy       string `json:"verified_by"`
	VerifiedAt       string `json:"verified_at"`
	RevokedBy        string `json:"revoked_by,omitempty"`
	RevokedAt        string `json:"revoked_at,omitempty"`
	UpdatedAt        string `json:"updated_at"`
}

func writeMessageComplianceProofAuditOutput(path string, rows []postgresinfra.MessageComplianceExternalProofAuditRow) error {
	output := messageComplianceProofOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Rows:        make([]messageComplianceProofOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, messageComplianceProofOutputRowFromResult(row))
	}
	return writeMessageComplianceProofJSON(path, output)
}

func writeMessageComplianceProofMutationOutput(path string, row postgresinfra.MessageComplianceExternalProofResult) error {
	output := messageComplianceProofMutationOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Row:         messageComplianceProofOutputRowFromResult(row),
	}
	return writeMessageComplianceProofJSON(path, output)
}

func messageComplianceProofOutputRowFromResult(row postgresinfra.MessageComplianceExternalProofResult) messageComplianceProofOutputRow {
	return messageComplianceProofOutputRow{
		TenantID:         row.TenantID,
		ExternalProofRef: row.ExternalProofRef,
		Status:           row.Status,
		Provider:         row.Provider,
		ProofHash:        row.ProofHash,
		VerifiedBy:       row.VerifiedBy,
		VerifiedAt:       row.VerifiedAt.UTC().Format(time.RFC3339Nano),
		RevokedBy:        row.RevokedBy,
		RevokedAt:        formatOptionalTimeRFC3339Nano(row.RevokedAt),
		UpdatedAt:        row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func writeMessageComplianceProofJSON(path string, output any) error {
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
