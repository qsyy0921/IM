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

func TestSendContactRequestCommandRejectsSensitiveSourceRef(t *testing.T) {
	tests := []string{
		"user@example.com",
		"+1 415 555 0101",
		"token=raw-invite-token",
		"bearer abc.def",
		"password:secret",
		"phone:13800138000",
		"safe-ref\nsecond-line",
	}

	for _, sourceRef := range tests {
		t.Run(sourceRef, func(t *testing.T) {
			command := validSendContactRequestCommand()
			command.SourceRef = sourceRef
			if err := command.Validate(); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("expected sensitive source_ref to be invalid, got %v", err)
			}
		})
	}
}

func TestSendContactRequestCommandAllowsLowSensitivitySourceRef(t *testing.T) {
	for _, sourceRef := range []string{
		"invite:abc",
		"group/project-42",
		"qr_code:campaign-2026-06",
		"import:batch_001",
	} {
		t.Run(sourceRef, func(t *testing.T) {
			command := validSendContactRequestCommand()
			command.SourceRef = sourceRef
			if err := command.Validate(); err != nil {
				t.Fatalf("expected low-sensitivity source_ref to be valid: %v", err)
			}
		})
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

func TestNormalizeContactProfileVisibilityFields(t *testing.T) {
	fields, err := NormalizeContactProfileVisibilityFields([]ContactProfileVisibilityField{
		ContactProfileVisibilityField("display-name"),
		ContactProfileVisibilityFieldDisplayName,
		ContactProfileVisibilityFieldStatusMessage,
	})
	if err != nil {
		t.Fatalf("normalize profile visibility fields: %v", err)
	}
	if len(fields) != 2 ||
		fields[0] != ContactProfileVisibilityFieldDisplayName ||
		fields[1] != ContactProfileVisibilityFieldStatusMessage {
		t.Fatalf("unexpected normalized fields: %+v", fields)
	}

	if _, err := NormalizeContactProfileVisibilityFields([]ContactProfileVisibilityField{""}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected empty explicit profile visibility field to be invalid, got %v", err)
	}
	if _, err := NormalizeContactProfileVisibilityFields([]ContactProfileVisibilityField{"EMAIL"}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected unsupported profile visibility field to be invalid, got %v", err)
	}
}

func TestSetContactPrivacyExceptionCommandValidation(t *testing.T) {
	command := SetContactPrivacyExceptionCommand{
		AuthContext: AuthContext{
			TenantID: "tenant-1",
			UserID:   "bob",
		},
		OtherUserID:    "alice",
		Decision:       ContactPrivacyExceptionDecision("deny"),
		IdempotencyKey: "privacy-exception-1",
	}
	if err := command.Validate(); err != nil {
		t.Fatalf("expected privacy exception command to be valid: %v", err)
	}
	if NormalizeContactPrivacyExceptionDecision(command.Decision) != ContactPrivacyExceptionDecisionDeny {
		t.Fatalf("expected decision to normalize to DENY")
	}
	command.Decision = "BLOCK"
	if err := command.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected invalid decision, got %v", err)
	}
	command = SetContactPrivacyExceptionCommand{
		AuthContext: AuthContext{
			TenantID: "tenant-1",
			UserID:   "bob",
		},
		OtherUserID:    "bob",
		Decision:       ContactPrivacyExceptionDecisionAllow,
		IdempotencyKey: "privacy-exception-2",
	}
	if err := command.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected self exception to be invalid, got %v", err)
	}
}

func TestPrivacyExceptionManagementCommandValidation(t *testing.T) {
	listCommand := ListContactPrivacyExceptionsCommand{
		AuthContext: AuthContext{
			TenantID: "tenant-1",
			UserID:   "bob",
		},
		PageSize: 10,
	}
	if err := listCommand.Validate(); err != nil {
		t.Fatalf("expected list privacy exceptions command to be valid: %v", err)
	}
	listCommand.PageSize = -1
	if err := listCommand.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected negative page size to be invalid, got %v", err)
	}

	deleteCommand := DeleteContactPrivacyExceptionCommand{
		AuthContext: AuthContext{
			TenantID: "tenant-1",
			UserID:   "bob",
		},
		OtherUserID:    "alice",
		IdempotencyKey: "privacy-exception-delete-1",
	}
	if err := deleteCommand.Validate(); err != nil {
		t.Fatalf("expected delete privacy exception command to be valid: %v", err)
	}
	deleteCommand.OtherUserID = "bob"
	if err := deleteCommand.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected self privacy exception delete to be invalid, got %v", err)
	}
	deleteCommand.OtherUserID = "alice"
	deleteCommand.IdempotencyKey = " "
	if err := deleteCommand.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected blank idempotency key to be invalid, got %v", err)
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
