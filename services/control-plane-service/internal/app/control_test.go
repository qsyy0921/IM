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

func TestRollbackConfigVersionPreparesCommandHash(t *testing.T) {
	repository := &capturingRepository{}
	version, replayed, err := NewRollbackConfigVersionUseCase(repository, fixedEventID("evt_rollback")).Execute(
		context.Background(),
		validRollbackCommand(),
	)
	if err != nil {
		t.Fatalf("rollback config version: %v", err)
	}
	if replayed {
		t.Fatal("unexpected replay")
	}
	if repository.rollback.CommandHash == "" || repository.rollbackEventID != "evt_rollback" {
		t.Fatalf("rollback was not prepared: %+v event=%s", repository.rollback, repository.rollbackEventID)
	}
	if version.Version != "quota-v1" {
		t.Fatalf("version = %q", version.Version)
	}
}

func TestRollbackConfigVersionRejectsMissingTarget(t *testing.T) {
	command := validRollbackCommand()
	command.TargetVersion = ""
	if _, _, err := NewRollbackConfigVersionUseCase(fakeRepository{}, fixedEventID("evt_rollback")).Execute(
		context.Background(),
		command,
	); err == nil {
		t.Fatal("expected validation error")
	}
}

type fixedEventID string

func (id fixedEventID) NewEventID() (string, error) { return string(id), nil }

type fakeRepository struct{}

func (fakeRepository) PublishConfigVersion(context.Context, domain.PreparedConfigVersion, string) (types.ConfigVersion, error) {
	return types.ConfigVersion{}, nil
}
func (fakeRepository) RollbackConfigVersion(context.Context, domain.PreparedConfigRollback, string) (types.ConfigVersion, bool, error) {
	return types.ConfigVersion{}, false, nil
}
func (fakeRepository) GetConfigSnapshot(context.Context, types.GetConfigSnapshotCommand) (types.ConfigSnapshot, error) {
	return types.ConfigSnapshot{}, nil
}
func (fakeRepository) AckAppliedConfigVersion(context.Context, types.AckAppliedConfigVersionCommand, string) (types.AppliedConfigVersion, error) {
	return types.AppliedConfigVersion{}, nil
}

type capturingRepository struct {
	fakeRepository
	prepared        domain.PreparedConfigVersion
	rollback        domain.PreparedConfigRollback
	rollbackEventID string
}

func (repository *capturingRepository) PublishConfigVersion(
	_ context.Context,
	prepared domain.PreparedConfigVersion,
	_ string,
) (types.ConfigVersion, error) {
	repository.prepared = prepared
	return types.ConfigVersion{PayloadChecksum: prepared.PayloadChecksum}, nil
}

func (repository *capturingRepository) RollbackConfigVersion(
	_ context.Context,
	prepared domain.PreparedConfigRollback,
	eventID string,
) (types.ConfigVersion, bool, error) {
	repository.rollback = prepared
	repository.rollbackEventID = eventID
	return types.ConfigVersion{Version: prepared.Command.TargetVersion}, false, nil
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

func validRollbackCommand() types.RollbackConfigVersionCommand {
	return types.RollbackConfigVersionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-control-test",
			UserID:   "operator-1",
		},
		Environment:    "local",
		ConfigKind:     types.ConfigKindAPIGatewayTenantQuota,
		BundleKey:      "api-gateway/default",
		TargetVersion:  "quota-v1",
		IdempotencyKey: "rollback-idem-1",
		OperatorRef:    "operator:test",
		ApprovalRef:    "approval:test",
	}
}
