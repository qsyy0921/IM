package domain

import (
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/control-plane-service/internal/types"
)

func TestPrepareConfigVersionCanonicalizesPayloadAndChecksum(t *testing.T) {
	prepared, err := PrepareConfigVersion(validPublishCommand(`{"b":2,"a":1}`))
	if err != nil {
		t.Fatalf("prepare config version: %v", err)
	}
	if prepared.PayloadJSON != `{"a":1,"b":2}` {
		t.Fatalf("unexpected canonical payload %s", prepared.PayloadJSON)
	}
	if prepared.PayloadChecksum == "" || prepared.CommandHash == "" {
		t.Fatalf("expected hashes: %+v", prepared)
	}
}

func TestPrepareConfigVersionRejectsUnsafePayloadKeys(t *testing.T) {
	if _, err := PrepareConfigVersion(validPublishCommand(`{"provider_token":"secret"}`)); err == nil {
		t.Fatal("unsafe payload key should fail")
	}
}

func validPublishCommand(payload string) types.PublishConfigVersionCommand {
	return types.PublishConfigVersionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-control-test",
			UserID:   "operator-1",
		},
		Environment:    "local",
		ConfigKind:     types.ConfigKindAPIGatewayTenantQuota,
		BundleKey:      "api-gateway/default",
		Version:        "quota-v1",
		SchemaVersion:  "quota-v1",
		PayloadJSON:    payload,
		EffectiveAt:    time.Unix(1000, 0),
		IdempotencyKey: "idem-1",
	}
}
