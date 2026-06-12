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
	currentKeyVersion string
	keys              map[string]cipher.AEAD
}

func NewChallengeDeliveryTokenManager(secret string) (*ChallengeDeliveryTokenManager, error) {
	return NewChallengeDeliveryTokenManagerWithKeyRing(challengeDeliveryKeyVersion, map[string]string{challengeDeliveryKeyVersion: secret})
}

func NewChallengeDeliveryTokenManagerWithKeyRing(currentKeyVersion string, secrets map[string]string) (*ChallengeDeliveryTokenManager, error) {
	currentKeyVersion = strings.TrimSpace(currentKeyVersion)
	if currentKeyVersion == "" {
		currentKeyVersion = challengeDeliveryKeyVersion
	}
	keys := make(map[string]cipher.AEAD, len(secrets))
	for keyVersion, secret := range secrets {
		keyVersion = strings.TrimSpace(keyVersion)
		if keyVersion == "" {
			return nil, types.NewChallengeDeliveryFailed("challenge delivery token key version is required")
		}
		aead, err := newChallengeDeliveryTokenAEAD(secret)
		if err != nil {
			return nil, err
		}
		keys[keyVersion] = aead
	}
	if _, ok := keys[currentKeyVersion]; !ok {
		return nil, types.NewChallengeDeliveryFailed("current challenge delivery token key version is not configured")
	}
	return &ChallengeDeliveryTokenManager{keys: keys, currentKeyVersion: currentKeyVersion}, nil
}

func newChallengeDeliveryTokenAEAD(secret string) (cipher.AEAD, error) {
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
	return aead, nil
}

func (manager *ChallengeDeliveryTokenManager) SealChallengeToken(token string) (types.EncryptedChallengeToken, error) {
	if manager == nil || manager.keys == nil {
		return types.EncryptedChallengeToken{}, types.NewChallengeDeliveryFailed("challenge delivery token encryption key is required")
	}
	aead, ok := manager.keys[manager.currentKeyVersion]
	if !ok {
		return types.EncryptedChallengeToken{}, types.NewChallengeDeliveryFailed("current challenge delivery token key version is not configured")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return types.EncryptedChallengeToken{}, types.NewChallengeDeliveryFailed("challenge delivery token is empty")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return types.EncryptedChallengeToken{}, types.NewChallengeDeliveryFailed(err.Error())
	}
	ciphertext := aead.Seal(nil, nonce, []byte(token), nil)
	return types.EncryptedChallengeToken{
		Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		KeyVersion: manager.currentKeyVersion,
	}, nil
}

func (manager *ChallengeDeliveryTokenManager) OpenChallengeToken(encrypted types.EncryptedChallengeToken) (string, error) {
	if manager == nil || manager.keys == nil {
		return "", types.NewChallengeDeliveryFailed("challenge delivery token encryption key is required")
	}
	keyVersion := strings.TrimSpace(encrypted.KeyVersion)
	if keyVersion == "" {
		keyVersion = challengeDeliveryKeyVersion
	}
	aead, ok := manager.keys[keyVersion]
	if !ok {
		return "", types.NewChallengeDeliveryFailed("challenge delivery token key version is not configured")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil {
		return "", types.NewChallengeDeliveryFailed("challenge delivery token ciphertext is invalid")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(encrypted.Nonce)
	if err != nil {
		return "", types.NewChallengeDeliveryFailed("challenge delivery token nonce is invalid")
	}
	if len(nonce) != aead.NonceSize() {
		return "", types.NewChallengeDeliveryFailed("challenge delivery token nonce is invalid")
	}
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", types.NewChallengeDeliveryFailed("challenge delivery token decrypt failed")
	}
	return string(plain), nil
}
