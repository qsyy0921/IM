package types

import "time"

type LatencyRecorder interface {
	ObserveConversationSeqAlloc(time.Duration)
	ObserveKafkaPublish(time.Duration)
}

type NoopLatencyRecorder struct{}

func (NoopLatencyRecorder) ObserveConversationSeqAlloc(time.Duration) {}

func (NoopLatencyRecorder) ObserveKafkaPublish(time.Duration) {}
