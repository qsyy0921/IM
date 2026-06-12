package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type Repository interface {
	IssueGatewaySession(context.Context, types.IssueGatewayTokenCommand, time.Time, time.Time) (types.IssueGatewayTokenResult, error)
	RevokeDevice(context.Context, types.RevokeDeviceCommand, time.Time) (types.RevokeDeviceResult, error)
	RevokeSession(context.Context, types.RevokeSessionCommand, time.Time) (types.RevokeSessionResult, error)
	GetDeviceState(context.Context, types.GetDeviceStateCommand) (types.GetDeviceStateResult, error)
}

type TokenSigner interface {
	SignGatewayToken(types.TokenClaims) (string, error)
}
