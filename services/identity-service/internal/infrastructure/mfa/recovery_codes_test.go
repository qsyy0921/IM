package mfa

import (
	"strings"
	"testing"
)

func TestRecoveryCodeManagerGeneratesHashedCodes(t *testing.T) {
	manager, err := NewRecoveryCodeManager("recovery-secret")
	if err != nil {
		t.Fatalf("new recovery manager: %v", err)
	}
	codes, err := manager.NewRecoveryCodes(2)
	if err != nil {
		t.Fatalf("new recovery codes: %v", err)
	}
	if len(codes) != 2 {
		t.Fatalf("expected 2 codes, got %d", len(codes))
	}
	seen := map[string]struct{}{}
	for _, code := range codes {
		if code.CodeID == "" || code.Code == "" || code.CodeHash == "" {
			t.Fatalf("expected id/code/hash, got %+v", code)
		}
		if strings.Contains(code.CodeHash, code.Code) {
			t.Fatalf("hash must not contain plaintext code: %+v", code)
		}
		if _, ok := seen[code.Code]; ok {
			t.Fatalf("duplicate recovery code generated: %s", code.Code)
		}
		seen[code.Code] = struct{}{}
	}
}

func TestRecoveryCodeManagerNormalizesCodeForHash(t *testing.T) {
	manager, err := NewRecoveryCodeManager("recovery-secret")
	if err != nil {
		t.Fatalf("new recovery manager: %v", err)
	}
	withDashes, err := manager.HashRecoveryCode("ABCD-EFGH-IJKL-MNOP")
	if err != nil {
		t.Fatalf("hash recovery code: %v", err)
	}
	withSpaces, err := manager.HashRecoveryCode("abcd efgh ijkl mnop")
	if err != nil {
		t.Fatalf("hash recovery code with spaces: %v", err)
	}
	if withDashes != withSpaces {
		t.Fatalf("expected normalized hashes to match: %q != %q", withDashes, withSpaces)
	}
}

func TestRecoveryCodeManagerRequiresSecret(t *testing.T) {
	if _, err := NewRecoveryCodeManager(""); err == nil {
		t.Fatal("expected empty secret to fail")
	}
}
