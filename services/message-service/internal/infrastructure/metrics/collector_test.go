package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCollectorSnapshot(t *testing.T) {
	collector := NewCollector()
	collector.ObserveSendMessage(40 * time.Millisecond)
	collector.ObserveRepositoryAppend(35 * time.Millisecond)
	collector.ObserveRepositoryBegin(6 * time.Millisecond)
	collector.ObserveRepositoryPoolAcquire(5 * time.Millisecond)
	collector.ObserveRepositoryTxBegin(1 * time.Millisecond)
	collector.ObserveRepositoryIdempotencyLock(7 * time.Millisecond)
	collector.ObserveRepositoryFindExisting(8 * time.Millisecond)
	collector.ObserveRepositoryEnsureSeq(9 * time.Millisecond)
	collector.ObserveRepositoryAllocateSeq(10 * time.Millisecond)
	collector.ObserveRepositoryInsertMessage(11 * time.Millisecond)
	collector.ObserveRepositoryInsertTimeline(12 * time.Millisecond)
	collector.ObserveRepositoryInsertOutbox(13 * time.Millisecond)
	collector.ObserveRepositoryCommit(4 * time.Millisecond)
	collector.ObserveConversationSeqAlloc(10 * time.Millisecond)
	collector.ObserveConversationSeqAlloc(20 * time.Millisecond)
	collector.ObserveKafkaPublish(30 * time.Millisecond)

	snapshot := collector.Snapshot()
	if snapshot.SendMessageLatencyMS.Count != 1 ||
		snapshot.SendMessageLatencyMS.AvgMS != 40 {
		t.Fatalf("unexpected send message snapshot: %+v", snapshot.SendMessageLatencyMS)
	}
	if snapshot.RepositoryAppendLatencyMS.Count != 1 ||
		snapshot.RepositoryAppendLatencyMS.AvgMS != 35 {
		t.Fatalf("unexpected repository append snapshot: %+v", snapshot.RepositoryAppendLatencyMS)
	}
	if snapshot.RepositoryBeginLatencyMS.Count != 1 ||
		snapshot.RepositoryPoolAcquireLatencyMS.Count != 1 ||
		snapshot.RepositoryPoolAcquireLatencyMS.AvgMS != 5 ||
		snapshot.RepositoryTxBeginLatencyMS.Count != 1 ||
		snapshot.RepositoryTxBeginLatencyMS.AvgMS != 1 ||
		snapshot.RepositoryIdempotencyLockLatencyMS.Count != 1 ||
		snapshot.RepositoryFindExistingLatencyMS.Count != 1 ||
		snapshot.RepositoryEnsureSeqLatencyMS.Count != 1 ||
		snapshot.RepositoryAllocateSeqLatencyMS.Count != 1 ||
		snapshot.RepositoryInsertMessageLatencyMS.Count != 1 ||
		snapshot.RepositoryInsertTimelineLatencyMS.Count != 1 ||
		snapshot.RepositoryInsertOutboxLatencyMS.Count != 1 {
		t.Fatalf("missing repository stage snapshots: %+v", snapshot)
	}
	if snapshot.RepositoryCommitLatencyMS.Count != 1 ||
		snapshot.RepositoryCommitLatencyMS.AvgMS != 4 {
		t.Fatalf("unexpected repository commit snapshot: %+v", snapshot.RepositoryCommitLatencyMS)
	}
	if snapshot.ConversationSeqAllocLatencyMS.Count != 2 ||
		snapshot.ConversationSeqAllocLatencyMS.AvgMS != 15 ||
		snapshot.ConversationSeqAllocLatencyMS.P95MS != 20 {
		t.Fatalf("unexpected seq alloc snapshot: %+v", snapshot.ConversationSeqAllocLatencyMS)
	}
	if snapshot.KafkaPublishLatencyMS.Count != 1 ||
		snapshot.KafkaPublishLatencyMS.AvgMS != 30 {
		t.Fatalf("unexpected kafka snapshot: %+v", snapshot.KafkaPublishLatencyMS)
	}
}

func TestCollectorServeHTTP(t *testing.T) {
	collector := NewCollector()
	collector.ObserveSendMessage(7 * time.Millisecond)
	collector.ObserveKafkaPublish(5 * time.Millisecond)

	request := httptest.NewRequest(http.MethodGet, "/debug/metrics", nil)
	response := httptest.NewRecorder()
	collector.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if snapshot.SendMessageLatencyMS.Count != 1 ||
		snapshot.KafkaPublishLatencyMS.Count != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}
