package kafka

import (
	"testing"

	kafkago "github.com/segmentio/kafka-go"
)

func TestEventTypeFromHeaders(t *testing.T) {
	headers := []kafkago.Header{
		{Key: "trace_id", Value: []byte("trace-1")},
		{Key: "nexusim-event-type", Value: []byte("knowledge.chunk.ready.v1")},
	}
	if got := eventTypeFromHeaders(headers); got != "knowledge.chunk.ready.v1" {
		t.Fatalf("unexpected event type: %s", got)
	}
}
