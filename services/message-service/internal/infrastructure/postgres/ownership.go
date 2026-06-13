package postgres

import (
	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func canMutateMessageOwnership(message domain.Message, actorID types.UserID, permission types.PermissionDecision) bool {
	return message.SenderID == actorID || permission.OwnershipOverride
}
