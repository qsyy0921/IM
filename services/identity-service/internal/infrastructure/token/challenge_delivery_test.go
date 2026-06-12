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

func TestChallengeDeliveryTokenManagerRequiresSecret(t *testing.T) {
	_, err := NewChallengeDeliveryTokenManager(" ")
	if !errors.Is(err, types.ErrChallengeDeliveryFailed) {
		t.Fatalf("expected challenge delivery failed for empty secret, got %v", err)
	}
}
