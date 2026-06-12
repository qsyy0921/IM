package token

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

const challengeDeliveryKeyVersion = "local-v1"

type ChallengeDeliveryTokenManager struct {
	aead       cipher.AEAD
	keyVersion string
}

func NewChallengeDeliveryTokenManager(secret string) (*ChallengeDeliveryTokenManager, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, types.NewChallengeDeliveryFailed("challenge delivery token encryption key is required")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, types.NewChallengeDeliveryFailed(err.Error())
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, types.NewChallengeDeliveryFailed(err.Error())
	}
	return &ChallengeDeliveryTokenManager{aead: aead, keyVersion: challengeDeliveryKeyVersion}, nil
}

func (manager *ChallengeDeliveryTokenManager) SealChallengeToken(token string) (types.EncryptedChallengeToken, error) {
	if manager == nil || manager.aead == nil {
		return types.EncryptedChallengeToken{}, types.NewChallengeDeliveryFailed("challenge delivery token encryption key is required")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return types.EncryptedChallengeToken{}, types.NewChallengeDeliveryFailed("challenge delivery token is empty")
	}
	nonce := make([]byte, manager.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return types.EncryptedChallengeToken{}, types.NewChallengeDeliveryFailed(err.Error())
	}
	ciphertext := manager.aead.Seal(nil, nonce, []byte(token), nil)
	return types.EncryptedChallengeToken{
		Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		KeyVersion: manager.keyVersion,
	}, nil
}

func (manager *ChallengeDeliveryTokenManager) OpenChallengeToken(encrypted types.EncryptedChallengeToken) (string, error) {
	if manager == nil || manager.aead == nil {
		return "", types.NewChallengeDeliveryFailed("challenge delivery token encryption key is required")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil {
		return "", types.NewChallengeDeliveryFailed("challenge delivery token ciphertext is invalid")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(encrypted.Nonce)
	if err != nil {
		return "", types.NewChallengeDeliveryFailed("challenge delivery token nonce is invalid")
	}
	if len(nonce) != manager.aead.NonceSize() {
		return "", types.NewChallengeDeliveryFailed("challenge delivery token nonce is invalid")
	}
	plain, err := manager.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", types.NewChallengeDeliveryFailed("challenge delivery token decrypt failed")
	}
	return string(plain), nil
}
