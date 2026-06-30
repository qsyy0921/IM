package redisroute

import (
	"sync/atomic"
	"time"
)

type DurationSnapshot struct {
	Count   uint64           `json:"count"`
	SumMS   float64          `json:"sum_ms"`
	MaxMS   float64          `json:"max_ms"`
	Buckets []DurationBucket `json:"buckets"`
}

type DurationBucket struct {
	LE    string `json:"le"`
	Count uint64 `json:"count"`
}

const routeDurationBucketCount = 14

var routeDurationUpperBoundsMS = [...]float64{
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

var routeDurationBucketLabels = [...]string{
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

type durationMetrics struct {
	count   atomic.Uint64
	sumNS   atomic.Uint64
	maxNS   atomic.Uint64
	buckets [routeDurationBucketCount]atomic.Uint64
}

func recordDuration(metrics *durationMetrics, duration time.Duration) {
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
	metrics.buckets[durationBucketIndex(duration)].Add(1)
}

func durationBucketIndex(duration time.Duration) int {
	ms := float64(duration.Nanoseconds()) / float64(time.Millisecond)
	for index, upperBoundMS := range routeDurationUpperBoundsMS {
		if ms <= upperBoundMS {
			return index
		}
	}
	return routeDurationBucketCount - 1
}

func snapshotDuration(metrics *durationMetrics) DurationSnapshot {
	count := metrics.count.Load()
	buckets := make([]DurationBucket, 0, routeDurationBucketCount)
	var cumulative uint64
	for index := 0; index < routeDurationBucketCount; index++ {
		cumulative += metrics.buckets[index].Load()
		buckets = append(buckets, DurationBucket{
			LE:    routeDurationBucketLabels[index],
			Count: cumulative,
		})
	}
	return DurationSnapshot{
		Count:   count,
		SumMS:   float64(metrics.sumNS.Load()) / float64(time.Millisecond),
		MaxMS:   float64(metrics.maxNS.Load()) / float64(time.Millisecond),
		Buckets: buckets,
	}
}
