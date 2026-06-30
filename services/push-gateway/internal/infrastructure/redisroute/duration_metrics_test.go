package redisroute

import (
	"testing"
	"time"
)

func TestDurationMetricsRecordsCumulativeBuckets(t *testing.T) {
	var metrics durationMetrics
	recordDuration(&metrics, 750*time.Microsecond)
	recordDuration(&metrics, 3*time.Millisecond)
	recordDuration(&metrics, 1200*time.Millisecond)

	snapshot := snapshotDuration(&metrics)
	if snapshot.Count != 3 {
		t.Fatalf("count = %d", snapshot.Count)
	}
	if snapshot.MaxMS != 1200 {
		t.Fatalf("max = %v", snapshot.MaxMS)
	}
	if got := routeBucketCount(snapshot, "1"); got != 1 {
		t.Fatalf("<=1ms bucket = %d", got)
	}
	if got := routeBucketCount(snapshot, "5"); got != 2 {
		t.Fatalf("<=5ms bucket = %d", got)
	}
	if got := routeBucketCount(snapshot, "+Inf"); got != 3 {
		t.Fatalf("+Inf bucket = %d", got)
	}
}

func routeBucketCount(snapshot DurationSnapshot, le string) uint64 {
	for _, bucket := range snapshot.Buckets {
		if bucket.LE == le {
			return bucket.Count
		}
	}
	return 0
}
