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
	collector.ObserveConversationSeqAlloc(10 * time.Millisecond)
	collector.ObserveConversationSeqAlloc(20 * time.Millisecond)
	collector.ObserveKafkaPublish(30 * time.Millisecond)

	snapshot := collector.Snapshot()
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
	if snapshot.KafkaPublishLatencyMS.Count != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}
