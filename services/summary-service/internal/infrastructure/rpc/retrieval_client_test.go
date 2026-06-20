package rpc

import (
	"context"
	"testing"
	"time"

	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/services/summary-service/internal/types"
	"google.golang.org/grpc"
)

func TestRetrievalClientForwardsAtConversationSeq(t *testing.T) {
	fake := &fakeRetrievalGatewayClient{
		response: &retrievalv1.RetrieveEvidenceResponse{
			Pack: &retrievalv1.EvidencePack{PackId: "pack-1"},
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
