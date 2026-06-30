package types

import (
	"testing"
	"time"
)

func TestWebSocketWriterMetricsRecordsWriteDurationBuckets(t *testing.T) {
	var metrics WebSocketWriterMetrics
	metrics.RecordFrameWriteDuration(ServerFrame{Op: OpServerPong}, 750*time.Microsecond)
	metrics.RecordFrameWriteDuration(ServerFrame{Op: OpDeliveryNotify}, 3*time.Millisecond)
	metrics.RecordFrameWriteDuration(ServerFrame{Op: OpDeliveryNotify}, 1200*time.Millisecond)

	snapshot := metrics.Snapshot()
	if snapshot.FrameWriteDuration.Count != 3 {
		t.Fatalf("frame write count = %d", snapshot.FrameWriteDuration.Count)
	}
	if snapshot.DeliveryNotifyWriteDuration.Count != 2 {
		t.Fatalf("delivery notify count = %d", snapshot.DeliveryNotifyWriteDuration.Count)
	}
	if snapshot.FrameWriteDuration.MaxMS != 1200 {
		t.Fatalf("frame write max = %v", snapshot.FrameWriteDuration.MaxMS)
	}
	if snapshot.DeliveryNotifyWriteDuration.MaxMS != 1200 {
		t.Fatalf("delivery notify max = %v", snapshot.DeliveryNotifyWriteDuration.MaxMS)
	}
	if got := bucketCount(snapshot.FrameWriteDuration, "1"); got != 1 {
		t.Fatalf("frame write <=1ms bucket = %d", got)
	}
	if got := bucketCount(snapshot.FrameWriteDuration, "5"); got != 2 {
		t.Fatalf("frame write <=5ms bucket = %d", got)
	}
	if got := bucketCount(snapshot.FrameWriteDuration, "+Inf"); got != 3 {
		t.Fatalf("frame write +Inf bucket = %d", got)
	}
	if got := bucketCount(snapshot.DeliveryNotifyWriteDuration, "5"); got != 1 {
		t.Fatalf("delivery notify <=5ms bucket = %d", got)
	}
	if got := bucketCount(snapshot.DeliveryNotifyWriteDuration, "+Inf"); got != 2 {
		t.Fatalf("delivery notify +Inf bucket = %d", got)
	}
}

func bucketCount(snapshot WebSocketWriterDurationSnapshot, le string) uint64 {
	for _, bucket := range snapshot.Buckets {
		if bucket.LE == le {
			return bucket.Count
		}
	}
	return 0
}
