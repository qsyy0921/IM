package app

import (
	"context"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/presence-service/internal/domain"
	"github.com/qsyy0921/IM/services/presence-service/internal/types"
)

func TestGetPresenceMasksUnauthorizedTarget(t *testing.T) {
	repository := &fakeRepository{
		states: []types.PresenceState{
			{TenantID: "tenant-1", UserID: "user-other", ActualState: types.PresenceStateOnline, VisibleState: types.PresenceStateOnline, DeviceCount: 1},
		},
	}
	result, err := NewGetPresenceUseCase(repository).Execute(context.Background(), types.GetPresenceCommand{
		AuthContext:     types.AuthContext{TenantID: "tenant-1", UserID: "user-self"},
		RequesterUserID: "user-self",
		TargetUserIDs:   []string{"user-other"},
		IncludeDevices:  true,
	})
	if err != nil {
		t.Fatalf("get presence: %v", err)
	}
	if len(result) != 1 || result[0].VisibleState != types.PresenceStateUnknown || result[0].DeviceCount != 0 {
		t.Fatalf("unexpected visibility result: %+v", result)
	}
}

func TestUpdateTypingRejectsDraftLikeInputByContract(t *testing.T) {
	repository := &fakeRepository{}
	_, err := NewUpdateTypingUseCase(repository, fixedEventIDGenerator{}).Execute(context.Background(), types.UpdateTypingCommand{
		AuthContext:    types.AuthContext{TenantID: "tenant-1", UserID: "user-1"},
		ConversationID: "conversation-1",
		UserID:         "user-1",
		DeviceID:       "device-1",
		TypingState:    "hello draft text",
		TTL:            time.Second,
	})
	if err == nil {
		t.Fatal("typing_state only accepts STARTED/STOPPED, not draft text")
	}
}

type fakeRepository struct {
	states []types.PresenceState
}

func (repository *fakeRepository) UpdatePresence(
	context.Context,
	domain.PreparedPresenceUpdate,
	string,
) (types.PresenceState, error) {
	return types.PresenceState{}, nil
}

func (repository *fakeRepository) GetPresenceStates(
	context.Context,
	types.GetPresenceCommand,
) ([]types.PresenceState, error) {
	return repository.states, nil
}

func (repository *fakeRepository) UpdateTyping(
	context.Context,
	domain.PreparedTypingUpdate,
	string,
) (types.TypingIndicator, error) {
	return types.TypingIndicator{}, nil
}

type fixedEventIDGenerator struct{}

func (fixedEventIDGenerator) NewEventID() string {
	return "evt_presence_test"
}
