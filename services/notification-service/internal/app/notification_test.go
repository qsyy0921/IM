package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

func TestCreateNotificationRequestHashesDestinationBeforeRepository(t *testing.T) {
	repository := &fakeRepository{}
	useCase := NewCreateNotificationRequestUseCase(repository, fakeHasher{hash: "hash-destination"}, fakeRequestIDs{id: "notif-test"})
	result, err := useCase.Execute(context.Background(), createCommand())
	if err != nil {
		t.Fatalf("create notification request: %v", err)
	}
	if result.RequestID != "notif-test" {
		t.Fatalf("unexpected request id: %+v", result)
	}
	if repository.destinationHash != "hash-destination" {
		t.Fatalf("destination hash not passed to repository: %q", repository.destinationHash)
	}
	if repository.command.DestinationRef != "user@example.com" {
		t.Fatalf("command should stay available to app layer before repository boundary: %+v", repository.command)
	}
}

func TestCreateNotificationRequestRequiresHasher(t *testing.T) {
	useCase := NewCreateNotificationRequestUseCase(&fakeRepository{}, nil, fakeRequestIDs{id: "notif-test"})
	if _, err := useCase.Execute(context.Background(), createCommand()); !errors.Is(err, types.ErrDependencyFailed) {
		t.Fatalf("expected dependency failed, got %v", err)
	}
}

func createCommand() types.CreateNotificationRequestCommand {
	return types.CreateNotificationRequestCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		RequesterService:      "identity-service",
		Channel:               types.ChannelEmail,
		RecipientRef:          "user:user-1",
		DestinationRef:        "user@example.com",
		DestinationMasked:     "u***@example.com",
		TemplateKey:           "verify-email",
		TemplateVersion:       "v1",
		IdempotencyKey:        "idem-1",
		TemplateVariablesJSON: "{}",
	}
}

type fakeHasher struct {
	hash string
}

func (hasher fakeHasher) HashDestination(string) (string, error) {
	return hasher.hash, nil
}

type fakeRequestIDs struct {
	id string
}

func (ids fakeRequestIDs) NewRequestID() (string, error) {
	return ids.id, nil
}

type fakeRepository struct {
	command         types.CreateNotificationRequestCommand
	requestID       string
	destinationHash string
	commandHash     string
}

func (repository *fakeRepository) CreateNotificationRequest(
	_ context.Context,
	command types.CreateNotificationRequestCommand,
	requestID string,
	destinationHash string,
	commandHash string,
) (types.NotificationRequest, error) {
	repository.command = command
	repository.requestID = requestID
	repository.destinationHash = destinationHash
	repository.commandHash = commandHash
	return types.NotificationRequest{
		TenantID:          command.AuthContext.TenantID,
		RequestID:         requestID,
		RequesterService:  command.RequesterService,
		Channel:           command.Channel,
		RecipientRef:      command.RecipientRef,
		DestinationHash:   destinationHash,
		DestinationMasked: command.DestinationMasked,
		TemplateKey:       command.TemplateKey,
		TemplateVersion:   command.TemplateVersion,
		Locale:            command.Locale,
		Priority:          command.Priority,
		Status:            types.StatusAccepted,
	}, nil
}

func (repository *fakeRepository) GetNotificationRequest(
	context.Context,
	types.TenantID,
	string,
) (types.NotificationRequest, error) {
	return types.NotificationRequest{}, nil
}

func (repository *fakeRepository) CancelNotificationRequest(
	context.Context,
	types.CancelNotificationRequestCommand,
) (types.NotificationRequest, error) {
	return types.NotificationRequest{}, nil
}
