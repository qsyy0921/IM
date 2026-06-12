package credential

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

const (
	defaultIterations = 210_000
	defaultSaltBytes  = 16
	defaultKeyBytes   = 32
	hashPrefix        = "pbkdf2-sha256"
)

type PBKDF2Hasher struct {
	iterations int
	saltBytes  int
	keyBytes   int
}

func NewPBKDF2Hasher(iterations int) *PBKDF2Hasher {
	if iterations <= 0 {
		iterations = defaultIterations
	}
	return &PBKDF2Hasher{
		iterations: iterations,
		saltBytes:  defaultSaltBytes,
		keyBytes:   defaultKeyBytes,
	}
}

func (hasher *PBKDF2Hasher) HashPassword(password string) (string, error) {
	if hasher == nil {
		hasher = NewPBKDF2Hasher(0)
	}
	if strings.TrimSpace(password) == "" {
		return "", types.NewInvalidCredentials("password is required")
	}
	salt := make([]byte, hasher.saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", types.NewTokenSigningFailed(err.Error())
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, hasher.iterations, hasher.keyBytes)
	if err != nil {
		return "", types.NewTokenSigningFailed(err.Error())
	}
	return strings.Join([]string{
		hashPrefix,
		strconv.Itoa(hasher.iterations),
		base64.RawURLEncoding.EncodeToString(salt),
		base64.RawURLEncoding.EncodeToString(key),
	}, "$"), nil
}

func (hasher *PBKDF2Hasher) VerifyPassword(password string, passwordHash string) bool {
	if hasher == nil || strings.TrimSpace(password) == "" || strings.TrimSpace(passwordHash) == "" {
		return false
	}
	parts := strings.Split(passwordHash, "$")
	if len(parts) != 4 || parts[0] != hashPrefix {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return false
	}
	want, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
