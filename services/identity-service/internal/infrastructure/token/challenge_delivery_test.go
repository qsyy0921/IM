package token

import (
	"errors"
	"strings"
	"testing"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestChallengeDeliveryTokenManagerEncryptsToken(t *testing.T) {
	manager, err := NewChallengeDeliveryTokenManager("delivery-secret")
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	encrypted, err := manager.SealChallengeToken("challenge-token")
	if err != nil {
		t.Fatalf("seal token: %v", err)
	}
	if encrypted.Ciphertext == "" || encrypted.Nonce == "" || encrypted.KeyVersion == "" {
		t.Fatalf("expected encrypted token fields, got %+v", encrypted)
	}
	if strings.Contains(encrypted.Ciphertext, "challenge-token") {
		t.Fatalf("ciphertext must not contain raw token: %+v", encrypted)
	}
	plain, err := manager.OpenChallengeToken(encrypted)
	if err != nil {
		t.Fatalf("open token: %v", err)
	}
	if plain != "challenge-token" {
		t.Fatalf("unexpected plain token %q", plain)
	}
}

func TestChallengeDeliveryTokenManagerKeyRingOpensOldKeyAndWritesCurrentKey(t *testing.T) {
	oldManager, err := NewChallengeDeliveryTokenManager("old-delivery-secret")
	if err != nil {
		t.Fatalf("new old manager: %v", err)
	}
	oldEncrypted, err := oldManager.SealChallengeToken("old-token")
	if err != nil {
		t.Fatalf("seal old token: %v", err)
	}
	keyRing, err := NewChallengeDeliveryTokenManagerWithKeyRing("v2", map[string]string{
		"local-v1": "old-delivery-secret",
		"v2":       "new-delivery-secret",
	})
	if err != nil {
		t.Fatalf("new keyring manager: %v", err)
	}
	openedOld, err := keyRing.OpenChallengeToken(oldEncrypted)
	if err != nil {
		t.Fatalf("open old token: %v", err)
	}
	if openedOld != "old-token" {
		t.Fatalf("unexpected old token %q", openedOld)
	}
	newEncrypted, err := keyRing.SealChallengeToken("new-token")
	if err != nil {
		t.Fatalf("seal new token: %v", err)
	}
	if newEncrypted.KeyVersion != "v2" {
		t.Fatalf("expected new key version v2, got %q", newEncrypted.KeyVersion)
	}
	openedNew, err := keyRing.OpenChallengeToken(newEncrypted)
	if err != nil {
		t.Fatalf("open new token: %v", err)
	}
	if openedNew != "new-token" {
		t.Fatalf("unexpected new token %q", openedNew)
	}
}

func TestChallengeDeliveryTokenManagerKeyRingRequiresCurrentKey(t *testing.T) {
	_, err := NewChallengeDeliveryTokenManagerWithKeyRing("v2", map[string]string{"local-v1": "old-delivery-secret"})
	if !errors.Is(err, types.ErrChallengeDeliveryFailed) {
		t.Fatalf("expected challenge delivery failed for missing current key, got %v", err)
	}
}

func TestChallengeDeliveryTokenManagerRequiresSecret(t *testing.T) {
	_, err := NewChallengeDeliveryTokenManager(" ")
	if !errors.Is(err, types.ErrChallengeDeliveryFailed) {
		t.Fatalf("expected challenge delivery failed for empty secret, got %v", err)
	}
}
