package mfa

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

const (
	defaultIssuer     = "NexusIM"
	totpPeriod        = int64(30)
	totpDigits        = 6
	totpWindow        = 1
	defaultKeyVersion = "local-v1"
)

type TOTPManager struct {
	currentKeyVersion string
	keys              map[string]cipher.AEAD
}

func NewTOTPManager(secret string) (*TOTPManager, error) {
	return NewTOTPManagerWithKeyRing(defaultKeyVersion, map[string]string{defaultKeyVersion: secret})
}

func NewTOTPManagerWithKeyRing(currentKeyVersion string, secrets map[string]string) (*TOTPManager, error) {
	currentKeyVersion = strings.TrimSpace(currentKeyVersion)
	if currentKeyVersion == "" {
		currentKeyVersion = defaultKeyVersion
	}
	keys := make(map[string]cipher.AEAD, len(secrets))
	for keyVersion, secret := range secrets {
		keyVersion = strings.TrimSpace(keyVersion)
		if keyVersion == "" {
			return nil, types.NewMFAUnavailable("mfa secret key version is required")
		}
		aead, err := newTOTPSecretAEAD(secret)
		if err != nil {
			return nil, err
		}
		keys[keyVersion] = aead
	}
	if _, ok := keys[currentKeyVersion]; !ok {
		return nil, types.NewMFAUnavailable("current mfa secret key version is not configured")
	}
	return &TOTPManager{keys: keys, currentKeyVersion: currentKeyVersion}, nil
}

func newTOTPSecretAEAD(secret string) (cipher.AEAD, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, types.NewMFAUnavailable("mfa secret encryption key is required")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead, nil
}

func (manager *TOTPManager) NewTOTPSecret() (string, types.EncryptedMFASecret, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", types.EncryptedMFASecret{}, err
	}
	plain := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	encrypted, err := manager.encrypt(plain)
	if err != nil {
		return "", types.EncryptedMFASecret{}, err
	}
	return plain, encrypted, nil
}

func (manager *TOTPManager) VerifyTOTP(encrypted types.EncryptedMFASecret, code string, now time.Time) (bool, error) {
	plain, err := manager.decrypt(encrypted)
	if err != nil {
		return false, err
	}
	code = strings.TrimSpace(code)
	for offset := -totpWindow; offset <= totpWindow; offset++ {
		if generateTOTPCode(plain, now.Add(time.Duration(offset)*time.Duration(totpPeriod)*time.Second)) == code {
			return true, nil
		}
	}
	return false, nil
}

func (manager *TOTPManager) OTPAuthURI(issuer string, accountName string, secret string) string {
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		issuer = defaultIssuer
	}
	accountName = strings.TrimSpace(accountName)
	label := url.PathEscape(fmt.Sprintf("%s:%s", issuer, accountName))
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", strconv.Itoa(totpDigits))
	query.Set("period", strconv.FormatInt(totpPeriod, 10))
	return "otpauth://totp/" + label + "?" + query.Encode()
}

func (manager *TOTPManager) encrypt(plain string) (types.EncryptedMFASecret, error) {
	aead, ok := manager.keys[manager.currentKeyVersion]
	if !ok {
		return types.EncryptedMFASecret{}, types.NewMFAUnavailable("current mfa secret key version is not configured")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return types.EncryptedMFASecret{}, err
	}
	ciphertext := aead.Seal(nil, nonce, []byte(plain), nil)
	return types.EncryptedMFASecret{
		Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		KeyVersion: manager.currentKeyVersion,
	}, nil
}

func (manager *TOTPManager) decrypt(encrypted types.EncryptedMFASecret) (string, error) {
	keyVersion := strings.TrimSpace(encrypted.KeyVersion)
	if keyVersion == "" {
		keyVersion = defaultKeyVersion
	}
	aead, ok := manager.keys[keyVersion]
	if !ok {
		return "", types.NewInvalidMFA("mfa secret key version is not configured")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil {
		return "", types.NewInvalidMFA("mfa secret ciphertext is invalid")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(encrypted.Nonce)
	if err != nil {
		return "", types.NewInvalidMFA("mfa secret nonce is invalid")
	}
	if len(nonce) != aead.NonceSize() {
		return "", types.NewInvalidMFA("mfa secret nonce is invalid")
	}
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", types.NewInvalidMFA("mfa secret decrypt failed")
	}
	return string(plain), nil
}

func generateTOTPCode(secret string, now time.Time) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return ""
	}
	counter := uint64(now.Unix() / totpPeriod)
	var counterBytes [8]byte
	binary.BigEndian.PutUint64(counterBytes[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(counterBytes[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (int(sum[offset])&0x7f)<<24 |
		(int(sum[offset+1])&0xff)<<16 |
		(int(sum[offset+2])&0xff)<<8 |
		(int(sum[offset+3]) & 0xff)
	code := value % 1_000_000
	return fmt.Sprintf("%06d", code)
}
