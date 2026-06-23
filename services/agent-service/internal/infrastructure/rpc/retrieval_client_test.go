package rpc

import (
	"context"
	"testing"
	"time"

	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
	"google.golang.org/grpc"
)

func TestRetrievalClientForwardsAtConversationSeq(t *testing.T) {
	fake := &fakeRetrievalGatewayClient{
		response: &retrievalv1.RetrieveEvidenceResponse{
			Pack: &retrievalv1.EvidencePack{
				PackId: "pack-1",
				Items: []*retrievalv1.EvidenceItem{{
					EvidenceId:      "memory:mem-1",
					SourceType:      retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT,
					SourceId:        "mem-1",
					MemoryEventId:   "mem-1",
					ConversationId:  "conv-1",
					ConversationSeq: 4,
					MemoryGraphEdges: []*retrievalv1.EvidenceMemoryGraphEdge{{
						EdgeId:            "edge-1",
						FromMemoryEventId: "mem-1",
						ToMemoryEventId:   "mem-2",
						RelationType:      "SUPPORTS",
						Confidence:        0.91,
						SourceRefs: []*retrievalv1.EvidenceSourceRef{{
							SourceType:      "MESSAGE",
							SourceId:        "msg-1",
							SourceEventId:   "evt-1",
							ConversationId:  "conv-1",
							ConversationSeq: 4,
						}},
					}},
				}},
			},
		},
	}
	client := NewRetrievalClient(fake, time.Second)

	result, err := client.RetrieveEvidence(context.Background(), types.RetrieveEvidenceQuery{
		AuthContext:       types.AuthContext{TenantID: "tenant-1", UserID: "user-1"},
		Query:             "launch",
		ConversationID:    "conv-1",
		AfterSeq:          3,
		AtConversationSeq: 21,
		Limit:             5,
		IncludeSearch:     true,
		IncludeMemory:     true,
		MemoryStatuses:    []string{types.MemoryStatusActive},
	})
	if err != nil {
		t.Fatalf("RetrieveEvidence returned error: %v", err)
	}
	if result.Pack.PackID != "pack-1" {
		t.Fatalf("unexpected pack: %+v", result.Pack)
	}
	if got := result.Pack.Items[0].MemoryGraphEdges; len(got) != 1 || got[0].RelationType != "SUPPORTS" || len(got[0].SourceRefs) != 1 {
		t.Fatalf("memory graph edge not mapped: %+v", got)
	}
	if fake.request.GetAtConversationSeq() != 21 {
		t.Fatalf("at_conversation_seq not forwarded: %+v", fake.request)
	}
}

type fakeRetrievalGatewayClient struct {
	request  *retrievalv1.RetrieveEvidenceRequest
	response *retrievalv1.RetrieveEvidenceResponse
	err      error
}

func (client *fakeRetrievalGatewayClient) RetrieveEvidence(
	_ context.Context,
	request *retrievalv1.RetrieveEvidenceRequest,
	_ ...grpc.CallOption,
) (*retrievalv1.RetrieveEvidenceResponse, error) {
	client.request = request
	if client.err != nil {
		return nil, client.err
	}
	return client.response, nil
}
