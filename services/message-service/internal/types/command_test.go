package types

import (
	"errors"
	"testing"
	"time"
)

func TestSendMessageCommandValidationSupportsAttachmentMessages(t *testing.T) {
	command := validSendMessageCommand()
	command.MessageType = MessageTypeImage
	command.AttachmentIDs = []string{"image-attachment-1"}

	if err := command.Validate(); err != nil {
		t.Fatalf("validate image message: %v", err)
	}

	command.MessageType = MessageTypeFile
	command.AttachmentIDs = []string{"file-attachment-1"}
	if err := command.Validate(); err != nil {
		t.Fatalf("validate file message: %v", err)
	}
}

func TestSendMessageCommandValidationRejectsUnsupportedMessageType(t *testing.T) {
	command := validSendMessageCommand()
	command.MessageType = "VOICE"

	err := command.Validate()
	if !errors.Is(err, ErrUnsupportedMessageType) {
		t.Fatalf("expected unsupported message type, got %v", err)
	}
}

func TestSendMessageCommandValidationRequiresAttachmentForAttachmentMessages(t *testing.T) {
	command := validSendMessageCommand()
	command.MessageType = MessageTypeFile
	command.AttachmentIDs = nil

	err := command.Validate()
	if err == nil || errors.Is(err, ErrUnsupportedMessageType) {
		t.Fatalf("expected attachment validation error, got %v", err)
	}
}

func validSendMessageCommand() SendMessageCommand {
	return SendMessageCommand{
		AuthContext: AuthContext{
			TenantID:  "tenant-1",
			UserID:    "user-1",
			DeviceID:  "device-1",
			SessionID: "session-1",
		},
		ConversationID: "conv-1",
		ClientMsgID:    "client-1",
		MessageType:    MessageTypeText,
		PayloadJSON:    []byte(`{"text":"hello"}`),
		ReceivedAt:     time.Unix(100, 0).UTC(),
	}
}
