package main

import (
	"fmt"
	"strings"

	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func validateSendAllow(cfg config, response *messagev1.SendMessageResponse, row messageRow) error {
	if response.GetMessageId() == "" || response.GetConversationSeq() <= 0 {
		return fmt.Errorf("allow returned invalid response message_id=%q seq=%d", response.GetMessageId(), response.GetConversationSeq())
	}
	if response.GetAcceptedAt() == nil {
		return fmt.Errorf("allow returned nil accepted_at")
	}
	if response.GetIdempotentReplay() {
		return fmt.Errorf("allow unexpectedly returned idempotent replay")
	}
	if row.MessageID != response.GetMessageId() || row.ConversationSeq != response.GetConversationSeq() {
		return fmt.Errorf("message row does not match response row=%+v response_message_id=%q response_seq=%d", row, response.GetMessageId(), response.GetConversationSeq())
	}
	if row.MessageStatus != "NORMAL" {
		return fmt.Errorf("message status=%q expected NORMAL", row.MessageStatus)
	}
	if !strings.Contains(row.MessagePayload, "policy integration smoke") {
		return fmt.Errorf("message payload does not contain smoke text: %s", row.MessagePayload)
	}
	if row.MessagePermissionVersion != cfg.expectedPermissionVer ||
		row.TimelinePermissionVersion != cfg.expectedPermissionVer {
		return fmt.Errorf("permission_version mismatch row=%+v expected=%d", row, cfg.expectedPermissionVer)
	}
	if row.MessageClassification != cfg.expectedClassification ||
		row.TimelineClassification != cfg.expectedClassification {
		return fmt.Errorf("classification mismatch row=%+v expected=%q", row, cfg.expectedClassification)
	}
	if row.OutboxStatus != "PENDING" && row.OutboxStatus != "PUBLISHED" {
		return fmt.Errorf("unexpected outbox status %q", row.OutboxStatus)
	}
	return nil
}

func validateBaseSend(cfg config, response *messagev1.SendMessageResponse, row messageRow) error {
	if row.MessageID == "" || row.ConversationSeq <= 0 {
		return fmt.Errorf("base SendMessage did not persist a message: %+v", row)
	}
	if response.GetMessageId() == "" || response.GetConversationSeq() <= 0 {
		return fmt.Errorf("base SendMessage returned invalid response message_id=%q seq=%d", response.GetMessageId(), response.GetConversationSeq())
	}
	if response.GetAcceptedAt() == nil {
		return fmt.Errorf("base SendMessage returned nil accepted_at")
	}
	if response.GetIdempotentReplay() {
		return fmt.Errorf("base SendMessage unexpectedly returned idempotent replay")
	}
	if row.MessageID != response.GetMessageId() || row.ConversationSeq != response.GetConversationSeq() {
		return fmt.Errorf("base SendMessage row does not match response row=%+v response_message_id=%q response_seq=%d", row, response.GetMessageId(), response.GetConversationSeq())
	}
	if row.MessageStatus != "NORMAL" {
		return fmt.Errorf("base SendMessage status=%q expected NORMAL", row.MessageStatus)
	}
	if !strings.Contains(row.MessagePayload, "policy integration smoke") {
		return fmt.Errorf("base SendMessage payload does not contain smoke text: %s", row.MessagePayload)
	}
	if row.MessagePermissionVersion != cfg.expectedPermissionVer || row.TimelinePermissionVersion != cfg.expectedPermissionVer {
		return fmt.Errorf("base SendMessage permission_version mismatch row=%+v expected=%d", row, cfg.expectedPermissionVer)
	}
	expectedClassification := expectedBaseSendClassification(cfg)
	if row.MessageClassification != expectedClassification || row.TimelineClassification != expectedClassification {
		return fmt.Errorf("base SendMessage classification mismatch row=%+v expected %s", row, expectedClassification)
	}
	if row.OutboxStatus != "PENDING" && row.OutboxStatus != "PUBLISHED" {
		return fmt.Errorf("unexpected base SendMessage outbox status %q", row.OutboxStatus)
	}
	return nil
}

func validateChangeAllow(
	cfg config,
	send *messagev1.SendMessageResponse,
	change *messagev1.MessageChangeResponse,
	row changeRow,
) error {
	if change.GetMessageId() != send.GetMessageId() {
		return fmt.Errorf("change message_id=%q expected %q", change.GetMessageId(), send.GetMessageId())
	}
	if change.GetConversationId() != cfg.conversationID {
		return fmt.Errorf("change conversation_id=%q expected %q", change.GetConversationId(), cfg.conversationID)
	}
	if change.GetConversationSeq() != send.GetConversationSeq()+1 {
		return fmt.Errorf("change seq=%d expected send seq + 1 (%d)", change.GetConversationSeq(), send.GetConversationSeq()+1)
	}
	if change.GetChangeVersion() <= 0 {
		return fmt.Errorf("change version must be positive, got %d", change.GetChangeVersion())
	}
	if change.GetAcceptedAt() == nil {
		return fmt.Errorf("change returned nil accepted_at")
	}
	if change.GetIdempotentReplay() {
		return fmt.Errorf("change unexpectedly returned idempotent replay")
	}
	expectedStatus := map[string]string{
		"edit":   "EDITED",
		"revoke": "REVOKED",
		"delete": "DELETED",
	}[cfg.action]
	expectedEventType := map[string]string{
		"edit":   "message.edited.v1",
		"revoke": "message.revoked.v1",
		"delete": "message.deleted.v1",
	}[cfg.action]
	if row.MessageStatus != expectedStatus {
		return fmt.Errorf("message status=%s expected %s", row.MessageStatus, expectedStatus)
	}
	if row.MessageID != send.GetMessageId() {
		return fmt.Errorf("change row message_id=%q expected %q", row.MessageID, send.GetMessageId())
	}
	if row.TimelineEventType != expectedEventType {
		return fmt.Errorf("timeline event_type=%s expected %s", row.TimelineEventType, expectedEventType)
	}
	if row.TimelinePermissionVersion != cfg.expectedPermissionVer {
		return fmt.Errorf("timeline permission_version=%d expected %d", row.TimelinePermissionVersion, cfg.expectedPermissionVer)
	}
	if row.TimelineClassification != cfg.expectedClassification {
		return fmt.Errorf("timeline classification=%q expected %q", row.TimelineClassification, cfg.expectedClassification)
	}
	if row.ChangeHistoryRows <= 0 {
		return fmt.Errorf("expected change history row, got %d", row.ChangeHistoryRows)
	}
	expectedChangeType := strings.ToUpper(cfg.action)
	if row.ChangeHistoryType != expectedChangeType {
		return fmt.Errorf("change_history type=%q expected %q", row.ChangeHistoryType, expectedChangeType)
	}
	if row.ChangeHistoryBeforeStatus != "NORMAL" || row.ChangeHistoryAfterStatus != expectedStatus {
		return fmt.Errorf("unexpected change_history statuses before=%q after=%q expected before=NORMAL after=%s", row.ChangeHistoryBeforeStatus, row.ChangeHistoryAfterStatus, expectedStatus)
	}
	if row.OutboxStatus != "PENDING" && row.OutboxStatus != "PUBLISHED" {
		return fmt.Errorf("unexpected change outbox status %q", row.OutboxStatus)
	}
	switch cfg.action {
	case "edit":
		if !row.EditedAtSet || row.RevokedAtSet || row.DeletedAtSet {
			return fmt.Errorf("unexpected edit timestamp flags edited=%v revoked=%v deleted=%v", row.EditedAtSet, row.RevokedAtSet, row.DeletedAtSet)
		}
		if !strings.Contains(row.MessagePayload, "policy integration smoke edited") {
			return fmt.Errorf("edited payload does not contain updated text: %s", row.MessagePayload)
		}
	case "revoke":
		if row.EditedAtSet || !row.RevokedAtSet || row.DeletedAtSet {
			return fmt.Errorf("unexpected revoke timestamp flags edited=%v revoked=%v deleted=%v", row.EditedAtSet, row.RevokedAtSet, row.DeletedAtSet)
		}
	case "delete":
		if row.EditedAtSet || row.RevokedAtSet || !row.DeletedAtSet {
			return fmt.Errorf("unexpected delete timestamp flags edited=%v revoked=%v deleted=%v", row.EditedAtSet, row.RevokedAtSet, row.DeletedAtSet)
		}
	}
	return nil
}

func validateDeny(err error) (errorSummary, error) {
	if err == nil {
		return errorSummary{}, fmt.Errorf("deny scenario unexpectedly succeeded")
	}
	st, ok := status.FromError(err)
	if !ok {
		return errorSummary{}, fmt.Errorf("deny returned non-grpc error: %w", err)
	}
	if st.Code() != codes.PermissionDenied {
		return errorSummary{}, fmt.Errorf("deny grpc code=%s, expected %s", st.Code(), codes.PermissionDenied)
	}
	var result errorSummary
	for _, detail := range st.Details() {
		if messageError, ok := detail.(*messagev1.MessageError); ok {
			if messageError.GetCode() != messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_PERMISSION_DENIED {
				return errorSummary{}, fmt.Errorf("deny message error code=%s", messageError.GetCode())
			}
			if messageError.GetRetryable() {
				return errorSummary{}, fmt.Errorf("deny message error unexpectedly retryable")
			}
			result = errorSummary{
				Code:      messageError.GetCode().String(),
				Message:   messageError.GetMessage(),
				Retryable: messageError.GetRetryable(),
			}
			break
		}
	}
	if result.Code == "" {
		return errorSummary{}, fmt.Errorf("deny response missing MessageError detail")
	}
	return result, nil
}

func validatePolicyAudit(cfg config, audit policyAudit) error {
	expectedAllowed := cfg.scenario == "allow"
	expectedClassification := cfg.expectedClassification
	expectedReasonCode := ""
	if !expectedAllowed {
		if cfg.seedConversationRole {
			expectedClassification = expectedRoleRule(cfg).Classification
		}
		expectedReasonCode = expectedClassification
	}
	if expectedRows := expectedPolicyAuditRows(cfg); expectedRows > 0 && audit.RowCount != expectedRows {
		return fmt.Errorf("policy audit row count=%d expected %d", audit.RowCount, expectedRows)
	}
	expectedAction := strings.ToUpper(cfg.action)
	if audit.Action != expectedAction {
		return fmt.Errorf("policy audit action=%q expected %q", audit.Action, expectedAction)
	}
	if cfg.action == "send" {
		if audit.MessageIDPresent || audit.MessageKeyPresent {
			return fmt.Errorf("policy audit send message context present=%v key_present=%v expected false", audit.MessageIDPresent, audit.MessageKeyPresent)
		}
	} else if !audit.MessageIDPresent || !audit.MessageKeyPresent {
		return fmt.Errorf("policy audit message context present=%v key_present=%v expected true", audit.MessageIDPresent, audit.MessageKeyPresent)
	}
	if audit.Allowed != expectedAllowed {
		return fmt.Errorf("policy audit allowed=%v expected %v", audit.Allowed, expectedAllowed)
	}
	if audit.PermissionVersion != cfg.expectedPermissionVer {
		return fmt.Errorf("policy audit permission_version=%d expected %d", audit.PermissionVersion, cfg.expectedPermissionVer)
	}
	if audit.Classification != expectedClassification {
		return fmt.Errorf("policy audit classification=%q expected %q", audit.Classification, expectedClassification)
	}
	if audit.ReasonCode != expectedReasonCode {
		return fmt.Errorf("policy audit reason_code=%q expected %q", audit.ReasonCode, expectedReasonCode)
	}
	if audit.Status != "PENDING" && audit.Status != "PUBLISHED" {
		return fmt.Errorf("policy audit status=%q expected PENDING or PUBLISHED", audit.Status)
	}
	return nil
}
