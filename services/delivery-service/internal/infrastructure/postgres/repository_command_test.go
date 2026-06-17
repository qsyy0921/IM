package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func TestRepositoryAckDeliveryIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedInbox(t, ctx, pool, 1)
	seedInbox(t, ctx, pool, 2)
	repository := NewRepository(pool)
	command := ackCommand(3)
	_, err := repository.AckDelivery(ctx, command)
	if !errors.Is(err, types.ErrAckOutOfVisibleRange) {
		t.Fatalf("expected out of visible range, got %v", err)
	}
	assertDeliveryCursor(t, ctx, pool, 0)

	result, err := repository.AckDelivery(ctx, ackCommand(2))
	if err != nil {
		t.Fatalf("ack visible seq: %v", err)
	}
	if result.LastReceivedSeq != 2 {
		t.Fatalf("expected cursor 2, got %d", result.LastReceivedSeq)
	}
	assertDeliveryCursor(t, ctx, pool, 2)
	assertDeliveryOutboxCount(t, ctx, pool, "delivery.ack.recorded.v1", 1)

	result, err = repository.AckDelivery(ctx, ackCommand(1))
	if err != nil {
		t.Fatalf("repeat lower ack should be idempotent: %v", err)
	}
	if result.LastReceivedSeq != 2 {
		t.Fatalf("expected cursor to stay 2, got %d", result.LastReceivedSeq)
	}
	assertDeliveryOutboxCount(t, ctx, pool, "delivery.ack.recorded.v1", 1)
}

func TestRepositoryAckDeliveryConcurrentFirstAckIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedInbox(t, ctx, pool, 5)
	repository := NewRepository(pool)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repository.AckDelivery(ctx, ackCommand(5))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent ack failed: %v", err)
		}
	}
	assertDeliveryCursor(t, ctx, pool, 5)
	assertDeliveryOutboxCount(t, ctx, pool, "delivery.ack.recorded.v1", 1)
}

func TestRepositoryHideInboxItemIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedInbox(t, ctx, pool, 1)
	seedInbox(t, ctx, pool, 2)
	repository := NewRepository(pool)

	hideCommand := types.HideInboxItemCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-delivery",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID:  "conv-delivery",
		ConversationSeq: 1,
		Reason:          "hide locally",
	}
	result, err := repository.HideInboxItem(ctx, hideCommand)
	if err != nil {
		t.Fatalf("hide inbox item: %v", err)
	}
	if result.AlreadyHidden {
		t.Fatalf("first hide should not be marked already hidden: %+v", result)
	}
	assertDeliveryOutboxCount(t, ctx, pool, types.DeliveryEventInboxItemHidden, 1)

	items, err := repository.PullInbox(ctx, types.PullInboxCommand{
		AuthContext:    hideCommand.AuthContext,
		ConversationID: hideCommand.ConversationID,
		AfterSeq:       0,
		Limit:          10,
	}, 10)
	if err != nil {
		t.Fatalf("pull inbox after hide: %v", err)
	}
	if len(items) != 1 || items[0].ConversationSeq != 2 {
		t.Fatalf("expected only seq 2 after hide, got %+v", items)
	}

	result, err = repository.HideInboxItem(ctx, hideCommand)
	if err != nil {
		t.Fatalf("repeat hide inbox item: %v", err)
	}
	if !result.AlreadyHidden {
		t.Fatalf("repeat hide should be idempotent already_hidden: %+v", result)
	}
	assertDeliveryOutboxCount(t, ctx, pool, types.DeliveryEventInboxItemHidden, 1)

	ackResult, err := repository.AckDelivery(ctx, ackCommand(2))
	if err != nil {
		t.Fatalf("ack after hide: %v", err)
	}
	if ackResult.LastReceivedSeq != 2 {
		t.Fatalf("expected ack cursor 2, got %d", ackResult.LastReceivedSeq)
	}

	missing := hideCommand
	missing.ConversationSeq = 99
	_, err = repository.HideInboxItem(ctx, missing)
	if !errors.Is(err, types.ErrInboxItemNotFound) {
		t.Fatalf("expected inbox item not found, got %v", err)
	}

	var hiddenByDeviceID, hideReason string
	if err := pool.QueryRow(ctx, `
SELECT hidden_by_device_id, hide_reason
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND user_id = 'user-1'
  AND conversation_id = 'conv-delivery'
  AND conversation_seq = 1
`).Scan(&hiddenByDeviceID, &hideReason); err != nil {
		t.Fatalf("read hidden metadata: %v", err)
	}
	if hiddenByDeviceID != "device-1" || hideReason != "hide locally" {
		t.Fatalf("unexpected hidden metadata device=%q reason=%q", hiddenByDeviceID, hideReason)
	}

	var payloadUserID, payloadDeviceID, payloadMessageID string
	var payloadSeq int64
	if err := pool.QueryRow(ctx, `
SELECT
    payload_json->>'user_id',
    payload_json->>'device_id',
    payload_json->>'message_id',
    (payload_json->>'conversation_seq')::BIGINT
FROM delivery_outbox
WHERE tenant_id = 'tenant-delivery'
  AND event_type = $1
`, types.DeliveryEventInboxItemHidden).Scan(&payloadUserID, &payloadDeviceID, &payloadMessageID, &payloadSeq); err != nil {
		t.Fatalf("read hidden outbox payload: %v", err)
	}
	if payloadUserID != "user-1" || payloadDeviceID != "device-1" || payloadMessageID != "seed-msg-1" || payloadSeq != 1 {
		t.Fatalf("unexpected hidden outbox payload user=%q device=%q message=%q seq=%d", payloadUserID, payloadDeviceID, payloadMessageID, payloadSeq)
	}
}
