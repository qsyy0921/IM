package types

import "time"

type LatencyRecorder interface {
	ObserveSendMessage(time.Duration)
	ObserveRepositoryAppend(time.Duration)
	ObserveRepositoryBegin(time.Duration)
	ObserveRepositoryIdempotencyLock(time.Duration)
	ObserveRepositoryFindExisting(time.Duration)
	ObserveRepositoryEnsureSeq(time.Duration)
	ObserveRepositoryAllocateSeq(time.Duration)
	ObserveRepositoryInsertMessage(time.Duration)
	ObserveRepositoryInsertTimeline(time.Duration)
	ObserveRepositoryInsertOutbox(time.Duration)
	ObserveRepositoryCommit(time.Duration)
	ObserveConversationSeqAlloc(time.Duration)
	ObserveKafkaPublish(time.Duration)
}

type NoopLatencyRecorder struct{}

func (NoopLatencyRecorder) ObserveSendMessage(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryAppend(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryBegin(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryIdempotencyLock(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryFindExisting(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryEnsureSeq(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryAllocateSeq(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryInsertMessage(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryInsertTimeline(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryInsertOutbox(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryCommit(time.Duration) {}

func (NoopLatencyRecorder) ObserveConversationSeqAlloc(time.Duration) {}

func (NoopLatencyRecorder) ObserveKafkaPublish(time.Duration) {}
