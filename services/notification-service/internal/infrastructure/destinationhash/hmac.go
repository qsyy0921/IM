package destinationhash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

type HMACHasher struct {
	key []byte
}

func NewHMACHasher(secret string) (*HMACHasher, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, types.NewDependencyFailed("notification destination hash key is not configured")
	}
	return &HMACHasher{key: []byte(secret)}, nil
}

func (hasher *HMACHasher) HashDestination(destinationRef string) (string, error) {
	if hasher == nil || len(hasher.key) == 0 {
		return "", types.NewDependencyFailed("notification destination hasher is not configured")
	}
	destinationRef = strings.TrimSpace(destinationRef)
	if destinationRef == "" {
		return "", types.NewInvalidArgument("destination_ref is required")
	}
	mac := hmac.New(sha256.New, hasher.key)
	_, _ = mac.Write([]byte(strings.ToLower(destinationRef)))
	return hex.EncodeToString(mac.Sum(nil)), nil
}
