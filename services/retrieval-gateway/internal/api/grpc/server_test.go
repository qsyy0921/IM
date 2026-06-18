package grpc

import (
	"context"
	"errors"
	"testing"

	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
)

func TestRetrieveEvidenceMapsResult(t *testing.T) {
	server := NewServer(fakeRetrieveExecutor{result: types.RetrieveEvidenceResult{
		Pack: types.EvidencePack{
			PackID:           "ep_1",
			TenantID:         "tenant-1",
			Query:            "launch",
			ConversationID:   "conv-1",
			RetrievalVersion: types.RetrievalVersion,
			Items: []types.EvidenceItem{{
				EvidenceID:      "search:msg-1",
				SourceType:      types.EvidenceSourceSearchMessage,
				SourceID:        "msg-1",
				ConversationID:  "conv-1",
				ConversationSeq: 2,
				Text:            "hello",
				MessageID:       "msg-1",
			}},
		},
	}})
	response, err := server.RetrieveEvidence(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("RetrieveEvidence returned error: %v", err)
	}
	if response.GetPack().GetPackId() != "ep_1" {
		t.Fatalf("unexpected pack id %q", response.GetPack().GetPackId())
	}
	if got := response.GetPack().GetItems()[0].GetSourceType(); got != retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE {
		t.Fatalf("unexpected source type %v", got)
	}
}

func TestRetrieveEvidenceRequiresAuthContext(t *testing.T) {
	_, err := NewServer(fakeRetrieveExecutor{}).RetrieveEvidence(context.Background(), &retrievalv1.RetrieveEvidenceRequest{Query: "x"})
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestRetrieveEvidenceMapsUnavailable(t *testing.T) {
	_, err := NewServer(fakeRetrieveExecutor{err: types.ErrMemoryUnavailable}).RetrieveEvidence(context.Background(), validRequest())
	if err == nil {
		t.Fatal("expected error")
	}
}

func validRequest() *retrievalv1.RetrieveEvidenceRequest {
	return &retrievalv1.RetrieveEvidenceRequest{
		AuthContext: &retrievalv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		Query: "launch",
		Limit: 10,
	}
}

type fakeRetrieveExecutor struct {
	result types.RetrieveEvidenceResult
	err    error
}

func (executor fakeRetrieveExecutor) Execute(context.Context, types.RetrieveEvidenceCommand) (types.RetrieveEvidenceResult, error) {
	if executor.err != nil {
		return types.RetrieveEvidenceResult{}, executor.err
	}
	if executor.result.Pack.PackID == "" {
		return types.RetrieveEvidenceResult{}, errors.New("missing fake result")
	}
	return executor.result, nil
}
