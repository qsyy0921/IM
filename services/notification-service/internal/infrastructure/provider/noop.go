package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

type NoopProvider struct {
	providerID string
}

func NewNoopProvider(providerID string) NoopProvider {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		providerID = "local-noop"
	}
	return NoopProvider{providerID: providerID}
}

func (provider NoopProvider) Send(_ context.Context, request types.DeliveryRequest) (types.DeliveryResult, error) {
	messageID := provider.providerID + ":" + string(request.TenantID) + ":" + request.RequestID + ":" + request.ProviderIdempotencyKey
	digest := sha256.Sum256([]byte(messageID))
	return types.DeliveryResult{
		ProviderID:            provider.providerID,
		ProviderMessageIDHash: hex.EncodeToString(digest[:]),
	}, nil
}
