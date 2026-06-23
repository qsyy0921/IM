package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	ragv1 "github.com/qsyy0921/IM/api/proto/nexusim/rag/v1"
	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/services/rag-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAnswerQuestionMapsResult(t *testing.T) {
	now := time.UnixMilli(1710000000000)
	executor := &fakeAnswerExecutor{result: types.AnswerQuestionResult{
		AnswerID:   "ans-1",
		Status:     types.AnswerStatusGrounded,
		AnswerText: "Grounded extractive answer: [1] answer",
		Confidence: 0.75,
		Citations: []types.Citation{{
			EvidenceID:      "search:msg-1",
			SourceType:      types.EvidenceSourceSearchMessage,
			SourceID:        "msg-1",
			SourceEventID:   "evt-1",
			ConversationID:  "conv-1",
			ConversationSeq: 4,
			OccurredAt:      now,
		}},
		EvidencePack: types.EvidencePack{
			PackID:         "ep-1",
			TenantID:       "tenant-1",
			Query:          "question?",
			ConversationID: "conv-1",
			Items: []types.EvidenceItem{{
				EvidenceID:      "search:msg-1",
				SourceType:      types.EvidenceSourceSearchMessage,
				SourceID:        "msg-1",
				ConversationID:  "conv-1",
				ConversationSeq: 4,
				Text:            "answer",
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
						ConversationSeq: 4,
					}},
				}},
			}},
		},
		RAGVersion:     types.RAGVersion,
		GeneratedByLLM: false,
	}}
	server := NewServer(executor)
	response, err := server.AnswerQuestion(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("AnswerQuestion returned error: %v", err)
	}
	if response.GetStatus() != ragv1.AnswerStatus_ANSWER_STATUS_GROUNDED {
		t.Fatalf("unexpected status %s", response.GetStatus())
	}
	if response.GetGeneratedByLlm() {
		t.Fatal("first-stage response should not be marked LLM-generated")
	}
	if response.GetEvidencePack().GetItems()[0].GetText() != "answer" {
		t.Fatalf("evidence pack not mapped: %+v", response.GetEvidencePack())
	}
	edges := response.GetEvidencePack().GetItems()[0].GetMemoryGraphEdges()
	if len(edges) != 1 || edges[0].GetRelationType() != "SUPPORTS" || len(edges[0].GetSourceRefs()) != 1 {
		t.Fatalf("memory graph edge not mapped: %+v", edges)
	}
	if response.GetCitations()[0].GetSourceEventId() != "evt-1" {
		t.Fatalf("citation not mapped: %+v", response.GetCitations()[0])
	}
	if executor.command.AtConversationSeq != 11 {
		t.Fatalf("expected at_conversation_seq to be mapped, got %+v", executor.command)
	}
}

func TestAnswerQuestionRequiresAuthContext(t *testing.T) {
	_, err := NewServer(&fakeAnswerExecutor{}).AnswerQuestion(context.Background(), &ragv1.AnswerQuestionRequest{
		Question: "question?",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestAnswerQuestionMapsRetrievalUnavailable(t *testing.T) {
	_, err := NewServer(&fakeAnswerExecutor{err: types.ErrRetrievalUnavailable}).AnswerQuestion(context.Background(), validRequest())
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func TestAnswerQuestionMapsCitationVerificationFailure(t *testing.T) {
	_, err := NewServer(&fakeAnswerExecutor{err: types.ErrCitationVerification}).AnswerQuestion(context.Background(), validRequest())
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal, got %v", err)
	}
	if status.Convert(err).Message() != "rag unavailable" {
		t.Fatalf("unexpected public message: %q", status.Convert(err).Message())
	}
}

func validRequest() *ragv1.AnswerQuestionRequest {
	return &ragv1.AnswerQuestionRequest{
		AuthContext: &retrievalv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		Question:          "question?",
		ConversationId:    "conv-1",
		AtConversationSeq: 11,
		Limit:             3,
	}
}

type fakeAnswerExecutor struct {
	command types.AnswerQuestionCommand
	result  types.AnswerQuestionResult
	err     error
}

func (executor *fakeAnswerExecutor) Execute(
	_ context.Context,
	command types.AnswerQuestionCommand,
) (types.AnswerQuestionResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.AnswerQuestionResult{}, executor.err
	}
	if executor.result.AnswerID == "" {
		return types.AnswerQuestionResult{}, errors.New("missing fake result")
	}
	return executor.result, nil
}
