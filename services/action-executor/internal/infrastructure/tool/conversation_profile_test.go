package tool

import (
	"context"
	"encoding/json"
	"errors"
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

func TestConversationProfileExecutorUpdatesProfileWithoutEchoingFields(t *testing.T) {
	fake := &fakeConversationProfileServer{}
	server, conn := newConversationProfileTestClient(t, fake)
	defer server.Stop()
	defer conn.Close()

	executor := NewConversationProfileExecutor(conversationv1.NewConversationServiceClient(conn), time.Second)
	result, err := executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-tool",
			UserID:   "agent-user",
		},
		Skill: types.SkillDefinition{
			AllowedActions: []string{types.ToolActionExecute},
		},
		ToolName:     types.ConversationProfileUpdateToolName,
		ResourceType: "conversation",
		ResourceID:   "conv-1",
		RiskLevel:    "LOW",
		InputJSON: `{
			"title": " Approved title ",
			"avatar_uri": " media://asset/new ",
			"announcement": " Approved announcement ",
			"expected_profile_version": 7
		}`,
		InputSHA256: "abc123",
	})
	if err != nil {
		t.Fatalf("execute conversation profile: %v", err)
	}
	if !result.Executed {
		t.Fatalf("expected execution result: %+v", result)
	}
	if fake.request.GetConversationId() != "conv-1" ||
		fake.request.GetTitle() != "Approved title" ||
		fake.request.GetAvatarUri() != "media://asset/new" ||
		fake.request.GetAnnouncement() != "Approved announcement" ||
		fake.request.GetExpectedProfileVersion() != 7 {
		t.Fatalf("unexpected request: %+v", fake.request)
	}
	for _, sensitive := range []string{"Approved title", "media://asset/new", "Approved announcement"} {
		if strings.Contains(result.OutputJSON, sensitive) {
			t.Fatalf("tool output must not echo profile field %q: %s", sensitive, result.OutputJSON)
		}
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(result.OutputJSON), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if output["adapter"] != "conversation-profile" ||
		output["conversation_id"] != "conv-1" ||
		output["input_sha256"] != "abc123" ||
		output["profile_version"] != float64(8) {
		t.Fatalf("unexpected output: %+v", output)
	}
	if output["title_sha256"] == "" || output["announcement_sha256"] == "" {
		t.Fatalf("expected hashed profile fields: %+v", output)
	}
}

func TestConversationProfileExecutorRequiresExpectedVersion(t *testing.T) {
	fake := &fakeConversationProfileServer{}
	server, conn := newConversationProfileTestClient(t, fake)
	defer server.Stop()
	defer conn.Close()

	executor := NewConversationProfileExecutor(conversationv1.NewConversationServiceClient(conn), time.Second)
	_, err := executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
		ToolName:     types.ConversationProfileUpdateToolName,
		ResourceType: "conversation",
		ResourceID:   "conv-1",
		RiskLevel:    "LOW",
		Skill:        types.SkillDefinition{AllowedActions: []string{types.ToolActionExecute}},
		InputJSON:    `{"title":"new title"}`,
	})
	if !errors.Is(err, types.ErrToolExecutionFailed) {
		t.Fatalf("expected failed validation, got %v", err)
	}
}

func TestConversationProfileExecutorRejectsHigherRisk(t *testing.T) {
	executor := NewConversationProfileExecutor(nil, time.Second)
	_, err := executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
		ToolName:     types.ConversationProfileUpdateToolName,
		ResourceType: "conversation",
		ResourceID:   "conv-1",
		RiskLevel:    "HIGH",
		Skill:        types.SkillDefinition{AllowedActions: []string{types.ToolActionExecute}},
		InputJSON:    `{"title":"new title","expected_profile_version":1}`,
	})
	if !errors.Is(err, types.ErrToolExecutionUnsupported) {
		t.Fatalf("expected unsupported, got %v", err)
	}
}

func newConversationProfileTestClient(
	t *testing.T,
	fake *fakeConversationProfileServer,
) (*grpc.Server, *grpc.ClientConn) {
	t.Helper()
	server := grpc.NewServer()
	conversationv1.RegisterConversationServiceServer(server, fake)
	listener := bufconn.Listen(1024 * 1024)
	go func() {
		_ = server.Serve(listener)
	}()
	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		server.Stop()
		t.Fatalf("dial bufconn: %v", err)
	}
	return server, conn
}

type fakeConversationProfileServer struct {
	conversationv1.UnimplementedConversationServiceServer
	request *conversationv1.UpdateConversationProfileRequest
}

func (server *fakeConversationProfileServer) UpdateConversationProfile(
	_ context.Context,
	request *conversationv1.UpdateConversationProfileRequest,
) (*conversationv1.UpdateConversationProfileResponse, error) {
	server.request = request
	return &conversationv1.UpdateConversationProfileResponse{
		Profile: &conversationv1.ConversationProfile{
			TenantId:          request.GetAuthContext().GetTenantId(),
			ConversationId:    request.GetConversationId(),
			Title:             request.GetTitle(),
			AvatarUri:         request.GetAvatarUri(),
			Announcement:      request.GetAnnouncement(),
			ProfileVersion:    request.GetExpectedProfileVersion() + 1,
			MemberVersion:     11,
			PermissionVersion: 12,
		},
	}, nil
}
