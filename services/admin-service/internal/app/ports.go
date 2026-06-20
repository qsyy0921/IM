package app

import (
	"context"

	"github.com/qsyy0921/IM/services/admin-service/internal/domain"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

type AdminRepository interface {
	CreateAdminOperation(context.Context, domain.PreparedOperation) (types.AdminOperation, bool, error)
	ApproveAdminOperation(context.Context, domain.PreparedApproval) (types.AdminOperation, types.AdminApproval, bool, error)
	GetAdminOperation(context.Context, types.GetAdminOperationCommand) (types.AdminOperation, []types.AdminApproval, error)
	ListAdminOperations(context.Context, types.ListAdminOperationsCommand) ([]types.AdminOperation, error)
}

type IDGenerator interface {
	NewID(prefix string) string
}
