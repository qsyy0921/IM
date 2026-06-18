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
	server := NewServer(fakeGenerateExecutor{result: types.GenerateConversationSummaryResult{
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
	}})
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
}

func TestGenerateConversationSummaryRequiresAuthContext(t *testing.T) {
	_, err := NewServer(fakeGenerateExecutor{}).GenerateConversationSummary(context.Background(), &summaryv1.GenerateConversationSummaryRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestGenerateConversationSummaryMapsCitationVerificationFailure(t *testing.T) {
	_, err := NewServer(fakeGenerateExecutor{err: types.ErrCitationVerification}).GenerateConversationSummary(context.Background(), validRequest())
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
		ConversationId: "conv-1",
		Focus:          "release recap",
		Limit:          3,
	}
}

type fakeGenerateExecutor struct {
	result types.GenerateConversationSummaryResult
	err    error
}

func (executor fakeGenerateExecutor) Execute(
	_ context.Context,
	_ types.GenerateConversationSummaryCommand,
) (types.GenerateConversationSummaryResult, error) {
	if executor.err != nil {
		return types.GenerateConversationSummaryResult{}, executor.err
	}
	return executor.result, nil
}
