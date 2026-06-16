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

	updatedAfter, err := envOptionalRFC3339Time("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_UPDATED_AFTER")
	if err != nil {
		return err
	}
	updatedBefore, err := envOptionalRFC3339Time("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_UPDATED_BEFORE")
	if err != nil {
		return err
	}
	filters := map[string]string{
		"tenant_id":          envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_TENANT_ID", ""),
		"external_proof_ref": envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_EXTERNAL_PROOF_REF", ""),
		"status":             envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_STATUS", ""),
		"provider":           envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_PROVIDER", ""),
		"updated_after":      formatOptionalFilterTime(updatedAfter),
		"updated_before":     formatOptionalFilterTime(updatedBefore),
	}
	rows, err := repository.AuditComplianceExternalProofs(ctx, postgresinfra.MessageComplianceExternalProofAuditOptions{
		TenantID:         filters["tenant_id"],
		ExternalProofRef: filters["external_proof_ref"],
		Status:           filters["status"],
		Provider:         filters["provider"],
		UpdatedAfter:     updatedAfter,
		UpdatedBefore:    updatedBefore,
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
		if err := writeMessageComplianceProofAuditOutput(outputPath, rows, filters); err != nil {
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

	options, err := messageComplianceProofRegisterOptionsFromEnv()
	if err != nil {
		return err
	}
	result, err := repository.RegisterComplianceExternalProof(ctx, options)
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
