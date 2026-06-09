package app

import (
	"context"

	"github.com/qsyy0921/IM/services/push-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

type ConnectSessionUseCase struct {
	registry SessionRegistry
}

func NewConnectSessionUseCase(registry SessionRegistry) *ConnectSessionUseCase {
	return &ConnectSessionUseCase{registry: registry}
}

func (usecase *ConnectSessionUseCase) Execute(
	ctx context.Context,
	command types.ConnectSessionCommand,
	outbound chan<- types.ServerFrame,
	evicted chan<- types.SessionEviction,
) (types.ConnectSessionResult, error) {
	if err := command.AuthContext.Validate(); err != nil {
		return types.ConnectSessionResult{}, err
	}
	queueSize := command.QueueSize
	if queueSize <= 0 {
		queueSize = types.DefaultSessionQueueSize
	}
	heartbeat := command.HeartbeatInterval
	if heartbeat <= 0 {
		heartbeat = types.DefaultHeartbeatInterval
	}
	sessionID := command.AuthContext.SessionID
	if sessionID == "" {
		sessionID = domain.NewOpaqueID("sess")
	}
	resumeRequested := command.ResumeToken != ""
	resumeToken := command.ResumeToken
	if resumeToken == "" {
		resumeToken = domain.NewOpaqueID("resume")
	}
	result := types.ConnectSessionResult{
		SessionID:           sessionID,
		ResumeToken:         resumeToken,
		HeartbeatIntervalMS: heartbeat.Milliseconds(),
	}
	auth := command.AuthContext
	auth.SessionID = result.SessionID
	if err := usecase.registry.Register(ctx, types.SessionRegistration{
		AuthContext:     auth,
		SessionID:       result.SessionID,
		ResumeToken:     result.ResumeToken,
		ResumeRequested: resumeRequested,
		LastReceived:    command.LastReceived,
		Outbound:        outbound,
		Evicted:         evicted,
	}); err != nil {
		return types.ConnectSessionResult{}, err
	}
	_ = queueSize
	return result, nil
}
