package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
)

type capacityWorkerResult struct {
	messageCount   int
	pullItemCount  int
	ackCount       int
	markReadCount  int
	errorCount     int
	latencySamples []float64
	firstErr       error
}

func executeCapacity(
	ctx context.Context,
	cfg config,
	pool *pgxpool.Pool,
	conversationClient conversationv1.ConversationServiceClient,
	messageClient messagev1.MessageServiceClient,
	deliveryClient deliveryv1.DeliveryServiceClient,
	receiptClient receiptv1.ReceiptServiceClient,
	result *summary,
) error {
	conversationIDs := make([]string, 0, cfg.vus)
	workerConfigs := make([]config, 0, cfg.vus)
	for vu := 1; vu <= cfg.vus; vu++ {
		workerCfg := capacityConfigForVU(cfg, vu)
		if err := seedConversation(ctx, pool, workerCfg); err != nil {
			return fmt.Errorf("seed capacity conversation vu=%d: %w", vu, err)
		}
		if _, err := createReceiverJoin(ctx, workerCfg, conversationClient); err != nil {
			return fmt.Errorf("create receiver join vu=%d: %w", vu, err)
		}
		if err := waitMembership(ctx, pool, workerCfg); err != nil {
			return fmt.Errorf("wait membership vu=%d: %w", vu, err)
		}
		workerConfigs = append(workerConfigs, workerCfg)
		conversationIDs = append(conversationIDs, workerCfg.conversationID)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	endAt := time.Now().Add(cfg.duration)
	var sequence atomic.Int64
	results := make([]capacityWorkerResult, len(workerConfigs))
	var wg sync.WaitGroup
	for index, workerCfg := range workerConfigs {
		wg.Add(1)
		go func(index int, workerCfg config) {
			defer wg.Done()
			results[index] = runCapacityWorker(runCtx, workerCfg, pool, messageClient, deliveryClient, receiptClient, &sequence, endAt)
			if results[index].firstErr != nil {
				cancel()
			}
		}(index, workerCfg)
	}
	wg.Wait()

	for index, workerResult := range results {
		result.CapacityMessageCount += workerResult.messageCount
		result.CapacityPullItemCount += workerResult.pullItemCount
		result.CapacityAckCount += workerResult.ackCount
		result.CapacityMarkReadCount += workerResult.markReadCount
		result.CapacityErrorCount += workerResult.errorCount
		result.CapacityLatencySamplesMS = append(result.CapacityLatencySamplesMS, workerResult.latencySamples...)
		if workerResult.firstErr != nil {
			return fmt.Errorf("capacity worker %d failed: %w", index+1, workerResult.firstErr)
		}
	}

	wantReceiptEvents := int64(result.CapacityAckCount + result.CapacityMarkReadCount)
	if err := waitReceiptOutboxPublishedForTenant(ctx, pool, cfg, wantReceiptEvents); err != nil {
		return err
	}
	if err := fillCapacityPostgresStats(ctx, pool, cfg, result); err != nil {
		return err
	}
	if wantReceiptEvents > 0 {
		eventCount, err := readReceiptEventCount(ctx, cfg, int(wantReceiptEvents), conversationIDs)
		if err != nil {
			return err
		}
		result.CapacityReceiptKafkaEventCount = eventCount
	}
	return nil
}

func capacityConfigForVU(cfg config, vu int) config {
	cfg.conversationID = fmt.Sprintf("%s-vu%02d", cfg.conversationID, vu)
	cfg.ownerUserID = fmt.Sprintf("%s-vu%02d", cfg.ownerUserID, vu)
	cfg.receiverUserID = fmt.Sprintf("%s-vu%02d", cfg.receiverUserID, vu)
	cfg.receiverDeviceID = fmt.Sprintf("%s-vu%02d", cfg.receiverDeviceID, vu)
	return cfg
}

func runCapacityWorker(
	ctx context.Context,
	cfg config,
	pool *pgxpool.Pool,
	messageClient messagev1.MessageServiceClient,
	deliveryClient deliveryv1.DeliveryServiceClient,
	receiptClient receiptv1.ReceiptServiceClient,
	sequence *atomic.Int64,
	endAt time.Time,
) capacityWorkerResult {
	result := capacityWorkerResult{latencySamples: make([]float64, 0, 64)}
	for time.Now().Before(endAt) {
		select {
		case <-ctx.Done():
			return result
		default:
		}
		messageIndex := int(sequence.Add(1))
		begin := time.Now()
		send, err := sendMessage(ctx, cfg, messageClient, messageIndex)
		if err != nil {
			return capacityWorkerError(result, fmt.Errorf("send message: %w", err))
		}
		result.messageCount++
		pull, err := pullInboxAtLeast(ctx, cfg, deliveryClient, send.GetConversationSeq())
		if err != nil {
			return capacityWorkerError(result, fmt.Errorf("pull inbox: %w", err))
		}
		if pull.MaxSeq < send.GetConversationSeq() {
			return capacityWorkerError(result, fmt.Errorf("pull inbox max seq %d did not reach sent seq %d", pull.MaxSeq, send.GetConversationSeq()))
		}
		result.pullItemCount += pull.ItemCount
		ack, err := ackDelivery(ctx, cfg, deliveryClient, send.GetConversationSeq())
		if err != nil {
			return capacityWorkerError(result, fmt.Errorf("ack delivery: %w", err))
		}
		if ack.GetLastReceivedSeq() < send.GetConversationSeq() {
			return capacityWorkerError(result, fmt.Errorf("ack last_received_seq %d did not reach sent seq %d", ack.GetLastReceivedSeq(), send.GetConversationSeq()))
		}
		result.ackCount++
		if err := waitReceiptReceived(ctx, pool, cfg, send.GetConversationSeq()); err != nil {
			return capacityWorkerError(result, err)
		}
		mark, err := markRead(ctx, cfg, receiptClient, send.GetConversationSeq())
		if err != nil {
			return capacityWorkerError(result, fmt.Errorf("mark read: %w", err))
		}
		if mark.GetLastReadSeq() != send.GetConversationSeq() {
			return capacityWorkerError(result, fmt.Errorf("mark read last_read_seq %d did not match sent seq %d", mark.GetLastReadSeq(), send.GetConversationSeq()))
		}
		result.markReadCount++
		result.latencySamples = append(result.latencySamples, elapsedMS(begin))
	}
	return result
}

func capacityWorkerError(result capacityWorkerResult, err error) capacityWorkerResult {
	result.errorCount++
	result.firstErr = err
	return result
}

func waitReceiptOutboxPublishedForTenant(ctx context.Context, pool *pgxpool.Pool, cfg config, wantPublished int64) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var total int64
		var published int64
		var dlq int64
		err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ')
FROM receipt_outbox
WHERE tenant_id = $1
`, cfg.tenantID).Scan(&total, &published, &dlq)
		if err != nil {
			return fmt.Errorf("query tenant receipt outbox publish state: %w", err)
		}
		if dlq > 0 {
			return fmt.Errorf("tenant receipt outbox reached DLQ: dlq=%d", dlq)
		}
		if total >= wantPublished && published >= wantPublished {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return fmt.Errorf("tenant receipt outbox publish timeout: want_published=%d", wantPublished)
}

func fillCapacityPostgresStats(ctx context.Context, pool *pgxpool.Pool, cfg config, result *summary) error {
	if err := fillTenantReceiptOutboxStats(ctx, pool, cfg, &result.ReceiptOutbox); err != nil {
		return err
	}
	if err := fillTenantDeliveryOutboxStats(ctx, pool, cfg, &result.DeliveryOutbox); err != nil {
		return err
	}
	return nil
}

func fillTenantReceiptOutboxStats(ctx context.Context, pool *pgxpool.Pool, cfg config, stats *receiptOutboxStats) error {
	if err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING'),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ')
FROM receipt_outbox
WHERE tenant_id = $1
`, cfg.tenantID).Scan(&stats.Total, &stats.Pending, &stats.Published, &stats.DLQ); err != nil {
		return fmt.Errorf("query tenant receipt outbox stats: %w", err)
	}
	rows, err := pool.Query(ctx, `
SELECT event_type, COUNT(*)
FROM receipt_outbox
WHERE tenant_id = $1
GROUP BY event_type
ORDER BY event_type
`, cfg.tenantID)
	if err != nil {
		return fmt.Errorf("query tenant receipt outbox by type: %w", err)
	}
	defer rows.Close()
	stats.ByEventType = map[string]int64{}
	for rows.Next() {
		var eventType string
		var count int64
		if err := rows.Scan(&eventType, &count); err != nil {
			return fmt.Errorf("scan tenant receipt outbox by type: %w", err)
		}
		stats.ByEventType[eventType] = count
	}
	return rows.Err()
}

func fillTenantDeliveryOutboxStats(ctx context.Context, pool *pgxpool.Pool, cfg config, stats *outboxStats) error {
	if err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING'),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ')
FROM delivery_outbox
WHERE tenant_id = $1
`, cfg.tenantID).Scan(&stats.Total, &stats.Pending, &stats.Published, &stats.DLQ); err != nil {
		return fmt.Errorf("query tenant delivery outbox stats: %w", err)
	}
	return nil
}
