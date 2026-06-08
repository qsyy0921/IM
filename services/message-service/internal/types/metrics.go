package types

import "time"

type LatencyRecorder interface {
	ObserveSendMessage(time.Duration)
	ObserveRepositoryAppend(time.Duration)
	ObserveRepositoryCommit(time.Duration)
	ObserveConversationSeqAlloc(time.Duration)
	ObserveKafkaPublish(time.Duration)
}

type NoopLatencyRecorder struct{}

func (NoopLatencyRecorder) ObserveSendMessage(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryAppend(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryCommit(time.Duration) {}

func (NoopLatencyRecorder) ObserveConversationSeqAlloc(time.Duration) {}

func (NoopLatencyRecorder) ObserveKafkaPublish(time.Duration) {}
