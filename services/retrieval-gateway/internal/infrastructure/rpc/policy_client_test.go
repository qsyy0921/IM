package rpc

import (
	"context"
	"errors"
	"testing"
	"time"

	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPolicyClientCheckRetrieveEvidenceMapsToolAction(t *testing.T) {
	fake := &fakePolicyServiceClient{response: &policyv1.CheckToolActionResponse{
		Allowed:           true,
		RequiresApproval:  false,
		PermissionVersion: 42,
		Classification:    "ALLOW",
		Reason:            "ok",
		DecisionSource:    "TOOL_RULE",
	}}
	decision, err := NewPolicyClient(fake, time.Second).CheckRetrieveEvidence(context.Background(), policyCheck("conv-1"))
	if err != nil {
		t.Fatalf("CheckRetrieveEvidence returned error: %v", err)
	}
	if !decision.Allowed || decision.RequiresApproval || decision.PermissionVersion != 42 {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	request := fake.toolRequest
	if request == nil {
		t.Fatal("expected CheckToolAction request")
	}
	if request.GetToolName() != types.RetrievalPolicyToolName {
		t.Fatalf("unexpected tool name %q", request.GetToolName())
	}
	if request.GetAction() != policyv1.ToolAction_TOOL_ACTION_CALL {
		t.Fatalf("unexpected tool action %v", request.GetAction())
	}
	if request.GetResourceType() != types.RetrievalPolicyResourceTypeConversation || request.GetResourceId() != "conv-1" {
		t.Fatalf("unexpected resource %s/%s", request.GetResourceType(), request.GetResourceId())
	}
	if request.GetRiskLevel() != types.RetrievalPolicyRiskLow {
		t.Fatalf("unexpected risk level %q", request.GetRiskLevel())
	}
	if request.GetIntent() != types.RetrievalPolicyIntent {
		t.Fatalf("unexpected intent %q", request.GetIntent())
	}
}

func TestPolicyClientCheckRetrieveEvidenceUsesTenantResourceWithoutConversation(t *testing.T) {
	fake := &fakePolicyServiceClient{response: &policyv1.CheckToolActionResponse{Allowed: true}}
	check := policyCheck("")
	_, err := NewPolicyClient(fake, time.Second).CheckRetrieveEvidence(context.Background(), check)
	if err != nil {
		t.Fatalf("CheckRetrieveEvidence returned error: %v", err)
	}
	if fake.toolRequest.GetResourceType() != types.RetrievalPolicyResourceTypeTenant {
		t.Fatalf("unexpected resource type %q", fake.toolRequest.GetResourceType())
	}
	if fake.toolRequest.GetResourceId() != string(check.AuthContext.TenantID) {
		t.Fatalf("unexpected resource id %q", fake.toolRequest.GetResourceId())
	}
}

func TestPolicyClientCheckRetrieveEvidenceMapsPolicyError(t *testing.T) {
	fake := &fakePolicyServiceClient{err: status.Error(codes.Unavailable, "down")}
	_, err := NewPolicyClient(fake, time.Second).CheckRetrieveEvidence(context.Background(), policyCheck("conv-1"))
	if !errors.Is(err, types.ErrRetrievalUnavailable) {
		t.Fatalf("expected retrieval unavailable, got %v", err)
	}
}

func policyCheck(conversationID types.ConversationID) types.RetrievalPolicyCheck {
	return types.RetrievalPolicyCheck{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: conversationID,
	}
}

type fakePolicyServiceClient struct {
	toolRequest *policyv1.CheckToolActionRequest
	response    *policyv1.CheckToolActionResponse
	err         error
}

func (client *fakePolicyServiceClient) CheckToolAction(
	_ context.Context,
	request *policyv1.CheckToolActionRequest,
	_ ...grpc.CallOption,
) (*policyv1.CheckToolActionResponse, error) {
	client.toolRequest = request
	return client.response, client.err
}

func (client *fakePolicyServiceClient) CheckMessageAction(
	context.Context,
	*policyv1.CheckMessageActionRequest,
	...grpc.CallOption,
) (*policyv1.CheckMessageActionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused")
}
