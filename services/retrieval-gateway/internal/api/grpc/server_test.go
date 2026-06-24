package grpc

import (
	"context"
	"errors"
	"testing"

	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
				RerankScore:     0.9,
				DedupeReason:    types.EvidenceDedupeUniqueSource,
				MemoryGraphEdges: []types.MemoryGraphEdge{{
					EdgeID:            "edge-1",
					FromMemoryEventID: "mem-1",
					ToMemoryEventID:   "mem-2",
					RelationType:      "SUPPORTS",
					Confidence:        0.8,
					SourceRefs: []types.EvidenceSourceRef{{
						SourceType:      "MESSAGE",
						SourceID:        "msg-1",
						ConversationID:  "conv-1",
						ConversationSeq: 2,
					}},
				}},
			}, {
				EvidenceID:            "vector:vitem-1",
				SourceType:            types.EvidenceSourceVectorItem,
				SourceID:              "vitem-1",
				VectorItemRef:         "vitem-1",
				VectorSourceRefHash:   "sha256:source",
				VectorSourceService:   "memory-service",
				VectorCollectionType:  types.VectorCollectionMemoryEvent,
				VectorTombstoneStatus: "NONE",
				Score:                 0.88,
				VisibilityVersion:     7,
				SourceRefs: []types.EvidenceSourceRef{{
					SourceType: "VECTOR_SOURCE_REF_HASH",
					SourceID:   "sha256:source",
				}},
			}},
			SourceCoverage: []types.EvidenceSourceCoverage{{
				SourceType:     types.EvidenceSourceSearchMessage,
				Requested:      true,
				CandidateCount: 2,
				ReturnedCount:  1,
				DedupedCount:   1,
				Status:         types.EvidenceCoverageReturned,
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
	vectorItem := response.GetPack().GetItems()[1]
	if vectorItem.GetSourceType() != retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_VECTOR_ITEM ||
		vectorItem.GetVectorItemRef() != "vitem-1" ||
		vectorItem.GetVectorSourceRefHash() != "sha256:source" ||
		vectorItem.GetVectorSourceService() != "memory-service" ||
		vectorItem.GetVectorCollectionType() != types.VectorCollectionMemoryEvent ||
		vectorItem.GetVectorTombstoneStatus() != "NONE" {
		t.Fatalf("vector evidence item not mapped: %+v", vectorItem)
	}
	if got := response.GetPack().GetItems()[0].GetRerankScore(); got != 0.9 {
		t.Fatalf("unexpected rerank score %v", got)
	}
	if got := response.GetPack().GetItems()[0].GetDedupeReason(); got != types.EvidenceDedupeUniqueSource {
		t.Fatalf("unexpected dedupe reason %q", got)
	}
	edges := response.GetPack().GetItems()[0].GetMemoryGraphEdges()
	if len(edges) != 1 || edges[0].GetRelationType() != "SUPPORTS" || len(edges[0].GetSourceRefs()) != 1 {
		t.Fatalf("unexpected graph edges: %+v", edges)
	}
	coverage := response.GetPack().GetSourceCoverage()
	if len(coverage) != 1 {
		t.Fatalf("expected one source coverage row, got %d", len(coverage))
	}
	if got := coverage[0].GetStatus(); got != retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_RETURNED {
		t.Fatalf("unexpected coverage status %v", got)
	}
	if coverage[0].GetCandidateCount() != 2 || coverage[0].GetReturnedCount() != 1 || coverage[0].GetDedupedCount() != 1 {
		t.Fatalf("unexpected coverage counts: %+v", coverage[0])
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

func TestRetrieveEvidenceMapsVectorUnavailable(t *testing.T) {
	_, err := NewServer(fakeRetrieveExecutor{err: types.ErrVectorUnavailable}).RetrieveEvidence(context.Background(), validRequest())
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
	if status.Convert(err).Message() != "vector unavailable" {
		t.Fatalf("unexpected public error %q", status.Convert(err).Message())
	}
}

func TestRetrieveEvidenceMapsAtConversationSeq(t *testing.T) {
	executor := &recordingRetrieveExecutor{result: types.RetrieveEvidenceResult{
		Pack: types.EvidencePack{PackID: "ep_1", RetrievalVersion: types.RetrievalVersion},
	}}
	request := validRequest()
	request.AtConversationSeq = 42
	_, err := NewServer(executor).RetrieveEvidence(context.Background(), request)
	if err != nil {
		t.Fatalf("RetrieveEvidence returned error: %v", err)
	}
	if got := executor.command.AtConversationSeq; got != 42 {
		t.Fatalf("expected at_conversation_seq 42, got %d", got)
	}
}

func TestRetrieveEvidenceMapsVectorRequestFields(t *testing.T) {
	executor := &recordingRetrieveExecutor{result: types.RetrieveEvidenceResult{
		Pack: types.EvidencePack{PackID: "ep_1", RetrievalVersion: types.RetrievalVersion},
	}}
	request := validRequest()
	request.IncludeVector = true
	request.QueryEmbeddingRef = "embedding-ref-1"
	request.VectorCollectionTypes = []string{types.VectorCollectionMemoryEvent}
	request.VectorVisibilityScope = "tenant:tenant-1:user:user-1"
	request.VectorPolicyVersion = "policy-v1"
	request.VectorMinScore = 0.35
	_, err := NewServer(executor).RetrieveEvidence(context.Background(), request)
	if err != nil {
		t.Fatalf("RetrieveEvidence returned error: %v", err)
	}
	command := executor.command
	if !command.IncludeVector ||
		command.QueryEmbeddingRef != "embedding-ref-1" ||
		len(command.VectorCollections) != 1 ||
		command.VectorCollections[0] != types.VectorCollectionMemoryEvent ||
		command.VectorVisibilityScope != "tenant:tenant-1:user:user-1" ||
		command.VectorPolicyVersion != "policy-v1" ||
		command.VectorMinScore != 0.35 {
		t.Fatalf("vector request fields not mapped: %+v", command)
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

type recordingRetrieveExecutor struct {
	result  types.RetrieveEvidenceResult
	command types.RetrieveEvidenceCommand
}

func (executor *recordingRetrieveExecutor) Execute(_ context.Context, command types.RetrieveEvidenceCommand) (types.RetrieveEvidenceResult, error) {
	executor.command = command
	return executor.result, nil
}
