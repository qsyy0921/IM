package types

import (
	"errors"
	"testing"
	"time"
)

func TestSendMessageCommandValidationSupportsAttachmentMessages(t *testing.T) {
	for _, tc := range []struct {
		name          string
		messageType   MessageType
		attachmentIDs []string
	}{
		{name: "image", messageType: MessageTypeImage, attachmentIDs: []string{"image-attachment-1"}},
		{name: "file", messageType: MessageTypeFile, attachmentIDs: []string{"file-attachment-1"}},
		{name: "voice", messageType: MessageTypeVoice, attachmentIDs: []string{"voice-attachment-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := validSendMessageCommand()
			command.MessageType = tc.messageType
			command.AttachmentIDs = tc.attachmentIDs

			if err := command.Validate(); err != nil {
				t.Fatalf("validate %s message: %v", tc.messageType, err)
			}
		})
	}
}

func TestSendMessageCommandValidationSupportsPayloadOnlyMessages(t *testing.T) {
	for _, messageType := range []MessageType{MessageTypeLocation, MessageTypeCard} {
		t.Run(string(messageType), func(t *testing.T) {
			command := validSendMessageCommand()
			command.MessageType = messageType

			if err := command.Validate(); err != nil {
				t.Fatalf("validate %s message: %v", messageType, err)
			}
		})
	}
}

func TestSendMessageCommandValidationRejectsUnsupportedMessageType(t *testing.T) {
	command := validSendMessageCommand()
	command.MessageType = "STICKER"

	err := command.Validate()
	if !errors.Is(err, ErrUnsupportedMessageType) {
		t.Fatalf("expected unsupported message type, got %v", err)
	}
}

func TestSendMessageCommandValidationRequiresAttachmentForAttachmentMessages(t *testing.T) {
	for _, messageType := range []MessageType{MessageTypeImage, MessageTypeFile, MessageTypeVoice} {
		t.Run(string(messageType), func(t *testing.T) {
			command := validSendMessageCommand()
			command.MessageType = messageType
			command.AttachmentIDs = nil

			err := command.Validate()
			if err == nil || errors.Is(err, ErrUnsupportedMessageType) {
				t.Fatalf("expected attachment validation error, got %v", err)
			}
		})
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
