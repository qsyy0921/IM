package rpc

import (
	"context"
	"testing"
	"time"

	memoryv1 "github.com/qsyy0921/IM/api/proto/nexusim/memory/v1"
	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
	"google.golang.org/grpc"
)

func TestMemoryClientSendsAtConversationSeq(t *testing.T) {
	fake := &fakeMemoryServiceClient{
		response: &memoryv1.QueryMemoryEventsResponse{
			Items: []*memoryv1.StructuredMemoryEvent{{
				MemoryEventId: "mem-1",
				Status:        memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE,
			}},
			ProjectionVersion: 9,
		},
	}
	result, err := NewMemoryClient(fake, time.Second).QueryMemoryEvents(context.Background(), types.MemoryQuery{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		Query:             "launch",
		ConversationID:    "conv-1",
		AfterValidFromSeq: 7,
		AtConversationSeq: 42,
		Statuses:          []string{types.MemoryStatusActive},
		Limit:             5,
	})
	if err != nil {
		t.Fatalf("QueryMemoryEvents returned error: %v", err)
	}
	if len(result.Items) != 1 || result.ProjectionVersion != 9 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if fake.request.GetAtConversationSeq() != 42 {
		t.Fatalf("expected at_conversation_seq 42, got %d", fake.request.GetAtConversationSeq())
	}
	if fake.request.GetAfterValidFromSeq() != 7 {
		t.Fatalf("expected after_valid_from_seq 7, got %d", fake.request.GetAfterValidFromSeq())
	}
	if len(fake.request.GetStatuses()) != 1 || fake.request.GetStatuses()[0] != memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE {
		t.Fatalf("expected active memory status, got %+v", fake.request.GetStatuses())
	}
}

type fakeMemoryServiceClient struct {
	request  *memoryv1.QueryMemoryEventsRequest
	response *memoryv1.QueryMemoryEventsResponse
}

func (client *fakeMemoryServiceClient) QueryMemoryEvents(
	_ context.Context,
	request *memoryv1.QueryMemoryEventsRequest,
	_ ...grpc.CallOption,
) (*memoryv1.QueryMemoryEventsResponse, error) {
	client.request = request
	return client.response, nil
}

func (client *fakeMemoryServiceClient) GetMemoryEvent(
	context.Context,
	*memoryv1.GetMemoryEventRequest,
	...grpc.CallOption,
) (*memoryv1.GetMemoryEventResponse, error) {
	return nil, nil
}

func (client *fakeMemoryServiceClient) ListProfileAggregates(
	context.Context,
	*memoryv1.ListProfileAggregatesRequest,
	...grpc.CallOption,
) (*memoryv1.ListProfileAggregatesResponse, error) {
	return nil, nil
}
