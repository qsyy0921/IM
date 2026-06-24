package tool

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestConversationNoteExecutorCreatesNoteWithoutEchoingBody(t *testing.T) {
	server := grpc.NewServer()
	fake := &fakeConversationNoteServer{}
	conversationv1.RegisterConversationServiceServer(server, fake)
	listener := bufconn.Listen(1024 * 1024)
	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Stop()
	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	defer conn.Close()

	executor := NewConversationNoteExecutor(conversationv1.NewConversationServiceClient(conn), time.Second)
	result, err := executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-tool",
			UserID:   "agent-user",
		},
		Skill: types.SkillDefinition{
			AllowedActions: []string{types.ToolActionExecute},
		},
		ProposalID:     "proposal-1",
		ApprovalID:     "approval-1",
		ToolName:       types.ConversationNoteCreateToolName,
		ResourceType:   "conversation",
		ResourceID:     "conv-1",
		RiskLevel:      "LOW",
		IdempotencyKey: "approval-1:input",
		InputJSON:      `{"body":" approved rollout note "}`,
		InputSHA256:    "abc123",
	})
	if err != nil {
		t.Fatalf("execute conversation note: %v", err)
	}
	if !result.Executed {
		t.Fatalf("expected execution result: %+v", result)
	}
	if fake.request.GetBody() != "approved rollout note" ||
		fake.request.GetConversationId() != "conv-1" ||
		fake.request.GetSourceProposalId() != "proposal-1" ||
		fake.request.GetSourceApprovalId() != "approval-1" {
		t.Fatalf("unexpected request: %+v", fake.request)
	}
	if strings.Contains(result.OutputJSON, "approved rollout note") {
		t.Fatalf("tool output must not echo note body: %s", result.OutputJSON)
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(result.OutputJSON), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if output["adapter"] != "conversation-note" || output["note_id"] != "note-1" || output["input_sha256"] != "abc123" {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestConversationNoteExecutorRejectsWrongResourceType(t *testing.T) {
	executor := NewConversationNoteExecutor(nil, time.Second)
	_, err := executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
		ToolName:     types.ConversationNoteCreateToolName,
		ResourceType: "conversation_note",
		RiskLevel:    "LOW",
		Skill:        types.SkillDefinition{AllowedActions: []string{types.ToolActionExecute}},
	})
	if err != types.ErrToolExecutionUnsupported {
		t.Fatalf("expected unsupported, got %v", err)
	}
}

type fakeConversationNoteServer struct {
	conversationv1.UnimplementedConversationServiceServer
	request *conversationv1.CreateConversationNoteRequest
}

func (server *fakeConversationNoteServer) CreateConversationNote(
	_ context.Context,
	request *conversationv1.CreateConversationNoteRequest,
) (*conversationv1.CreateConversationNoteResponse, error) {
	server.request = request
	return &conversationv1.CreateConversationNoteResponse{
		Note: &conversationv1.ConversationNote{
			TenantId:         request.GetAuthContext().GetTenantId(),
			ConversationId:   request.GetConversationId(),
			NoteId:           "note-1",
			AuthorUserId:     request.GetAuthContext().GetUserId(),
			SourceToolName:   request.GetSourceToolName(),
			SourceProposalId: request.GetSourceProposalId(),
			SourceApprovalId: request.GetSourceApprovalId(),
		},
	}, nil
}
