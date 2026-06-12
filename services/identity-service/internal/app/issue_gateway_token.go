package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/domain"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type IssueGatewayTokenUseCase struct {
	repository Repository
	signer     TokenSigner
	now        func() time.Time
}

func NewIssueGatewayTokenUseCase(repository Repository, signer TokenSigner) *IssueGatewayTokenUseCase {
	return &IssueGatewayTokenUseCase{repository: repository, signer: signer, now: func() time.Time { return time.Now().UTC() }}
}

func (uc *IssueGatewayTokenUseCase) Execute(ctx context.Context, command types.IssueGatewayTokenCommand) (types.IssueGatewayTokenResult, error) {
	if err := domain.ValidateIssueGatewayToken(command); err != nil {
		return types.IssueGatewayTokenResult{}, err
	}
	if uc.repository == nil {
		return types.IssueGatewayTokenResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	if uc.signer == nil {
		return types.IssueGatewayTokenResult{}, types.NewTokenSigningFailed("token signer is not configured")
	}
	command.Audience = domain.NormalizeAudience(command.Audience)
	issuedAt := uc.now()
	expiresAt := issuedAt.Add(domain.NormalizeTTL(command.TTLSeconds))
	result, err := uc.repository.IssueGatewaySession(ctx, command, issuedAt, expiresAt)
	if err != nil {
		return types.IssueGatewayTokenResult{}, err
	}
	token, err := uc.signer.SignGatewayToken(types.TokenClaims{
		TenantID:  result.TenantID,
		UserID:    result.UserID,
		DeviceID:  result.DeviceID,
		SessionID: result.SessionID,
		Audience:  result.Audience,
		TraceID:   command.TraceID,
		ExpiresAt: expiresAt.Unix(),
	})
	if err != nil {
		return types.IssueGatewayTokenResult{}, err
	}
	result.GatewayToken = token
	return result, nil
}
