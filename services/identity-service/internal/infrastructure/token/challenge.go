package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type ChallengeTokenCodec struct{}

func NewChallengeTokenCodec() *ChallengeTokenCodec {
	return &ChallengeTokenCodec{}
}

func (codec *ChallengeTokenCodec) NewChallengeToken() (string, types.ChallengeRecord, error) {
	challengeID, err := randomURLToken(24)
	if err != nil {
		return "", types.ChallengeRecord{}, types.NewInvalidChallenge(err.Error())
	}
	token, err := randomURLToken(32)
	if err != nil {
		return "", types.ChallengeRecord{}, types.NewInvalidChallenge(err.Error())
	}
	return token, types.ChallengeRecord{
		ChallengeID: types.ChallengeID(challengeID),
		TokenHash:   codec.HashChallengeToken(token),
	}, nil
}

func (codec *ChallengeTokenCodec) HashChallengeToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomURLToken(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
