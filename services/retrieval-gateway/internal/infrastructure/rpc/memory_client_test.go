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

func TestMemoryClientGetsGraphEdges(t *testing.T) {
	fake := &fakeMemoryServiceClient{
		getResponse: &memoryv1.GetMemoryEventResponse{
			Item: &memoryv1.StructuredMemoryEvent{
				MemoryEventId: "mem-1",
				Status:        memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE,
				FactText:      "memory fact",
			},
			GraphEdges: []*memoryv1.MemoryGraphEdge{{
				EdgeId:            "edge-1",
				FromMemoryEventId: "mem-1",
				ToMemoryEventId:   "mem-2",
				RelationType:      "SUPPORTS",
				Confidence:        0.9,
				SourceRefs: []*memoryv1.SourceRef{{
					SourceType:      memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_MESSAGE,
					SourceId:        "msg-1",
					ConversationId:  "conv-1",
					ConversationSeq: 2,
				}},
			}},
		},
	}
	result, err := NewMemoryClient(fake, time.Second).GetMemoryEvent(context.Background(), types.MemoryEventLookup{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		MemoryEventID: "mem-1",
	})
	if err != nil {
		t.Fatalf("GetMemoryEvent returned error: %v", err)
	}
	if fake.getRequest.GetMemoryEventId() != "mem-1" {
		t.Fatalf("unexpected get request: %+v", fake.getRequest)
	}
	if result.Item.MemoryEventID != "mem-1" || len(result.GraphEdges) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if edge := result.GraphEdges[0]; edge.RelationType != "SUPPORTS" || len(edge.SourceRefs) != 1 || edge.SourceRefs[0].SourceID != "msg-1" {
		t.Fatalf("unexpected graph edge: %+v", edge)
	}
}

func TestMemoryClientListsProfileAggregates(t *testing.T) {
	fake := &fakeMemoryServiceClient{
		listProfilesResponse: &memoryv1.ListProfileAggregatesResponse{
			Items: []*memoryv1.ProfileAggregate{{
				ProfileId:                "profile-1",
				SubjectUserId:            "user-1",
				AggregateType:            "SKILL",
				AggregateKey:             "phoenix-launch",
				Status:                   memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE,
				ReviewState:              memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_APPROVED,
				SummaryText:              "reviewed cross-group skill profile",
				SupportingMemoryEventIds: []string{"mem-1", "mem-2"},
				Confidence:               0.91,
				UpdatedAtUnixMs:          2000,
			}},
		},
	}
	result, err := NewMemoryClient(fake, time.Second).ListProfileAggregates(context.Background(), types.ProfileAggregateQuery{
		AuthContext:   types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SubjectUserID: "user-1",
		Statuses:      []string{types.MemoryStatusActive},
		Limit:         5,
	})
	if err != nil {
		t.Fatalf("ListProfileAggregates returned error: %v", err)
	}
	if fake.listProfilesRequest.GetSubjectUserId() != "user-1" {
		t.Fatalf("unexpected list profiles request: %+v", fake.listProfilesRequest)
	}
	if len(result.Items) != 1 {
		t.Fatalf("unexpected profile result: %+v", result)
	}
	item := result.Items[0]
	if item.ProfileID != "profile-1" ||
		item.SubjectUserID != "user-1" ||
		item.AggregateType != "SKILL" ||
		len(item.SupportingMemoryEventIDs) != 2 ||
		item.ReviewState != "APPROVED" ||
		item.UpdatedAt.IsZero() {
		t.Fatalf("profile aggregate not mapped: %+v", item)
	}
}

type fakeMemoryServiceClient struct {
	request              *memoryv1.QueryMemoryEventsRequest
	response             *memoryv1.QueryMemoryEventsResponse
	getRequest           *memoryv1.GetMemoryEventRequest
	getResponse          *memoryv1.GetMemoryEventResponse
	listProfilesRequest  *memoryv1.ListProfileAggregatesRequest
	listProfilesResponse *memoryv1.ListProfileAggregatesResponse
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
	_ context.Context,
	request *memoryv1.GetMemoryEventRequest,
	_ ...grpc.CallOption,
) (*memoryv1.GetMemoryEventResponse, error) {
	client.getRequest = request
	return client.getResponse, nil
}

func (client *fakeMemoryServiceClient) ListProfileAggregates(
	_ context.Context,
	request *memoryv1.ListProfileAggregatesRequest,
	_ ...grpc.CallOption,
) (*memoryv1.ListProfileAggregatesResponse, error) {
	client.listProfilesRequest = request
	return client.listProfilesResponse, nil
}
