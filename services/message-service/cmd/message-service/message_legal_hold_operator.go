package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

func runMessageLegalHoldAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repository, closeRepository, err := messageRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closeRepository()

	rows, err := repository.AuditMessageLegalHolds(ctx, postgresinfra.MessageLegalHoldAuditOptions{
		TenantID:       envString("NEXUSIM_MESSAGE_LEGAL_HOLD_AUDIT_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_MESSAGE_LEGAL_HOLD_AUDIT_CONVERSATION_ID", ""),
		MessageID:      envString("NEXUSIM_MESSAGE_LEGAL_HOLD_AUDIT_MESSAGE_ID", ""),
		HoldID:         envString("NEXUSIM_MESSAGE_LEGAL_HOLD_AUDIT_HOLD_ID", ""),
		Status:         envString("NEXUSIM_MESSAGE_LEGAL_HOLD_AUDIT_STATUS", ""),
		Limit:          envInt("NEXUSIM_MESSAGE_LEGAL_HOLD_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("message-service legal hold audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"message_legal_hold tenant_id=%s conversation_id=%s message_id=%s hold_id=%s status=%s reason_present=%t created_by=%s released_by=%s updated_at=%s",
			row.TenantID,
			row.ConversationID,
			row.MessageID,
			row.HoldID,
			row.Status,
			row.ReasonPresent,
			row.CreatedBy,
			row.ReleasedBy,
			row.UpdatedAt.Format(time.RFC3339),
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_LEGAL_HOLD_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeMessageLegalHoldAuditOutput(outputPath, rows); err != nil {
			return err
		}
	}
	return nil
}

func runMessageLegalHoldSet() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repository, closeRepository, err := messageRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closeRepository()

	result, err := repository.SetMessageLegalHold(ctx, postgresinfra.MessageLegalHoldMutationOptions{
		TenantID:       envString("NEXUSIM_MESSAGE_LEGAL_HOLD_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_MESSAGE_LEGAL_HOLD_CONVERSATION_ID", ""),
		MessageID:      envString("NEXUSIM_MESSAGE_LEGAL_HOLD_MESSAGE_ID", ""),
		HoldID:         envString("NEXUSIM_MESSAGE_LEGAL_HOLD_ID", ""),
		OperatorID:     envString("NEXUSIM_MESSAGE_LEGAL_HOLD_OPERATOR_ID", ""),
		Reason:         envString("NEXUSIM_MESSAGE_LEGAL_HOLD_REASON", "manual legal hold"),
	})
	if err != nil {
		return err
	}
	log.Printf(
		"message-service legal hold set tenant_id=%s conversation_id=%s message_id=%s hold_id=%s status=%s",
		result.TenantID,
		result.ConversationID,
		result.MessageID,
		result.HoldID,
		result.Status,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_LEGAL_HOLD_OUTPUT")); outputPath != "" {
		if err := writeMessageLegalHoldMutationOutput(outputPath, result); err != nil {
			return err
		}
	}
	return nil
}

func runMessageLegalHoldRelease() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repository, closeRepository, err := messageRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closeRepository()

	result, err := repository.ReleaseMessageLegalHold(ctx, postgresinfra.MessageLegalHoldMutationOptions{
		TenantID:   envString("NEXUSIM_MESSAGE_LEGAL_HOLD_TENANT_ID", ""),
		HoldID:     envString("NEXUSIM_MESSAGE_LEGAL_HOLD_ID", ""),
		OperatorID: envString("NEXUSIM_MESSAGE_LEGAL_HOLD_OPERATOR_ID", ""),
	})
	if err != nil {
		return err
	}
	log.Printf(
		"message-service legal hold released tenant_id=%s conversation_id=%s message_id=%s hold_id=%s status=%s",
		result.TenantID,
		result.ConversationID,
		result.MessageID,
		result.HoldID,
		result.Status,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_LEGAL_HOLD_OUTPUT")); outputPath != "" {
		if err := writeMessageLegalHoldMutationOutput(outputPath, result); err != nil {
			return err
		}
	}
	return nil
}

func messageRepositoryFromEnv(ctx context.Context) (*postgresinfra.MessageRepository, func(), error) {
	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return nil, func() {}, errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return nil, func() {}, err
	}
	return postgresinfra.NewMessageRepository(pool), pool.Close, nil
}
