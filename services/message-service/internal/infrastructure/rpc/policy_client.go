package rpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	policyMetadataTraceID   = "x-nexusim-trace-id"
	policyMetadataRequestID = "x-nexusim-request-id"
)

type PolicyClient struct {
	client  policyv1.PolicyServiceClient
	timeout time.Duration
}

func NewPolicyClient(client policyv1.PolicyServiceClient, timeout time.Duration) PolicyClient {
	if timeout <= 0 {
		timeout = 30 * time.Millisecond
	}
	return PolicyClient{client: client, timeout: timeout}
}

func DialPolicyClient(ctx context.Context, addr string, timeout time.Duration) (PolicyClient, func() error, error) {
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return PolicyClient{}, nil, err
	}
	return NewPolicyClient(policyv1.NewPolicyServiceClient(conn), timeout), conn.Close, nil
}

func (c PolicyClient) CheckSendPermission(ctx context.Context, command types.SendMessageCommand) (types.PermissionDecision, error) {
	return c.checkMessageAction(ctx, command.AuthContext, command.ConversationID, "", policyv1.MessageAction_MESSAGE_ACTION_SEND)
}

func (c PolicyClient) CheckEditPermission(ctx context.Context, command types.EditMessageCommand) (types.PermissionDecision, error) {
	return c.checkMessageAction(ctx, command.AuthContext, command.ConversationID, types.MessageID(command.MessageID), policyv1.MessageAction_MESSAGE_ACTION_EDIT)
}

func (c PolicyClient) CheckRevokePermission(ctx context.Context, command types.RevokeMessageCommand) (types.PermissionDecision, error) {
	return c.checkMessageAction(ctx, command.AuthContext, command.ConversationID, command.MessageID, policyv1.MessageAction_MESSAGE_ACTION_REVOKE)
}

func (c PolicyClient) CheckDeletePermission(ctx context.Context, command types.DeleteMessageCommand) (types.PermissionDecision, error) {
	return c.checkMessageAction(ctx, command.AuthContext, command.ConversationID, command.MessageID, policyv1.MessageAction_MESSAGE_ACTION_DELETE)
}

func (c PolicyClient) checkMessageAction(
	ctx context.Context,
	auth types.AuthContext,
	conversationID types.ConversationID,
	messageID types.MessageID,
	action policyv1.MessageAction,
) (types.PermissionDecision, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	callCtx = policyOutgoingMetadataContext(callCtx, auth)

	response, err := c.client.CheckMessageAction(callCtx, &policyv1.CheckMessageActionRequest{
		AuthContext: &policyv1.AuthContext{
			TenantId:  string(auth.TenantID),
			UserId:    string(auth.UserID),
			DeviceId:  string(auth.DeviceID),
			SessionId: string(auth.SessionID),
			TraceId:   auth.TraceID,
			RequestId: auth.RequestID,
		},
		ConversationId: string(conversationID),
		Action:         action,
		MessageId:      string(messageID),
	})
	if err != nil {
		return types.PermissionDecision{}, mapPolicyError(err)
	}
	if err := validatePolicyResponse(auth, conversationID, messageID, action, response); err != nil {
		return types.PermissionDecision{}, err
	}
	return types.PermissionDecision{
		Allowed:           response.GetAllowed(),
		PermissionVersion: response.GetPermissionVersion(),
		Classification:    response.GetClassification(),
		Reason:            response.GetReason(),
	}, nil
}

func policyOutgoingMetadataContext(ctx context.Context, auth types.AuthContext) context.Context {
	pairs := make([]string, 0, 4)
	if auth.TraceID != "" {
		pairs = append(pairs, policyMetadataTraceID, auth.TraceID)
	}
	if auth.RequestID != "" {
		pairs = append(pairs, policyMetadataRequestID, auth.RequestID)
	}
	if len(pairs) == 0 {
		return ctx
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
}

func validatePolicyResponse(
	auth types.AuthContext,
	conversationID types.ConversationID,
	messageID types.MessageID,
	action policyv1.MessageAction,
	response *policyv1.CheckMessageActionResponse,
) error {
	if response == nil {
		return types.NewDependencyUnavailable("policy service returned empty response")
	}
	if response.GetTenantId() != string(auth.TenantID) ||
		response.GetUserId() != string(auth.UserID) ||
		response.GetConversationId() != string(conversationID) ||
		response.GetMessageId() != string(messageID) ||
		response.GetAction() != action {
		return types.NewDependencyUnavailable("policy service returned mismatched decision")
	}
	if response.GetPermissionVersion() <= 0 {
		return types.NewDependencyUnavailable("policy service returned invalid permission version")
	}
	if response.GetClassification() == "" {
		return types.NewDependencyUnavailable("policy service returned empty classification")
	}
	return nil
}

func mapPolicyError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return types.NewDependencyUnavailable("policy service deadline exceeded")
	}
	st, ok := status.FromError(err)
	if !ok {
		return types.NewDependencyUnavailable("policy service error")
	}
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.NewDependencyUnavailable("policy service unavailable")
	default:
		return types.NewDependencyUnavailable(fmt.Sprintf("policy service returned %s", st.Code()))
	}
}
