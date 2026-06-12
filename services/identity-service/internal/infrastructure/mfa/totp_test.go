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

func TestTOTPManagerKeyRingVerifiesOldKeyAndWritesCurrentKey(t *testing.T) {
	oldManager, err := NewTOTPManager("old-mfa-encryption-key")
	if err != nil {
		t.Fatalf("new old totp manager: %v", err)
	}
	oldPlain, oldEncrypted, err := oldManager.NewTOTPSecret()
	if err != nil {
		t.Fatalf("new old totp secret: %v", err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	keyRing, err := NewTOTPManagerWithKeyRing("v2", map[string]string{
		"local-v1": "old-mfa-encryption-key",
		"v2":       "new-mfa-encryption-key",
	})
	if err != nil {
		t.Fatalf("new keyring totp manager: %v", err)
	}

	oldOK, err := keyRing.VerifyTOTP(oldEncrypted, generateTOTPCode(oldPlain, now), now)
	if err != nil {
		t.Fatalf("verify old encrypted totp: %v", err)
	}
	if !oldOK {
		t.Fatal("expected old key version to verify")
	}
	newPlain, newEncrypted, err := keyRing.NewTOTPSecret()
	if err != nil {
		t.Fatalf("new keyring totp secret: %v", err)
	}
	if newEncrypted.KeyVersion != "v2" {
		t.Fatalf("expected new secret key version v2, got %q", newEncrypted.KeyVersion)
	}
	newOK, err := keyRing.VerifyTOTP(newEncrypted, generateTOTPCode(newPlain, now), now)
	if err != nil {
		t.Fatalf("verify new encrypted totp: %v", err)
	}
	if !newOK {
		t.Fatal("expected current key version to verify")
	}
}

func TestTOTPManagerKeyRingRequiresCurrentKey(t *testing.T) {
	_, err := NewTOTPManagerWithKeyRing("v2", map[string]string{"local-v1": "old-mfa-encryption-key"})
	if !errors.Is(err, types.ErrMFAUnavailable) {
		t.Fatalf("expected mfa unavailable for missing current key, got %v", err)
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
