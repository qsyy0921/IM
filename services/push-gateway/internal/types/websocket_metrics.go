package types

import (
	"sync/atomic"
	"time"
)

type WebSocketWriterSnapshot struct {
	OutboundFrameDequeuedCount       uint64                          `json:"outbound_frame_dequeued_count"`
	FrameWriteAttemptCount           uint64                          `json:"frame_write_attempt_count"`
	FrameWriteSuccessCount           uint64                          `json:"frame_write_success_count"`
	FrameWriteErrorCount             uint64                          `json:"frame_write_error_count"`
	DeliveryNotifyWriteAttemptCount  uint64                          `json:"delivery_notify_write_attempt_count"`
	DeliveryNotifyWriteSuccessCount  uint64                          `json:"delivery_notify_write_success_count"`
	DeliveryNotifyWriteErrorCount    uint64                          `json:"delivery_notify_write_error_count"`
	ResumeHintWriteAttemptCount      uint64                          `json:"resume_hint_write_attempt_count"`
	ResumeHintWriteSuccessCount      uint64                          `json:"resume_hint_write_success_count"`
	ResumeHintWriteErrorCount        uint64                          `json:"resume_hint_write_error_count"`
	LastWriteSuccessAtMS             int64                           `json:"last_write_success_at_ms,omitempty"`
	LastWriteErrorAtMS               int64                           `json:"last_write_error_at_ms,omitempty"`
	LastDeliveryNotifyWriteAtMS      int64                           `json:"last_delivery_notify_write_at_ms,omitempty"`
	LastDeliveryNotifyWriteErrorAtMS int64                           `json:"last_delivery_notify_write_error_at_ms,omitempty"`
	FrameWriteDuration               WebSocketWriterDurationSnapshot `json:"frame_write_duration"`
	DeliveryNotifyWriteDuration      WebSocketWriterDurationSnapshot `json:"delivery_notify_write_duration"`
	FrameQueueDuration               WebSocketWriterDurationSnapshot `json:"frame_queue_duration"`
	DeliveryNotifyQueueDuration      WebSocketWriterDurationSnapshot `json:"delivery_notify_queue_duration"`
}

type WebSocketWriterDurationSnapshot struct {
	Count   uint64                          `json:"count"`
	SumMS   float64                         `json:"sum_ms"`
	MaxMS   float64                         `json:"max_ms"`
	Buckets []WebSocketWriterDurationBucket `json:"buckets"`
}

type WebSocketWriterDurationBucket struct {
	LE    string `json:"le"`
	Count uint64 `json:"count"`
}

const webSocketWriteDurationBucketCount = 14

var webSocketWriteDurationUpperBoundsMS = [...]float64{
	0.1,
	0.25,
	0.5,
	1,
	2,
	5,
	10,
	25,
	50,
	100,
	250,
	500,
	1000,
}

var webSocketWriteDurationBucketLabels = [...]string{
	"0.1",
	"0.25",
	"0.5",
	"1",
	"2",
	"5",
	"10",
	"25",
	"50",
	"100",
	"250",
	"500",
	"1000",
	"+Inf",
}

type webSocketDurationMetrics struct {
	count   atomic.Uint64
	sumNS   atomic.Uint64
	maxNS   atomic.Uint64
	buckets [webSocketWriteDurationBucketCount]atomic.Uint64
}

type WebSocketWriterMetrics struct {
	outboundFrameDequeuedCount       atomic.Uint64
	frameWriteAttemptCount           atomic.Uint64
	frameWriteSuccessCount           atomic.Uint64
	frameWriteErrorCount             atomic.Uint64
	deliveryNotifyWriteAttemptCount  atomic.Uint64
	deliveryNotifyWriteSuccessCount  atomic.Uint64
	deliveryNotifyWriteErrorCount    atomic.Uint64
	resumeHintWriteAttemptCount      atomic.Uint64
	resumeHintWriteSuccessCount      atomic.Uint64
	resumeHintWriteErrorCount        atomic.Uint64
	lastWriteSuccessAtMS             atomic.Int64
	lastWriteErrorAtMS               atomic.Int64
	lastDeliveryNotifyWriteAtMS      atomic.Int64
	lastDeliveryNotifyWriteErrorAtMS atomic.Int64
	frameWriteDuration               webSocketDurationMetrics
	deliveryNotifyWriteDuration      webSocketDurationMetrics
	frameQueueDuration               webSocketDurationMetrics
	deliveryNotifyQueueDuration      webSocketDurationMetrics
}

func (metrics *WebSocketWriterMetrics) RecordOutboundFrameDequeued(frame ServerFrame) {
	if metrics == nil {
		return
	}
	metrics.outboundFrameDequeuedCount.Add(1)
	if frame.EnqueuedAtMS <= 0 {
		return
	}
	duration := time.Since(time.UnixMilli(frame.EnqueuedAtMS))
	recordWebSocketDuration(&metrics.frameQueueDuration, duration)
	switch frame.Op {
	case OpDeliveryNotify, OpDeliveryHide:
		recordWebSocketDuration(&metrics.deliveryNotifyQueueDuration, duration)
	}
}

func (metrics *WebSocketWriterMetrics) RecordFrameWriteAttempt(frame ServerFrame) {
	if metrics == nil {
		return
	}
	metrics.frameWriteAttemptCount.Add(1)
	switch frame.Op {
	case OpDeliveryNotify, OpDeliveryHide:
		metrics.deliveryNotifyWriteAttemptCount.Add(1)
	case OpResumeHint:
		metrics.resumeHintWriteAttemptCount.Add(1)
	}
}

