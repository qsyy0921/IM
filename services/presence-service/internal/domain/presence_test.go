package domain

import (
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/presence-service/internal/types"
)

func TestPreparePresenceUpdateRejectsMismatchedUser(t *testing.T) {
	_, err := PreparePresenceUpdate(types.UpdatePresenceCommand{
		AuthContext:    types.AuthContext{TenantID: "tenant-1", UserID: "user-a"},
		UserID:         "user-b",
		DeviceID:       "device-1",
		SessionID:      "session-1",
		PresenceState:  types.PresenceStateOnline,
		Source:         types.SourceClient,
		TTL:            time.Minute,
		IdempotencyKey: "idem-1",
	}, time.Unix(100, 0))
	if err == nil {
		t.Fatal("expected mismatched user to fail")
	}
}

func TestApplyVisibilityMasksUnauthorizedAndInvisible(t *testing.T) {
	states := []types.PresenceState{
		{
			TenantID:     "tenant-1",
			UserID:       "user-self",
			ActualState:  types.PresenceStateOnline,
			VisibleState: types.PresenceStateOnline,
			DeviceCount:  1,
			DeviceStates: []types.DevicePresence{{DeviceID: "device-1"}},
		},
		{
			TenantID:     "tenant-1",
			UserID:       "user-other",
			ActualState:  types.PresenceStateOnline,
			VisibleState: types.PresenceStateOnline,
			DeviceCount:  1,
			DeviceStates: []types.DevicePresence{{DeviceID: "device-2"}},
		},
		{
			TenantID:     "tenant-1",
			UserID:       "user-invisible",
			ActualState:  types.PresenceStateInvisible,
			VisibleState: types.PresenceStateOffline,
			DeviceCount:  1,
			DeviceStates: []types.DevicePresence{{DeviceID: "device-3"}},
		},
	}
	filtered := ApplyVisibility(types.GetPresenceCommand{
		AuthContext:     types.AuthContext{TenantID: "tenant-1", UserID: "user-self"},
		RequesterUserID: "user-self",
		TargetUserIDs:   []string{"user-self", "user-other", "user-invisible"},
		IncludeDevices:  true,
	}, states)
	if filtered[0].VisibleState != types.PresenceStateOnline || len(filtered[0].DeviceStates) != 1 {
		t.Fatalf("self should see own state: %+v", filtered[0])
	}
	if filtered[1].VisibleState != types.PresenceStateUnknown || len(filtered[1].DeviceStates) != 0 {
		t.Fatalf("unauthorized target should be masked: %+v", filtered[1])
	}
	if filtered[2].VisibleState != types.PresenceStateOffline || len(filtered[2].DeviceStates) != 0 {
		t.Fatalf("invisible target should be offline masked: %+v", filtered[2])
	}
}
