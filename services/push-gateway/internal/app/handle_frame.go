package app

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

type HandleClientFrameUseCase struct {
	deliveryClient DeliveryClient
	now            func() time.Time
}

func NewHandleClientFrameUseCase(deliveryClient DeliveryClient) *HandleClientFrameUseCase {
	return &HandleClientFrameUseCase{deliveryClient: deliveryClient, now: time.Now}
}

func (usecase *HandleClientFrameUseCase) Execute(
	ctx context.Context,
	auth types.AuthContext,
	frame types.ClientFrame,
) (types.ServerFrame, error) {
	switch frame.Op {
	case types.OpClientPing:
		return domain.ServerPong(frame.RequestID, usecase.now()), nil
	case types.OpClientHello:
		return types.ServerFrame{}, nil
	case types.OpDeliveryAck:
		if frame.ConversationID == "" {
			return types.ServerFrame{}, types.NewInvalidFrame("conversation_id is required")
		}
		if frame.ReceivedSeq <= 0 {
			return types.ServerFrame{}, types.NewInvalidFrame("received_seq must be positive")
		}
		if usecase.deliveryClient == nil {
			return types.ServerFrame{}, types.NewDeliveryUnavailable("delivery client is not configured")
		}
		result, err := usecase.deliveryClient.AckDelivery(ctx, types.AckDeliveryCommand{
			AuthContext:    auth,
			ConversationID: frame.ConversationID,
			ReceivedSeq:    frame.ReceivedSeq,
		})
		if err != nil {
			return types.ServerFrame{}, err
		}
		return domain.DeliveryAckOK(frame.RequestID, result), nil
	default:
		if frame.Op == "" {
			return types.ServerFrame{}, types.NewInvalidFrame("op is required")
		}
		return types.ServerFrame{}, types.NewInvalidFrame("unsupported op")
	}
}

func PublicErrorFrame(requestID string, err error) types.ServerFrame {
	switch {
	case errors.Is(err, types.ErrInvalidFrame):
		return domain.ErrorFrame(requestID, "INVALID_FRAME", "invalid frame", false)
	case errors.Is(err, types.ErrAckOutOfVisibleRange):
		return domain.ErrorFrame(requestID, "ACK_OUT_OF_VISIBLE_RANGE", "ack out of visible range", false)
	case errors.Is(err, types.ErrDeliveryUnavailable):
		return domain.ErrorFrame(requestID, "DELIVERY_UNAVAILABLE", "delivery unavailable", true)
	case errors.Is(err, types.ErrSessionQueueFull):
		return domain.ErrorFrame(requestID, "SERVER_BUSY", "server busy", true)
	default:
		return domain.ErrorFrame(requestID, "SERVER_BUSY", "server busy", true)
	}
}
