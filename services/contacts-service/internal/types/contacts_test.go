package types

import (
	"errors"
	"strings"
	"testing"
)

func TestSendContactRequestCommandSourceValidation(t *testing.T) {
	command := validSendContactRequestCommand()
	if err := command.Validate(); err != nil {
		t.Fatalf("expected default source to be valid: %v", err)
	}
	if command.NormalizedSourceType() != ContactRequestSourceTypeDirect {
		t.Fatalf("expected empty source type to normalize to DIRECT, got %q", command.NormalizedSourceType())
	}

	command = validSendContactRequestCommand()
	command.SourceType = ContactRequestSourceType("UNKNOWN")
	if err := command.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected invalid source type, got %v", err)
	}

	command = validSendContactRequestCommand()
	command.SourceRef = strings.Repeat("x", maxContactSourceRefLength+1)
	if err := command.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected too-long source_ref to be invalid, got %v", err)
	}

	command = validSendContactRequestCommand()
	command.SourceRef = "  invite:abc  "
	if command.NormalizedSourceRef() != "invite:abc" {
		t.Fatalf("expected source_ref trim, got %q", command.NormalizedSourceRef())
	}
}

func TestTenantContactRequestSourcePolicyCommandValidation(t *testing.T) {
	getCommand := GetTenantContactRequestSourcePolicyCommand{TenantID: "tenant-1"}
	if err := getCommand.Validate(); err != nil {
		t.Fatalf("expected default get source policy command to be valid: %v", err)
	}
	if getCommand.NormalizedSourceType() != ContactRequestSourceTypeDirect {
		t.Fatalf("expected default get source type to normalize to DIRECT, got %q", getCommand.NormalizedSourceType())
	}

	setCommand := SetTenantContactRequestSourcePolicyCommand{TenantID: "tenant-1"}
	if err := setCommand.Validate(); err != nil {
		t.Fatalf("expected default set source policy command to be valid: %v", err)
	}
	if setCommand.NormalizedSourceType() != ContactRequestSourceTypeDirect {
		t.Fatalf("expected default set source type to normalize to DIRECT, got %q", setCommand.NormalizedSourceType())
	}

	getCommand.SourceType = ContactRequestSourceType("UNKNOWN")
	if err := getCommand.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected invalid get source type, got %v", err)
	}

	setCommand.SourceType = ContactRequestSourceType("UNKNOWN")
	if err := setCommand.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected invalid set source type, got %v", err)
	}
}

func validSendContactRequestCommand() SendContactRequestCommand {
	return SendContactRequestCommand{
		AuthContext: AuthContext{
			TenantID: "tenant-1",
			UserID:   "alice",
		},
		TargetUserID:   "bob",
		IdempotencyKey: "send-1",
		Message:        "hello",
	}
}
