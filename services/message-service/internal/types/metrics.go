package types

import "time"

type LatencyRecorder interface {
	ObserveSendMessage(time.Duration)
	ObserveSendMessageCommandBuild(time.Duration)
	ObserveSendMessageAdmission(time.Duration)
	ObserveSendMessageDependencyRead(time.Duration)
	ObserveSendMessageConversationContext(time.Duration)
	ObserveSendMessagePolicyCheck(time.Duration)
	ObserveSendMessageSeqFloor(time.Duration)
	ObserveSendMessageSequencerAllocate(time.Duration)
	ObserveSendMessageRepositoryAppendCall(time.Duration)
	ObserveRepositoryAppend(time.Duration)
	ObserveRepositoryBegin(time.Duration)
	ObserveRepositoryPoolAcquire(time.Duration)
	ObserveRepositoryTxBegin(time.Duration)
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
	ObserveKafkaPublishCall(time.Duration, int)
	ObserveOutboxProcessReady(time.Duration)
	ObserveOutboxProcessReadyResult(time.Duration, int)
	ObserveOutboxFetchReady(time.Duration)
	ObserveOutboxMarkPublished(time.Duration)
	ObserveOutboxCommit(time.Duration)
}

type NoopLatencyRecorder struct{}

func (NoopLatencyRecorder) ObserveSendMessage(time.Duration) {}

func (NoopLatencyRecorder) ObserveSendMessageCommandBuild(time.Duration) {}

func (NoopLatencyRecorder) ObserveSendMessageAdmission(time.Duration) {}

func (NoopLatencyRecorder) ObserveSendMessageDependencyRead(time.Duration) {}

func (NoopLatencyRecorder) ObserveSendMessageConversationContext(time.Duration) {}

func (NoopLatencyRecorder) ObserveSendMessagePolicyCheck(time.Duration) {}

func (NoopLatencyRecorder) ObserveSendMessageSeqFloor(time.Duration) {}

func (NoopLatencyRecorder) ObserveSendMessageSequencerAllocate(time.Duration) {}

func (NoopLatencyRecorder) ObserveSendMessageRepositoryAppendCall(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryAppend(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryBegin(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryPoolAcquire(time.Duration) {}

func (NoopLatencyRecorder) ObserveRepositoryTxBegin(time.Duration) {}

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

func (NoopLatencyRecorder) ObserveKafkaPublishCall(time.Duration, int) {}

func (NoopLatencyRecorder) ObserveOutboxProcessReady(time.Duration) {}

func (NoopLatencyRecorder) ObserveOutboxProcessReadyResult(time.Duration, int) {}

func (NoopLatencyRecorder) ObserveOutboxFetchReady(time.Duration) {}

func (NoopLatencyRecorder) ObserveOutboxMarkPublished(time.Duration) {}

func (NoopLatencyRecorder) ObserveOutboxCommit(time.Duration) {}
