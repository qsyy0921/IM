package grpc

import (
	"context"
	"testing"

	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	summaryv1 "github.com/qsyy0921/IM/api/proto/nexusim/summary/v1"
	"github.com/qsyy0921/IM/services/summary-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGenerateConversationSummaryMapsResult(t *testing.T) {
	executor := &fakeGenerateExecutor{result: types.GenerateConversationSummaryResult{
		SummaryID:      "sum-1",
		Status:         types.SummaryStatusGrounded,
		SummaryText:    "summary",
		Confidence:     0.8,
		SummaryVersion: types.SummaryVersion,
		Citations: []types.Citation{{
			EvidenceID:      "evidence-1",
			SourceType:      types.EvidenceSourceSearchMessage,
			SourceID:        "msg-1",
			ConversationID:  "conv-1",
			ConversationSeq: 2,
		}},
		EvidencePack: types.EvidencePack{
			PackID:         "pack-1",
			TenantID:       "tenant-1",
			ConversationID: "conv-1",
			Items: []types.EvidenceItem{{
				EvidenceID:      "memory:mem-1",
				SourceType:      types.EvidenceSourceMemoryEvent,
				SourceID:        "mem-1",
				MemoryEventID:   "mem-1",
				ConversationID:  "conv-1",
				ConversationSeq: 2,
				MemoryGraphEdges: []types.MemoryGraphEdge{{
					EdgeID:            "edge-1",
					FromMemoryEventID: "mem-1",
					ToMemoryEventID:   "mem-2",
					RelationType:      "SUPPORTS",
					Confidence:        0.91,
					SourceRefs: []types.EvidenceSourceRef{{
						SourceType:      types.EvidenceSourceSearchMessage,
						SourceID:        "msg-1",
						SourceEventID:   "evt-1",
						ConversationID:  "conv-1",
						ConversationSeq: 2,
					}},
				}},
			}},
		},
	}}
	server := NewServer(executor)
	response, err := server.GenerateConversationSummary(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("GenerateConversationSummary returned error: %v", err)
	}
	if response.GetSummaryId() != "sum-1" || response.GetSummaryText() != "summary" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.GetStatus() != summaryv1.SummaryStatus_SUMMARY_STATUS_GROUNDED {
		t.Fatalf("unexpected status: %s", response.GetStatus())
	}
	if len(response.GetCitations()) != 1 {
		t.Fatalf("expected citation, got %#v", response.GetCitations())
	}
	edges := response.GetEvidencePack().GetItems()[0].GetMemoryGraphEdges()
	if len(edges) != 1 || edges[0].GetRelationType() != "SUPPORTS" || len(edges[0].GetSourceRefs()) != 1 {
		t.Fatalf("memory graph edge not mapped: %+v", edges)
	}
	if executor.command.AtConversationSeq != 13 {
		t.Fatalf("expected at_conversation_seq to be mapped, got %+v", executor.command)
	}
}

func TestGenerateConversationSummaryRequiresAuthContext(t *testing.T) {
	_, err := NewServer(&fakeGenerateExecutor{}).GenerateConversationSummary(context.Background(), &summaryv1.GenerateConversationSummaryRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestGenerateConversationSummaryMapsCitationVerificationFailure(t *testing.T) {
	_, err := NewServer(&fakeGenerateExecutor{err: types.ErrCitationVerification}).GenerateConversationSummary(context.Background(), validRequest())
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal, got %v", err)
	}
	if status.Convert(err).Message() != "summary unavailable" {
		t.Fatalf("unexpected public message: %q", status.Convert(err).Message())
	}
}

func validRequest() *summaryv1.GenerateConversationSummaryRequest {
	return &summaryv1.GenerateConversationSummaryRequest{
		AuthContext: &retrievalv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		ConversationId:    "conv-1",
		Focus:             "release recap",
		AtConversationSeq: 13,
		Limit:             3,
	}
}

type fakeGenerateExecutor struct {
	command types.GenerateConversationSummaryCommand
	result  types.GenerateConversationSummaryResult
	err     error
}

func (executor *fakeGenerateExecutor) Execute(
	_ context.Context,
	command types.GenerateConversationSummaryCommand,
) (types.GenerateConversationSummaryResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.GenerateConversationSummaryResult{}, executor.err
	}
	return executor.result, nil
}
