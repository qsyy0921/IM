package access

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type StaticAllowAccess struct{}

func NewStaticAllowAccess() StaticAllowAccess {
	return StaticAllowAccess{}
}

func (StaticAllowAccess) CanMarkRead(_ context.Context, auth types.AuthContext, conversationID types.ConversationID) (types.ReceiptAccessContext, error) {
	return allow(auth, conversationID), nil
}

func (StaticAllowAccess) CanViewReceiptState(_ context.Context, auth types.AuthContext, conversationID types.ConversationID) (types.ReceiptAccessContext, error) {
	return allow(auth, conversationID), nil
}

func allow(auth types.AuthContext, conversationID types.ConversationID) types.ReceiptAccessContext {
	return types.ReceiptAccessContext{
		TenantID:       auth.TenantID,
		UserID:         auth.UserID,
		ConversationID: conversationID,
		VisibilityMode: types.ReceiptVisibilityDetailed,
	}
}
