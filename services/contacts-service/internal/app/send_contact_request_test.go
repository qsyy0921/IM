package app

import (
	"context"
	"testing"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func TestSourcePolicyUseCasesNormalizeDefaultSourceType(t *testing.T) {
	getRepo := &captureGetSourcePolicyRepository{}
	getResult, err := NewGetTenantContactRequestSourcePolicyUseCase(getRepo).Execute(
		context.Background(),
		types.GetTenantContactRequestSourcePolicyCommand{TenantID: "tenant-1"},
	)
	if err != nil {
		t.Fatalf("get source policy: %v", err)
	}
	if getRepo.command.SourceType != types.ContactRequestSourceTypeDirect ||
		getResult.Policy.SourceType != types.ContactRequestSourceTypeDirect {
		t.Fatalf("expected default get source type to normalize to DIRECT, command=%+v result=%+v", getRepo.command, getResult)
	}

	setRepo := &captureSetSourcePolicyRepository{}
	setResult, err := NewSetTenantContactRequestSourcePolicyUseCase(setRepo).Execute(
		context.Background(),
		types.SetTenantContactRequestSourcePolicyCommand{
			TenantID:             "tenant-1",
			AllowContactRequests: true,
		},
	)
	if err != nil {
		t.Fatalf("set source policy: %v", err)
	}
	if setRepo.command.SourceType != types.ContactRequestSourceTypeDirect ||
		setResult.Policy.SourceType != types.ContactRequestSourceTypeDirect {
		t.Fatalf("expected default set source type to normalize to DIRECT, command=%+v result=%+v", setRepo.command, setResult)
	}
}

type captureGetSourcePolicyRepository struct {
	command types.GetTenantContactRequestSourcePolicyCommand
}

func (r *captureGetSourcePolicyRepository) GetTenantContactRequestSourcePolicy(
	_ context.Context,
	command types.GetTenantContactRequestSourcePolicyCommand,
) (types.GetTenantContactRequestSourcePolicyResult, error) {
	r.command = command
	return types.GetTenantContactRequestSourcePolicyResult{
		TenantID: command.TenantID,
		Policy: types.ContactRequestSourcePolicy{
			SourceType:           command.SourceType,
			AllowContactRequests: true,
		},
	}, nil
}

type captureSetSourcePolicyRepository struct {
	command types.SetTenantContactRequestSourcePolicyCommand
}

func (r *captureSetSourcePolicyRepository) SetTenantContactRequestSourcePolicy(
	_ context.Context,
	command types.SetTenantContactRequestSourcePolicyCommand,
) (types.SetTenantContactRequestSourcePolicyResult, error) {
	r.command = command
	return types.SetTenantContactRequestSourcePolicyResult{
		TenantID: command.TenantID,
		Policy: types.ContactRequestSourcePolicy{
			SourceType:           command.SourceType,
			AllowContactRequests: command.AllowContactRequests,
		},
		Changed: true,
	}, nil
}
