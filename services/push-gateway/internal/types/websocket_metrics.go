package types

import (
	"sync/atomic"
	"time"
)

type WebSocketWriterSnapshot struct {
	OutboundFrameDequeuedCount       uint64 `json:"outbound_frame_dequeued_count"`
	FrameWriteAttemptCount           uint64 `json:"frame_write_attempt_count"`
	FrameWriteSuccessCount           uint64 `json:"frame_write_success_count"`
	FrameWriteErrorCount             uint64 `json:"frame_write_error_count"`
	DeliveryNotifyWriteAttemptCount  uint64 `json:"delivery_notify_write_attempt_count"`
	DeliveryNotifyWriteSuccessCount  uint64 `json:"delivery_notify_write_success_count"`
	DeliveryNotifyWriteErrorCount    uint64 `json:"delivery_notify_write_error_count"`
	ResumeHintWriteAttemptCount      uint64 `json:"resume_hint_write_attempt_count"`
	ResumeHintWriteSuccessCount      uint64 `json:"resume_hint_write_success_count"`
	ResumeHintWriteErrorCount        uint64 `json:"resume_hint_write_error_count"`
	LastWriteSuccessAtMS             int64  `json:"last_write_success_at_ms,omitempty"`
	LastWriteErrorAtMS               int64  `json:"last_write_error_at_ms,omitempty"`
	LastDeliveryNotifyWriteAtMS      int64  `json:"last_delivery_notify_write_at_ms,omitempty"`
	LastDeliveryNotifyWriteErrorAtMS int64  `json:"last_delivery_notify_write_error_at_ms,omitempty"`
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
}

func (metrics *WebSocketWriterMetrics) RecordOutboundFrameDequeued(frame ServerFrame) {
	if metrics == nil {
		return
	}
	metrics.outboundFrameDequeuedCount.Add(1)
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
	}
}
