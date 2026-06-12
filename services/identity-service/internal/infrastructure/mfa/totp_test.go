package mfa

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestTOTPManagerRequiresEncryptionKey(t *testing.T) {
	_, err := NewTOTPManager(" ")
	if !errors.Is(err, types.ErrMFAUnavailable) {
		t.Fatalf("expected mfa unavailable for empty encryption key, got %v", err)
	}
}

func TestTOTPManagerEncryptsAndVerifiesCode(t *testing.T) {
	manager, err := NewTOTPManager("test-mfa-encryption-key")
	if err != nil {
		t.Fatalf("new totp manager: %v", err)
	}
	plain, encrypted, err := manager.NewTOTPSecret()
	if err != nil {
		t.Fatalf("new totp secret: %v", err)
	}
	if plain == "" || encrypted.Ciphertext == "" || encrypted.Nonce == "" || strings.Contains(encrypted.Ciphertext, plain) {
		t.Fatalf("unexpected secret material: plain=%q encrypted=%+v", plain, encrypted)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	code := generateTOTPCode(plain, now)
	ok, err := manager.VerifyTOTP(encrypted, code, now)
	if err != nil {
		t.Fatalf("verify totp: %v", err)
	}
	if !ok {
		t.Fatal("expected totp code to verify")
	}
	ok, err = manager.VerifyTOTP(encrypted, "000000", now)
	if err != nil {
		t.Fatalf("verify wrong totp: %v", err)
	}
	if ok {
		t.Fatal("wrong totp code should not verify")
	}
}

func TestTOTPManagerRejectsInvalidEncryptedSecret(t *testing.T) {
	manager, err := NewTOTPManager("test-mfa-encryption-key")
	if err != nil {
		t.Fatalf("new totp manager: %v", err)
	}
	_, err = manager.VerifyTOTP(types.EncryptedMFASecret{Ciphertext: "bad", Nonce: "bad"}, "123456", time.Now())
	if err == nil {
		t.Fatal("expected invalid encrypted secret to fail")
	}
}

func TestTOTPManagerOTPAuthURI(t *testing.T) {
	manager, err := NewTOTPManager("test-mfa-encryption-key")
	if err != nil {
		t.Fatalf("new totp manager: %v", err)
	}
	uri := manager.OTPAuthURI("NexusIM", "tenant:user", "SECRET")
	if !strings.HasPrefix(uri, "otpauth://totp/") || !strings.Contains(uri, "secret=SECRET") || !strings.Contains(uri, "issuer=NexusIM") {
		t.Fatalf("unexpected otpauth uri: %s", uri)
	}
}
