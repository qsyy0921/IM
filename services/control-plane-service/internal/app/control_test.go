package app

import (
	"context"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/control-plane-service/internal/domain"
	"github.com/qsyy0921/IM/services/control-plane-service/internal/types"
)

func TestPublishConfigVersionRejectsUnsafePayload(t *testing.T) {
	_, err := NewPublishConfigVersionUseCase(fakeRepository{}, fixedEventID("evt_1")).Execute(
		context.Background(),
		validPublishCommand(`{"api_token":"secret"}`),
	)
	if err == nil {
		t.Fatal("unsafe payload should fail")
	}
}

func TestPublishConfigVersionPreparesChecksum(t *testing.T) {
	repository := &capturingRepository{}
	version, err := NewPublishConfigVersionUseCase(repository, fixedEventID("evt_1")).Execute(
		context.Background(),
		validPublishCommand(`{"plans":{"tenant-free":{"requests_per_second":20,"burst":40}}}`),
	)
	if err != nil {
		t.Fatalf("publish config version: %v", err)
	}
	if repository.prepared.PayloadChecksum == "" || version.PayloadChecksum != repository.prepared.PayloadChecksum {
		t.Fatalf("expected prepared checksum to be persisted: version=%+v prepared=%+v", version, repository.prepared)
	}
}

type fixedEventID string

func (id fixedEventID) NewEventID() string { return string(id) }

type fakeRepository struct{}

func (fakeRepository) PublishConfigVersion(context.Context, domain.PreparedConfigVersion, string) (types.ConfigVersion, error) {
	return types.ConfigVersion{}, nil
}
func (fakeRepository) GetConfigSnapshot(context.Context, types.GetConfigSnapshotCommand) (types.ConfigSnapshot, error) {
	return types.ConfigSnapshot{}, nil
}
func (fakeRepository) AckAppliedConfigVersion(context.Context, types.AckAppliedConfigVersionCommand, string) (types.AppliedConfigVersion, error) {
	return types.AppliedConfigVersion{}, nil
}

type capturingRepository struct {
	fakeRepository
	prepared domain.PreparedConfigVersion
}

func (repository *capturingRepository) PublishConfigVersion(
	_ context.Context,
	prepared domain.PreparedConfigVersion,
	_ string,
) (types.ConfigVersion, error) {
	repository.prepared = prepared
	return types.ConfigVersion{PayloadChecksum: prepared.PayloadChecksum}, nil
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
