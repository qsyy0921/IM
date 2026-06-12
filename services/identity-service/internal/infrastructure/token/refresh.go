package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

const refreshTokenPrefix = "rft"

type RefreshTokenCodec struct{}

func NewRefreshTokenCodec() *RefreshTokenCodec {
	return &RefreshTokenCodec{}
}

func (codec *RefreshTokenCodec) NewRefreshToken() (string, types.RefreshTokenRecord, error) {
	tokenID, err := randomHex(16)
	if err != nil {
		return "", types.RefreshTokenRecord{}, err
	}
	secret, err := randomURLSafe(32)
	if err != nil {
		return "", types.RefreshTokenRecord{}, err
	}
	plain := refreshTokenPrefix + "_" + tokenID + "." + secret
	return plain, types.RefreshTokenRecord{
		TokenID:   types.RefreshTokenID(refreshTokenPrefix + "_" + tokenID),
		TokenHash: codec.HashRefreshTokenSecret(secret),
	}, nil
}

func (codec *RefreshTokenCodec) ParseRefreshToken(value string) (types.ParsedRefreshToken, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ".")
	if len(parts) != 2 || !strings.HasPrefix(parts[0], refreshTokenPrefix+"_") || parts[1] == "" {
		return types.ParsedRefreshToken{}, types.NewInvalidRefreshToken("invalid refresh token")
	}
	return types.ParsedRefreshToken{
		TokenID: types.RefreshTokenID(parts[0]),
		Secret:  parts[1],
	}, nil
}

func (codec *RefreshTokenCodec) HashRefreshTokenSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", types.NewTokenSigningFailed(err.Error())
	}
	return hex.EncodeToString(buf), nil
}

func randomURLSafe(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", types.NewTokenSigningFailed(err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
