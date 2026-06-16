package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

func runMessageComplianceProofAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repository, closeRepository, err := messageRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closeRepository()

	rows, err := repository.AuditComplianceExternalProofs(ctx, postgresinfra.MessageComplianceExternalProofAuditOptions{
		TenantID:         envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_TENANT_ID", ""),
		ExternalProofRef: envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_EXTERNAL_PROOF_REF", ""),
		Status:           envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_STATUS", ""),
		Provider:         envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_PROVIDER", ""),
		Limit:            envInt("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("message-service compliance proof audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"message_compliance_external_proof tenant_id=%s external_proof_ref=%s status=%s provider=%s proof_hash=%s verified_by=%s revoked_by=%s updated_at=%s",
			row.TenantID,
			row.ExternalProofRef,
			row.Status,
			row.Provider,
			row.ProofHash,
			row.VerifiedBy,
			row.RevokedBy,
			row.UpdatedAt.Format(time.RFC3339),
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeMessageComplianceProofAuditOutput(outputPath, rows); err != nil {
			return err
		}
	}
	return nil
}

func runMessageComplianceProofRegister() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repository, closeRepository, err := messageRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closeRepository()

	result, err := repository.RegisterComplianceExternalProof(ctx, postgresinfra.MessageComplianceExternalProofMutationOptions{
		TenantID:         envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_TENANT_ID", ""),
		ExternalProofRef: envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_EXTERNAL_PROOF_REF", ""),
		Provider:         envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_PROVIDER", ""),
		ProofHash:        envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_HASH", ""),
		OperatorID:       envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_OPERATOR_ID", ""),
	})
	if err != nil {
		return err
	}
	log.Printf(
		"message-service compliance proof registered tenant_id=%s external_proof_ref=%s status=%s provider=%s proof_hash=%s",
		result.TenantID,
		result.ExternalProofRef,
		result.Status,
		result.Provider,
		result.ProofHash,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_OUTPUT")); outputPath != "" {
		if err := writeMessageComplianceProofMutationOutput(outputPath, result); err != nil {
			return err
		}
	}
	return nil
}

func runMessageComplianceProofRevoke() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repository, closeRepository, err := messageRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closeRepository()

	result, err := repository.RevokeComplianceExternalProof(ctx, postgresinfra.MessageComplianceExternalProofMutationOptions{
		TenantID:         envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_TENANT_ID", ""),
		ExternalProofRef: envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_EXTERNAL_PROOF_REF", ""),
		OperatorID:       envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_OPERATOR_ID", ""),
	})
	if err != nil {
		return err
	}
	log.Printf(
		"message-service compliance proof revoked tenant_id=%s external_proof_ref=%s status=%s",
		result.TenantID,
		result.ExternalProofRef,
		result.Status,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_OUTPUT")); outputPath != "" {
		if err := writeMessageComplianceProofMutationOutput(outputPath, result); err != nil {
			return err
		}
	}
	return nil
}