func (metrics *WebSocketWriterMetrics) RecordFrameWriteSuccess(frame ServerFrame) {
	if metrics == nil {
		return
	}
	now := time.Now().UnixMilli()
	metrics.frameWriteSuccessCount.Add(1)
	metrics.lastWriteSuccessAtMS.Store(now)
	switch frame.Op {
	case OpDeliveryNotify, OpDeliveryHide:
		metrics.deliveryNotifyWriteSuccessCount.Add(1)
		metrics.lastDeliveryNotifyWriteAtMS.Store(now)
	case OpResumeHint:
		metrics.resumeHintWriteSuccessCount.Add(1)
	}
}

func (metrics *WebSocketWriterMetrics) RecordFrameWriteError(frame ServerFrame) {
	if metrics == nil {
		return
	}
	now := time.Now().UnixMilli()
	metrics.frameWriteErrorCount.Add(1)
	metrics.lastWriteErrorAtMS.Store(now)
	switch frame.Op {
	case OpDeliveryNotify, OpDeliveryHide:
		metrics.deliveryNotifyWriteErrorCount.Add(1)
		metrics.lastDeliveryNotifyWriteErrorAtMS.Store(now)
	case OpResumeHint:
		metrics.resumeHintWriteErrorCount.Add(1)
	}
}

func (metrics *WebSocketWriterMetrics) RecordFrameWriteDuration(frame ServerFrame, duration time.Duration) {
	if metrics == nil {
		return
	}
	recordWebSocketDuration(&metrics.frameWriteDuration, duration)
	switch frame.Op {
	case OpDeliveryNotify, OpDeliveryHide:
		recordWebSocketDuration(&metrics.deliveryNotifyWriteDuration, duration)
	}
}

func (metrics *WebSocketWriterMetrics) Snapshot() WebSocketWriterSnapshot {
	if metrics == nil {
		return WebSocketWriterSnapshot{}
	}
	return WebSocketWriterSnapshot{
		OutboundFrameDequeuedCount:       metrics.outboundFrameDequeuedCount.Load(),
		FrameWriteAttemptCount:           metrics.frameWriteAttemptCount.Load(),
		FrameWriteSuccessCount:           metrics.frameWriteSuccessCount.Load(),
		FrameWriteErrorCount:             metrics.frameWriteErrorCount.Load(),
		DeliveryNotifyWriteAttemptCount:  metrics.deliveryNotifyWriteAttemptCount.Load(),
		DeliveryNotifyWriteSuccessCount:  metrics.deliveryNotifyWriteSuccessCount.Load(),
		DeliveryNotifyWriteErrorCount:    metrics.deliveryNotifyWriteErrorCount.Load(),
		ResumeHintWriteAttemptCount:      metrics.resumeHintWriteAttemptCount.Load(),
		ResumeHintWriteSuccessCount:      metrics.resumeHintWriteSuccessCount.Load(),
		ResumeHintWriteErrorCount:        metrics.resumeHintWriteErrorCount.Load(),
		LastWriteSuccessAtMS:             metrics.lastWriteSuccessAtMS.Load(),
		LastWriteErrorAtMS:               metrics.lastWriteErrorAtMS.Load(),
		LastDeliveryNotifyWriteAtMS:      metrics.lastDeliveryNotifyWriteAtMS.Load(),
		LastDeliveryNotifyWriteErrorAtMS: metrics.lastDeliveryNotifyWriteErrorAtMS.Load(),
		FrameWriteDuration:               snapshotWebSocketDuration(&metrics.frameWriteDuration),
		DeliveryNotifyWriteDuration:      snapshotWebSocketDuration(&metrics.deliveryNotifyWriteDuration),
		FrameQueueDuration:               snapshotWebSocketDuration(&metrics.frameQueueDuration),
		DeliveryNotifyQueueDuration:      snapshotWebSocketDuration(&metrics.deliveryNotifyQueueDuration),
	}
}

func recordWebSocketDuration(metrics *webSocketDurationMetrics, duration time.Duration) {
	if duration < 0 {
		duration = 0
	}
	nanos := uint64(duration.Nanoseconds())
	metrics.count.Add(1)
	metrics.sumNS.Add(nanos)
	for {
		current := metrics.maxNS.Load()
		if nanos <= current || metrics.maxNS.CompareAndSwap(current, nanos) {
			break
		}
	}
	bucketIndex := webSocketDurationBucketIndex(duration)
	metrics.buckets[bucketIndex].Add(1)
}

func webSocketDurationBucketIndex(duration time.Duration) int {
	ms := float64(duration.Nanoseconds()) / float64(time.Millisecond)
	for index, upperBoundMS := range webSocketWriteDurationUpperBoundsMS {
		if ms <= upperBoundMS {
			return index
		}
	}
	return webSocketWriteDurationBucketCount - 1
}

func snapshotWebSocketDuration(metrics *webSocketDurationMetrics) WebSocketWriterDurationSnapshot {
	count := metrics.count.Load()
	buckets := make([]WebSocketWriterDurationBucket, 0, webSocketWriteDurationBucketCount)
	var cumulative uint64
	for index := 0; index < webSocketWriteDurationBucketCount; index++ {
		cumulative += metrics.buckets[index].Load()
		buckets = append(buckets, WebSocketWriterDurationBucket{
			LE:    webSocketWriteDurationBucketLabels[index],
			Count: cumulative,
		})
	}
	return WebSocketWriterDurationSnapshot{
		Count:   count,
		SumMS:   float64(metrics.sumNS.Load()) / float64(time.Millisecond),
		MaxMS:   float64(metrics.maxNS.Load()) / float64(time.Millisecond),
		Buckets: buckets,
	}
}
