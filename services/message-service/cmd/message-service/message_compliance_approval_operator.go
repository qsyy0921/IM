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

func runMessageComplianceApprovalAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repository, closeRepository, err := messageRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closeRepository()

	rows, err := repository.AuditComplianceDeleteApprovals(ctx, postgresinfra.MessageComplianceDeleteApprovalAuditOptions{
		TenantID:       envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_AUDIT_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_AUDIT_CONVERSATION_ID", ""),
		MessageID:      envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_AUDIT_MESSAGE_ID", ""),
		ApprovalID:     envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_AUDIT_APPROVAL_ID", ""),
		Status:         envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_AUDIT_STATUS", ""),
		Limit:          envInt("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("message-service compliance approval audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"message_compliance_delete_approval tenant_id=%s conversation_id=%s message_id=%s approval_id=%s status=%s external_proof_ref=%s reason_present=%t approved_by=%s consumed_by=%s canceled_by=%s updated_at=%s",
			row.TenantID,
			row.ConversationID,
			row.MessageID,
			row.ApprovalID,
			row.Status,
			row.ExternalProofRef,
			row.ReasonPresent,
			row.ApprovedBy,
			row.ConsumedBy,
			row.CanceledBy,
			row.UpdatedAt.Format(time.RFC3339),
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeMessageComplianceApprovalAuditOutput(outputPath, rows); err != nil {
			return err
		}
	}
	return nil
}

func runMessageComplianceApprovalCreate() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repository, closeRepository, err := messageRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closeRepository()

	result, err := repository.ApproveComplianceDelete(ctx, postgresinfra.MessageComplianceDeleteApprovalMutationOptions{
		TenantID:         envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_TENANT_ID", ""),
		ConversationID:   envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_CONVERSATION_ID", ""),
		MessageID:        envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_MESSAGE_ID", ""),
		ApprovalID:       envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_ID", ""),
		ExternalProofRef: envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_EXTERNAL_PROOF_REF", ""),
		OperatorID:       envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_OPERATOR_ID", ""),
		Reason:           envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_REASON", "manual compliance approval"),
	})
	if err != nil {
		return err
	}
	log.Printf(
		"message-service compliance approval created tenant_id=%s conversation_id=%s message_id=%s approval_id=%s status=%s external_proof_ref=%s",
		result.TenantID,
		result.ConversationID,
		result.MessageID,
		result.ApprovalID,
		result.Status,
		result.ExternalProofRef,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_OUTPUT")); outputPath != "" {
		if err := writeMessageComplianceApprovalMutationOutput(outputPath, result); err != nil {
			return err
		}
	}
	return nil
}

func runMessageComplianceApprovalCancel() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repository, closeRepository, err := messageRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closeRepository()

	result, err := repository.CancelComplianceDeleteApproval(ctx, postgresinfra.MessageComplianceDeleteApprovalMutationOptions{
		TenantID:   envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_TENANT_ID", ""),
		ApprovalID: envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_ID", ""),
		OperatorID: envString("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_OPERATOR_ID", ""),
	})
	if err != nil {
		return err
	}
	log.Printf(
		"message-service compliance approval canceled tenant_id=%s conversation_id=%s message_id=%s approval_id=%s status=%s",
		result.TenantID,
		result.ConversationID,
		result.MessageID,
		result.ApprovalID,
		result.Status,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_OUTPUT")); outputPath != "" {
		if err := writeMessageComplianceApprovalMutationOutput(outputPath, result); err != nil {
			return err
		}
	}
	return nil
}
