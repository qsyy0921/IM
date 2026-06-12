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
	aead       cipher.AEAD
	keyVersion string
}

func NewTOTPManager(secret string) (*TOTPManager, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, types.NewTokenSigningFailed("mfa secret encryption key is required")
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
	return &TOTPManager{aead: aead, keyVersion: defaultKeyVersion}, nil
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
	nonce := make([]byte, manager.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return types.EncryptedMFASecret{}, err
	}
	ciphertext := manager.aead.Seal(nil, nonce, []byte(plain), nil)
	return types.EncryptedMFASecret{
		Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		KeyVersion: manager.keyVersion,
	}, nil
}

func (manager *TOTPManager) decrypt(encrypted types.EncryptedMFASecret) (string, error) {
	ciphertext, err := base64.RawURLEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil {
		return "", types.NewInvalidMFA("mfa secret ciphertext is invalid")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(encrypted.Nonce)
	if err != nil {
		return "", types.NewInvalidMFA("mfa secret nonce is invalid")
	}
	if len(nonce) != manager.aead.NonceSize() {
		return "", types.NewInvalidMFA("mfa secret nonce is invalid")
	}
	plain, err := manager.aead.Open(nil, nonce, ciphertext, nil)
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
