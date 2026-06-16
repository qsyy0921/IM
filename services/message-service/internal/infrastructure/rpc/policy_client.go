package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
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

type PolicyClientDialConfig struct {
	Addr    string
	Timeout time.Duration
	TLS     PolicyClientTLSConfig
}

type PolicyClientTLSConfig = GRPCClientTLSConfig

func NewPolicyClient(client policyv1.PolicyServiceClient, timeout time.Duration) PolicyClient {
	if timeout <= 0 {
		timeout = 30 * time.Millisecond
	}
	return PolicyClient{client: client, timeout: timeout}
}

func DialPolicyClient(ctx context.Context, addr string, timeout time.Duration) (PolicyClient, func() error, error) {
	return DialPolicyClientWithConfig(ctx, PolicyClientDialConfig{Addr: addr, Timeout: timeout})
}

func DialPolicyClientWithConfig(_ context.Context, config PolicyClientDialConfig) (PolicyClient, func() error, error) {
	addr := strings.TrimSpace(config.Addr)
	if addr == "" {
		return PolicyClient{}, nil, errors.New("policy service address is required")
	}
	transportCredentials := grpc.WithTransportCredentials(insecure.NewCredentials())
	if config.TLS.Enabled() {
		creds, err := policyClientTLSCredentials(config.TLS)
		if err != nil {
			return PolicyClient{}, nil, err
		}
		transportCredentials = grpc.WithTransportCredentials(creds)
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		transportCredentials,
	)
	if err != nil {
		return PolicyClient{}, nil, err
	}
	return NewPolicyClient(policyv1.NewPolicyServiceClient(conn), config.Timeout), conn.Close, nil
}

func policyClientTLSCredentials(config PolicyClientTLSConfig) (credentials.TransportCredentials, error) {
	return grpcClientTLSCredentials(
		config,
		"NEXUSIM_POLICY_SERVICE_TLS_CA_FILE",
		"NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE",
		"NEXUSIM_POLICY_SERVICE_TLS_CLIENT_KEY_FILE",
	)
}

func (c PolicyClient) CheckSendPermission(
	ctx context.Context,
	command types.SendMessageCommand,
	conversation types.ConversationSendContext,
) (types.PermissionDecision, error) {
	return c.checkMessageAction(ctx, command.AuthContext, command.ConversationID, "", policyv1.MessageAction_MESSAGE_ACTION_SEND, conversation.DirectPeerUserID, "", conversation.PermissionVersion, messageTextFromPayloadJSON(command.PayloadJSON))
}

func (c PolicyClient) CheckEditPermission(ctx context.Context, command types.EditMessageCommand, conversation types.ConversationSendContext, message types.MessagePolicyContext) (types.PermissionDecision, error) {
	return c.checkMessageAction(ctx, command.AuthContext, command.ConversationID, types.MessageID(command.MessageID), policyv1.MessageAction_MESSAGE_ACTION_EDIT, "", message.SenderUserID, conversation.PermissionVersion, messageTextFromPayloadJSON(command.PayloadJSON))
}

func (c PolicyClient) CheckRevokePermission(ctx context.Context, command types.RevokeMessageCommand, conversation types.ConversationSendContext, message types.MessagePolicyContext) (types.PermissionDecision, error) {
	return c.checkMessageAction(ctx, command.AuthContext, command.ConversationID, command.MessageID, policyv1.MessageAction_MESSAGE_ACTION_REVOKE, "", message.SenderUserID, conversation.PermissionVersion, "")
}

func (c PolicyClient) CheckDeletePermission(ctx context.Context, command types.DeleteMessageCommand, conversation types.ConversationSendContext, message types.MessagePolicyContext) (types.PermissionDecision, error) {
	return c.checkMessageAction(ctx, command.AuthContext, command.ConversationID, command.MessageID, policyv1.MessageAction_MESSAGE_ACTION_DELETE, "", message.SenderUserID, conversation.PermissionVersion, "")
}

func (c PolicyClient) checkMessageAction(
	ctx context.Context,
	auth types.AuthContext,
	conversationID types.ConversationID,
	messageID types.MessageID,
	action policyv1.MessageAction,
	directPeerUserID types.UserID,
	messageSenderUserID types.UserID,
	conversationPermissionVersion int64,
	messageText string,
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
		ConversationId:                string(conversationID),
		Action:                        action,
		MessageId:                     string(messageID),
		DirectPeerUserId:              string(directPeerUserID),
		MessageSenderUserId:           string(messageSenderUserID),
		ConversationPermissionVersion: conversationPermissionVersion,
		MessageText:                   messageText,
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
		OwnershipOverride: response.GetOwnershipOverride(),
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
	if response.GetOwnershipOverride() {
		if !response.GetAllowed() {
			return types.NewDependencyUnavailable("policy service returned denied ownership override")
		}
		switch action {
		case policyv1.MessageAction_MESSAGE_ACTION_EDIT,
			policyv1.MessageAction_MESSAGE_ACTION_REVOKE,
			policyv1.MessageAction_MESSAGE_ACTION_DELETE:
		default:
			return types.NewDependencyUnavailable("policy service returned invalid ownership override")
		}
	}
	return nil
}

func messageTextFromPayloadJSON(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var values map[string]any
	if err := json.Unmarshal(payload, &values); err != nil {
		return ""
	}
	text, ok := values["text"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
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
