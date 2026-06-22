package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	gatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/gateway/v1"
	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	nhooyr "nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	opClientHello    = "client.hello"
	opDeliveryNotify = "delivery.notify"
	opServerHello    = "server.hello"
)

func main() {
	cfg := parseConfig()
	ctx := context.Background()
	if err := run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config) error {
	started := time.Now().UTC()
	result := summary{
		Commit:              gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:          gitOutput("rev-parse", "HEAD"),
		GitDirty:            gitOutput("status", "--short") != "",
		ResultDir:           cfg.resultDir,
		TenantID:            cfg.tenantID,
		GroupConversationID: cfg.conversationID,
		SenderUserID:        cfg.senderUserID,
		ReceiverUserID:      cfg.receiverUserID,
		ReceiverDevice:      cfg.receiverDeviceID,
		BFFBaseURL:          strings.TrimRight(cfg.bffBaseURL, "/"),
		PushURL:             cfg.pushURL,
		StartedAt:           started,
	}
	runErr := runClientWebSmoke(ctx, cfg, &result)
	return finish(cfg, &result, runErr)
}

func runClientWebSmoke(ctx context.Context, cfg config, result *summary) error {
	if strings.TrimSpace(cfg.gatewayAuthHMACSecret) == "" {
		return errors.New("--gateway-auth-hmac-secret is required")
	}
	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()
	if cfg.cleanup {
		if err := cleanupTenant(ctx, pool, cfg.tenantID); err != nil {
			return err
		}
	}
	if err := setupIdentityUsers(ctx, cfg, result); err != nil {
		return err
	}

	sender, err := bffLogin(ctx, cfg, cfg.senderUserID, cfg.senderPassword, cfg.senderDeviceID)
	if err != nil {
		return fmt.Errorf("bff sender login: %w", err)
	}
	result.SenderLogin = loginSummaryFromSession(sender)
	receiver, err := bffLogin(ctx, cfg, cfg.receiverUserID, cfg.receiverPassword, cfg.receiverDeviceID)
	if err != nil {
		return fmt.Errorf("bff receiver login: %w", err)
	}
	result.ReceiverLogin = loginSummaryFromSession(receiver)
	contact, err := establishContact(ctx, cfg, sender, receiver)
	if err != nil {
		return fmt.Errorf("establish contact: %w", err)
	}
	result.Contact = contact

	conn, hello, err := connectPush(ctx, cfg, receiver)
	if err != nil {
		return fmt.Errorf("connect push websocket: %w", err)
	}
	defer conn.CloseNow()
	result.ServerHello = hello

	directID, err := bffOpenDirectConversation(ctx, cfg, sender, cfg.receiverUserID)
	if err != nil {
		return fmt.Errorf("open direct conversation: %w", err)
	}
	if err := waitMembership(ctx, pool, cfg, directID); err != nil {
		return err
	}
	direct, err := runConversationScenario(ctx, cfg, pool, conn, sender, receiver, directID, "DIRECT", "NexusIM direct client smoke message")
	if err != nil {
		return fmt.Errorf("direct chat scenario: %w", err)
	}
	result.DirectChat = direct

	groupID, err := bffCreateGroupConversation(ctx, cfg, sender)
	if err != nil {
		return fmt.Errorf("create group conversation: %w", err)
	}
	join, err := createReceiverJoin(ctx, cfg, groupID)
	if err != nil {
		return fmt.Errorf("create receiver group join: %w", err)
	}
	if err := waitMembership(ctx, pool, cfg, groupID); err != nil {
		return err
	}
	group, err := runConversationScenario(ctx, cfg, pool, conn, sender, receiver, groupID, "GROUP", "NexusIM group client smoke message")
	if err != nil {
		return fmt.Errorf("group chat scenario: %w", err)
	}
	group.MemberChangeID = join.GetChangeId()
	group.MemberBoundarySeq = join.GetBoundarySeq()
	result.GroupChat = group
	memberActions, err := runGroupMemberActions(ctx, cfg, sender, receiver, groupID)
	if err != nil {
		return fmt.Errorf("group member actions: %w", err)
	}
	result.GroupMemberActions = memberActions
	result.Success = true
	return nil
}

func setupIdentityUsers(ctx context.Context, cfg config, result *summary) error {
	dialOption, err := grpctls.DialOption(cfg.identityTLS, "identity-tls")
	if err != nil {
		return fmt.Errorf("configure identity TLS: %w", err)
	}
	conn, err := grpc.NewClient(cfg.identityTarget, dialOption)
	if err != nil {
		return fmt.Errorf("connect identity-service: %w", err)
	}
	defer conn.Close()
	client := identityv1.NewIdentityServiceClient(conn)
	if err := registerUser(ctx, cfg, client, cfg.senderUserID, cfg.senderPassword); err != nil {
		return fmt.Errorf("register sender: %w", err)
	}
	result.Setup.SenderRegistered = true
	if err := registerUser(ctx, cfg, client, cfg.receiverUserID, cfg.receiverPassword); err != nil {
		return fmt.Errorf("register receiver: %w", err)
	}
	result.Setup.ReceiverRegistered = true
	return nil
}

func registerUser(ctx context.Context, cfg config, client identityv1.IdentityServiceClient, userID string, password string) error {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	_, err := client.RegisterUser(requestCtx, &identityv1.RegisterUserRequest{
		TenantId:  cfg.tenantID,
		UserId:    userID,
		Password:  password,
		TraceId:   "client-web-smoke-register",
		RequestId: "client-web-smoke-register-" + userID,
	})
	if status.Code(err) == codes.AlreadyExists {
		return nil
	}
	return err
}

func createReceiverJoin(ctx context.Context, cfg config, conversationID string) (*conversationv1.CreateMemberChangeResponse, error) {
	dialOption, err := grpctls.DialOption(cfg.gatewayTLS, "gateway-tls")
	if err != nil {
		return nil, fmt.Errorf("configure api-gateway TLS: %w", err)
	}
	conn, err := grpc.NewClient(cfg.gatewayTarget, dialOption)
	if err != nil {
		return nil, fmt.Errorf("connect api-gateway: %w", err)
	}
	defer conn.Close()
	token, err := signSetupToken(cfg, cfg.senderUserID, cfg.senderDeviceID, "client-web-smoke-sender-session")
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	requestCtx = metadata.NewOutgoingContext(requestCtx, metadata.Pairs(
		"authorization", "Bearer "+token,
		"x-nexusim-request-id", "client-web-smoke-join",
		"x-nexusim-trace-id", "client-web-smoke-join",
	))
	return gatewayv1.NewGatewayServiceClient(conn).CreateMemberChange(requestCtx, &conversationv1.CreateMemberChangeRequest{
		ConversationId:        conversationID,
		TargetUserId:          cfg.receiverUserID,
		ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN,
		TargetRole:            conversationv1.MemberRole_MEMBER_ROLE_MEMBER,
		ExpectedMemberVersion: 1,
		IdempotencyKey:        "client-web-smoke-join-" + conversationID + "-" + cfg.receiverUserID,
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                "client web smoke receiver join",
	})
}

func signSetupToken(cfg config, userID string, deviceID string, sessionID string) (string, error) {
	return gatewayauth.SignGatewayToken(cfg.gatewayAuthHMACSecret, map[string]string{
		"tenant_id":  cfg.tenantID,
		"user_id":    userID,
		"device_id":  deviceID,
		"session_id": sessionID,
		"trace_id":   "client-web-smoke",
		"aud":        cfg.gatewayAuthAudience,
	}, time.Now().Add(cfg.gatewayAuthTokenTTL))
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	queries := []string{
		`DELETE FROM identity_challenge_delivery_repair_audit WHERE tenant_id = $1`,
		`DELETE FROM identity_challenge_delivery_outbox WHERE tenant_id = $1`,
		`DELETE FROM identity_challenge_request_limits WHERE tenant_id = $1`,
		`DELETE FROM identity_mfa_recovery_codes WHERE tenant_id = $1`,
		`DELETE FROM identity_mfa_factors WHERE tenant_id = $1`,
		`DELETE FROM identity_challenges WHERE tenant_id = $1`,
		`DELETE FROM identity_outbox WHERE tenant_id = $1`,
		`DELETE FROM identity_refresh_tokens WHERE tenant_id = $1`,
		`DELETE FROM identity_sessions WHERE tenant_id = $1`,
		`DELETE FROM identity_devices WHERE tenant_id = $1`,
		`DELETE FROM identity_users WHERE tenant_id = $1`,
		`DELETE FROM user_conversation_summaries WHERE tenant_id = $1`,
		`DELETE FROM receipt_outbox WHERE tenant_id = $1`,
		`DELETE FROM message_receipt_states WHERE tenant_id = $1`,
		`DELETE FROM user_read_cursors WHERE tenant_id = $1`,
		`DELETE FROM user_received_cursors WHERE tenant_id = $1`,
		`DELETE FROM device_received_cursors WHERE tenant_id = $1`,
		`DELETE FROM receipt_inbox_projection WHERE tenant_id = $1`,
		`DELETE FROM delivery_outbox WHERE tenant_id = $1`,
		`DELETE FROM device_delivery_cursors WHERE tenant_id = $1`,
		`DELETE FROM user_inbox WHERE tenant_id = $1`,
		`DELETE FROM delivery_membership_projection WHERE tenant_id = $1`,
		`DELETE FROM message_outbox WHERE tenant_id = $1`,
		`DELETE FROM conversation_timeline_events WHERE tenant_id = $1`,
		`DELETE FROM message_log WHERE tenant_id = $1`,
		`DELETE FROM conversation_seq WHERE tenant_id = $1`,
		`DELETE FROM member_change_saga WHERE tenant_id = $1`,
		`DELETE FROM conversation_members WHERE tenant_id = $1`,
		`DELETE FROM conversations WHERE tenant_id = $1`,
	}
	for _, query := range queries {
		if _, err := pool.Exec(ctx, query, tenantID); err != nil {
			return fmt.Errorf("cleanup query %q: %w", query, err)
		}
	}
	return nil
}

func bffLogin(ctx context.Context, cfg config, userID string, password string, deviceID string) (authSession, error) {
	var response struct {
		TenantID               string          `json:"tenant_id"`
		UserID                 string          `json:"user_id"`
		DeviceID               string          `json:"device_id"`
		SessionID              string          `json:"session_id"`
		TokenType              string          `json:"token_type"`
		GatewayToken           string          `json:"gateway_token"`
		PushGatewayToken       string          `json:"push_gateway_token"`
		RefreshToken           string          `json:"refresh_token"`
		GatewayExpiresAtUnixMS json.RawMessage `json:"gateway_expires_at_unix_ms"`
		PushExpiresAtUnixMS    json.RawMessage `json:"push_gateway_expires_at_unix_ms"`
	}
	err := bffJSON(ctx, cfg, http.MethodPost, "/api/auth/login", "", map[string]any{
		"tenant_id":  cfg.tenantID,
		"user_id":    userID,
		"password":   password,
		"device_id":  deviceID,
		"audience":   "api-gateway",
		"trace_id":   "client-web-smoke-login",
		"request_id": "client-web-smoke-login-" + userID,
	}, &response)
	if err != nil {
		return authSession{}, err
	}
	expires, _ := int64JSON(response.GatewayExpiresAtUnixMS)
	pushExpires, _ := int64JSON(response.PushExpiresAtUnixMS)
	session := authSession{
		TenantID:       response.TenantID,
		UserID:         response.UserID,
		DeviceID:       response.DeviceID,
		SessionID:      response.SessionID,
		TokenType:      response.TokenType,
		GatewayToken:   response.GatewayToken,
		PushToken:      response.PushGatewayToken,
		RefreshToken:   response.RefreshToken,
		GatewayExpires: expires,
		PushExpires:    pushExpires,
	}
	if session.GatewayToken == "" || session.RefreshToken == "" || session.SessionID == "" {
		return authSession{}, errors.New("BFF login returned incomplete session")
	}
	if session.PushToken == "" {
		return authSession{}, errors.New("BFF login returned no push gateway token")
	}
	return session, nil
}

func establishContact(ctx context.Context, cfg config, sender authSession, receiver authSession) (contactSummary, error) {
	requestID, err := bffSendContactRequest(ctx, cfg, sender, cfg.receiverUserID)
	if err != nil {
		return contactSummary{}, err
	}
	if err := bffAcceptIncomingContactRequest(ctx, cfg, receiver, requestID); err != nil {
		return contactSummary{}, err
	}
	if err := waitContactActive(ctx, cfg, sender, cfg.receiverUserID); err != nil {
		return contactSummary{}, fmt.Errorf("sender contact state: %w", err)
	}
	if err := waitContactActive(ctx, cfg, receiver, cfg.senderUserID); err != nil {
		return contactSummary{}, fmt.Errorf("receiver contact state: %w", err)
	}
	return contactSummary{RequestID: requestID, SenderActive: true, ReceiverActive: true}, nil
}

func bffSendContactRequest(ctx context.Context, cfg config, session authSession, targetUserID string) (string, error) {
	var response struct {
		RequestID string `json:"request_id"`
		Status    string `json:"status"`
	}
	err := bffJSON(ctx, cfg, http.MethodPost, "/api/contact-requests/send", session.GatewayToken, map[string]any{
		"target_user_id":  targetUserID,
		"idempotency_key": "client-web-smoke-contact-" + session.UserID + "-" + targetUserID,
		"message":         "client web smoke contact request",
		"source_type":     "CONTACT_REQUEST_SOURCE_TYPE_DIRECT",
	}, &response)
	if err != nil {
		return "", err
	}
	if response.RequestID == "" {
		return "", fmt.Errorf("BFF contact request returned no request_id: %+v", response)
	}
	if response.Status != "" && response.Status != "CONTACT_REQUEST_STATUS_PENDING" && response.Status != "CONTACT_REQUEST_STATUS_ACCEPTED" {
		return "", fmt.Errorf("BFF contact request returned unexpected status %s", response.Status)
	}
	return response.RequestID, nil
}

func bffAcceptIncomingContactRequest(ctx context.Context, cfg config, session authSession, requestID string) error {
	var response struct {
		RequestID string `json:"request_id"`
		Status    string `json:"status"`
	}
	if err := bffJSON(ctx, cfg, http.MethodPost, "/api/contact-requests/respond", session.GatewayToken, map[string]any{
		"request_id":      requestID,
		"decision":        "CONTACT_DECISION_ACCEPT",
		"idempotency_key": "client-web-smoke-contact-accept-" + requestID,
	}, &response); err != nil {
		return err
	}
	if response.RequestID != requestID || response.Status != "CONTACT_REQUEST_STATUS_ACCEPTED" {
		return fmt.Errorf("BFF contact accept returned invalid response: %+v", response)
	}
	return nil
}

func waitContactActive(ctx context.Context, cfg config, session authSession, otherUserID string) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	var lastStatus string
	for time.Now().Before(deadline) {
		statusValue, err := bffGetContactState(ctx, cfg, session, otherUserID)
		if err != nil {
			return err
		}
		lastStatus = statusValue
		if statusValue == "CONTACT_EDGE_STATUS_ACTIVE" {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return fmt.Errorf("contact state for %s did not become ACTIVE, last=%s", otherUserID, lastStatus)
}

func bffGetContactState(ctx context.Context, cfg config, session authSession, otherUserID string) (string, error) {
	var response struct {
		ContactUserID string `json:"contact_user_id"`
		Status        string `json:"status"`
	}
	path := "/api/contacts/state?other_user_id=" + url.QueryEscape(otherUserID)
	if err := bffJSON(ctx, cfg, http.MethodGet, path, session.GatewayToken, nil, &response); err != nil {
		return "", err
	}
	if response.ContactUserID != otherUserID {
		return "", fmt.Errorf("BFF contact state returned wrong contact_user_id: %+v", response)
	}
	return response.Status, nil
}

func bffOpenDirectConversation(ctx context.Context, cfg config, session authSession, peerUserID string) (string, error) {
	var response struct {
		ConversationID   string `json:"conversation_id"`
		ConversationType string `json:"conversation_type"`
		DirectPeerUserID string `json:"direct_peer_user_id"`
	}
	err := bffJSON(ctx, cfg, http.MethodPost, "/api/conversations/direct", session.GatewayToken, map[string]any{
		"peer_user_id":    peerUserID,
		"idempotency_key": "client-web-smoke-direct-" + session.UserID + "-" + peerUserID,
	}, &response)
	if err != nil {
		return "", err
	}
	if response.ConversationID == "" ||
		response.ConversationType != "CONVERSATION_TYPE_DIRECT" ||
		response.DirectPeerUserID != peerUserID {
		return "", fmt.Errorf("BFF direct conversation returned invalid response: %+v", response)
	}
	return response.ConversationID, nil
}

func bffCreateGroupConversation(ctx context.Context, cfg config, session authSession) (string, error) {
	var response struct {
		ConversationID   string `json:"conversation_id"`
		ConversationType string `json:"conversation_type"`
	}
	err := bffJSON(ctx, cfg, http.MethodPost, "/api/conversations/create", session.GatewayToken, map[string]any{
		"conversation_id":   cfg.conversationID,
		"conversation_type": "CONVERSATION_TYPE_GROUP",
		"idempotency_key":   "client-web-smoke-group-" + cfg.conversationID,
	}, &response)
	if err != nil {
		return "", err
	}
	if response.ConversationID != cfg.conversationID || response.ConversationType != "CONVERSATION_TYPE_GROUP" {
		return "", fmt.Errorf("BFF group conversation returned invalid response: %+v", response)
	}
	return response.ConversationID, nil
}

func runGroupMemberActions(
	ctx context.Context,
	cfg config,
	sender authSession,
	receiver authSession,
	conversationID string,
) (groupMemberActionsSummary, error) {
	var result groupMemberActionsSummary
	initial, err := waitGroupMemberRole(ctx, cfg, sender, conversationID, cfg.receiverUserID, "MEMBER_ROLE_MEMBER")
	if err != nil {
		return result, fmt.Errorf("initial member list: %w", err)
	}
	if err := requireGroupMemberRole(initial, cfg.senderUserID, "MEMBER_ROLE_OWNER"); err != nil {
		return result, err
	}
	result.Initial = initial

	roleChange, err := bffUpdateGroupMemberRole(ctx, cfg, sender, conversationID, cfg.receiverUserID, "ADMIN", initial.MemberVersion)
	if err != nil {
		return result, fmt.Errorf("role change receiver to admin: %w", err)
	}
	result.RoleChange = roleChange
	afterRole, err := waitGroupMemberRole(ctx, cfg, sender, conversationID, cfg.receiverUserID, "MEMBER_ROLE_ADMIN")
	if err != nil {
		return result, fmt.Errorf("after role change member list: %w", err)
	}
	result.AfterRoleChange = afterRole

	transfer, err := bffTransferGroupOwner(ctx, cfg, sender, conversationID, cfg.receiverUserID, afterRole.MemberVersion)
	if err != nil {
		return result, fmt.Errorf("transfer owner to receiver: %w", err)
	}
	result.OwnerTransfer = transfer
	afterTransfer, err := waitGroupMemberRole(ctx, cfg, receiver, conversationID, cfg.receiverUserID, "MEMBER_ROLE_OWNER")
	if err != nil {
		return result, fmt.Errorf("after owner transfer member list: %w", err)
	}
	if err := requireGroupMemberRole(afterTransfer, cfg.senderUserID, "MEMBER_ROLE_ADMIN"); err != nil {
		return result, err
	}
	result.AfterTransfer = afterTransfer

	removed, err := bffRemoveGroupMember(ctx, cfg, receiver, conversationID, cfg.senderUserID, afterTransfer.MemberVersion)
	if err != nil {
		return result, fmt.Errorf("remove previous owner: %w", err)
	}
	result.RemoveMember = removed
	final, err := waitGroupMemberAbsent(ctx, cfg, receiver, conversationID, cfg.senderUserID)
	if err != nil {
		return result, fmt.Errorf("final member list: %w", err)
	}
	if err := requireGroupMemberRole(final, cfg.receiverUserID, "MEMBER_ROLE_OWNER"); err != nil {
		return result, err
	}
	result.Final = final
	return result, nil
}

func waitGroupMemberRole(ctx context.Context, cfg config, session authSession, conversationID string, userID string, role string) (groupMemberListSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last groupMemberListSummary
	for {
		list, err := bffListGroupMembers(ctx, cfg, session, conversationID)
		if err != nil {
			return groupMemberListSummary{}, err
		}
		last = list
		if hasGroupMemberRole(list, userID, role) {
			return list, nil
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("member %s did not reach role %s", userID, role)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func waitGroupMemberAbsent(ctx context.Context, cfg config, session authSession, conversationID string, userID string) (groupMemberListSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last groupMemberListSummary
	for {
		list, err := bffListGroupMembers(ctx, cfg, session, conversationID)
		if err != nil {
			return groupMemberListSummary{}, err
		}
		last = list
		if !hasGroupMember(list, userID) {
			return list, nil
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("member %s still appears in active member list", userID)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func bffListGroupMembers(ctx context.Context, cfg config, session authSession, conversationID string) (groupMemberListSummary, error) {
	var response struct {
		ConversationID    string          `json:"conversation_id"`
		MemberVersion     json.RawMessage `json:"member_version"`
		PermissionVersion json.RawMessage `json:"permission_version"`
		Members           []struct {
			UserID            string          `json:"user_id"`
			Role              string          `json:"role"`
			Status            string          `json:"status"`
			JoinSeq           json.RawMessage `json:"join_seq"`
			LeaveSeq          json.RawMessage `json:"leave_seq"`
			MemberVersion     json.RawMessage `json:"member_version"`
			PermissionVersion json.RawMessage `json:"permission_version"`
		} `json:"members"`
	}
	path := fmt.Sprintf("/api/conversations/%s/members?page_size=100", url.PathEscape(conversationID))
	if err := bffJSON(ctx, cfg, http.MethodGet, path, session.GatewayToken, nil, &response); err != nil {
		return groupMemberListSummary{}, err
	}
	if response.ConversationID != conversationID {
		return groupMemberListSummary{}, fmt.Errorf("BFF members returned conversation_id %s, want %s", response.ConversationID, conversationID)
	}
	memberVersion, err := int64JSON(response.MemberVersion)
	if err != nil {
		return groupMemberListSummary{}, fmt.Errorf("parse member version: %w", err)
	}
	permissionVersion, err := int64JSON(response.PermissionVersion)
	if err != nil {
		return groupMemberListSummary{}, fmt.Errorf("parse permission version: %w", err)
	}
	result := groupMemberListSummary{
		ConversationID:    response.ConversationID,
		MemberVersion:     memberVersion,
		PermissionVersion: permissionVersion,
		Members:           []groupMemberSummaryMember{},
	}
	for _, item := range response.Members {
		joinSeq, err := int64JSON(item.JoinSeq)
		if err != nil {
			return groupMemberListSummary{}, fmt.Errorf("parse join seq: %w", err)
		}
		leaveSeq, err := int64JSON(item.LeaveSeq)
		if err != nil {
			return groupMemberListSummary{}, fmt.Errorf("parse leave seq: %w", err)
		}
		itemMemberVersion, err := int64JSON(item.MemberVersion)
		if err != nil {
			return groupMemberListSummary{}, fmt.Errorf("parse item member version: %w", err)
		}
		itemPermissionVersion, err := int64JSON(item.PermissionVersion)
		if err != nil {
			return groupMemberListSummary{}, fmt.Errorf("parse item permission version: %w", err)
		}
		result.Members = append(result.Members, groupMemberSummaryMember{
			UserID:            item.UserID,
			Role:              item.Role,
			Status:            item.Status,
			JoinSeq:           joinSeq,
			LeaveSeq:          leaveSeq,
			MemberVersion:     itemMemberVersion,
			PermissionVersion: itemPermissionVersion,
		})
	}
	result.ItemCount = len(result.Members)
	return result, nil
}

func bffUpdateGroupMemberRole(ctx context.Context, cfg config, session authSession, conversationID string, targetUserID string, targetRole string, expectedMemberVersion int64) (memberActionSummary, error) {
	var response struct {
		ChangeID      string          `json:"change_id"`
		TargetUserID  string          `json:"target_user_id"`
		ChangeType    string          `json:"change_type"`
		Status        string          `json:"status"`
		MemberVersion json.RawMessage `json:"member_version"`
		BoundarySeq   json.RawMessage `json:"boundary_seq"`
	}
	path := fmt.Sprintf("/api/conversations/%s/members/role", url.PathEscape(conversationID))
	err := bffJSON(ctx, cfg, http.MethodPost, path, session.GatewayToken, map[string]any{
		"target_user_id":          targetUserID,
		"target_role":             targetRole,
		"expected_member_version": expectedMemberVersion,
		"idempotency_key":         "client-web-smoke-role-" + conversationID + "-" + targetUserID + "-" + targetRole,
		"reason":                  "client web smoke role change",
	}, &response)
	if err != nil {
		return memberActionSummary{}, err
	}
	if response.TargetUserID != targetUserID || response.ChangeType != "MEMBER_CHANGE_TYPE_ROLE_CHANGED" {
		return memberActionSummary{}, fmt.Errorf("BFF role change returned invalid response: %+v", response)
	}
	return memberActionFromBFF(response.ChangeID, response.TargetUserID, response.ChangeType, response.Status, response.MemberVersion, response.BoundarySeq)
}

func bffRemoveGroupMember(ctx context.Context, cfg config, session authSession, conversationID string, targetUserID string, expectedMemberVersion int64) (memberActionSummary, error) {
	var response struct {
		ChangeID      string          `json:"change_id"`
		TargetUserID  string          `json:"target_user_id"`
		ChangeType    string          `json:"change_type"`
		Status        string          `json:"status"`
		MemberVersion json.RawMessage `json:"member_version"`
		BoundarySeq   json.RawMessage `json:"boundary_seq"`
	}
	path := fmt.Sprintf("/api/conversations/%s/members/remove", url.PathEscape(conversationID))
	err := bffJSON(ctx, cfg, http.MethodPost, path, session.GatewayToken, map[string]any{
		"target_user_id":          targetUserID,
		"expected_member_version": expectedMemberVersion,
		"idempotency_key":         "client-web-smoke-remove-" + conversationID + "-" + targetUserID,
		"reason":                  "client web smoke remove previous owner",
	}, &response)
	if err != nil {
		return memberActionSummary{}, err
	}
	if response.TargetUserID != targetUserID || response.ChangeType != "MEMBER_CHANGE_TYPE_REMOVE" {
		return memberActionSummary{}, fmt.Errorf("BFF remove returned invalid response: %+v", response)
	}
	return memberActionFromBFF(response.ChangeID, response.TargetUserID, response.ChangeType, response.Status, response.MemberVersion, response.BoundarySeq)
}

func bffTransferGroupOwner(ctx context.Context, cfg config, session authSession, conversationID string, newOwnerUserID string, expectedMemberVersion int64) (ownerTransferSummary, error) {
	var response struct {
		ChangeID            string          `json:"change_id"`
		PreviousOwnerUserID string          `json:"previous_owner_user_id"`
		NewOwnerUserID      string          `json:"new_owner_user_id"`
		Status              string          `json:"status"`
		MemberVersion       json.RawMessage `json:"member_version"`
		BoundarySeq         json.RawMessage `json:"boundary_seq"`
	}
	path := fmt.Sprintf("/api/conversations/%s/owner/transfer", url.PathEscape(conversationID))
	err := bffJSON(ctx, cfg, http.MethodPost, path, session.GatewayToken, map[string]any{
		"new_owner_user_id":       newOwnerUserID,
		"expected_member_version": expectedMemberVersion,
		"idempotency_key":         "client-web-smoke-owner-transfer-" + conversationID + "-" + newOwnerUserID,
		"reason":                  "client web smoke owner transfer",
	}, &response)
	if err != nil {
		return ownerTransferSummary{}, err
	}
	if response.PreviousOwnerUserID != session.UserID || response.NewOwnerUserID != newOwnerUserID {
		return ownerTransferSummary{}, fmt.Errorf("BFF owner transfer returned invalid response: %+v", response)
	}
	memberVersion, err := int64JSON(response.MemberVersion)
	if err != nil {
		return ownerTransferSummary{}, fmt.Errorf("parse transfer member version: %w", err)
	}
	boundarySeq, err := int64JSON(response.BoundarySeq)
	if err != nil {
		return ownerTransferSummary{}, fmt.Errorf("parse transfer boundary seq: %w", err)
	}
	if response.ChangeID == "" || memberVersion <= 0 || boundarySeq <= 0 {
		return ownerTransferSummary{}, fmt.Errorf("BFF owner transfer returned incomplete response: %+v", response)
	}
	return ownerTransferSummary{
		ChangeID:            response.ChangeID,
		PreviousOwnerUserID: response.PreviousOwnerUserID,
		NewOwnerUserID:      response.NewOwnerUserID,
		Status:              response.Status,
		MemberVersion:       memberVersion,
		BoundarySeq:         boundarySeq,
	}, nil
}

func memberActionFromBFF(changeID string, targetUserID string, changeType string, statusValue string, rawMemberVersion json.RawMessage, rawBoundarySeq json.RawMessage) (memberActionSummary, error) {
	memberVersion, err := int64JSON(rawMemberVersion)
	if err != nil {
		return memberActionSummary{}, fmt.Errorf("parse action member version: %w", err)
	}
	boundarySeq, err := int64JSON(rawBoundarySeq)
	if err != nil {
		return memberActionSummary{}, fmt.Errorf("parse action boundary seq: %w", err)
	}
	if changeID == "" || memberVersion <= 0 || boundarySeq <= 0 {
		return memberActionSummary{}, fmt.Errorf("BFF member action returned incomplete response: change_id=%s member_version=%d boundary_seq=%d", changeID, memberVersion, boundarySeq)
	}
	return memberActionSummary{
		ChangeID:      changeID,
		TargetUserID:  targetUserID,
		ChangeType:    changeType,
		Status:        statusValue,
		MemberVersion: memberVersion,
		BoundarySeq:   boundarySeq,
	}, nil
}

func requireGroupMemberRole(list groupMemberListSummary, userID string, role string) error {
	if hasGroupMemberRole(list, userID, role) {
		return nil
	}
	return fmt.Errorf("member %s does not have role %s in %+v", userID, role, list.Members)
}

func hasGroupMemberRole(list groupMemberListSummary, userID string, role string) bool {
	for _, member := range list.Members {
		if member.UserID == userID && member.Role == role && member.Status == "MEMBER_STATUS_ACTIVE" {
			return true
		}
	}
	return false
}

func hasGroupMember(list groupMemberListSummary, userID string) bool {
	for _, member := range list.Members {
		if member.UserID == userID {
			return true
		}
	}
	return false
}

func runConversationScenario(
	ctx context.Context,
	cfg config,
	pool *pgxpool.Pool,
	conn *nhooyr.Conn,
	sender authSession,
	receiver authSession,
	conversationID string,
	conversationType string,
	text string,
) (scenarioSummary, error) {
	result := scenarioSummary{ConversationID: conversationID, ConversationType: conversationType}
	sent, err := bffSendMessage(ctx, cfg, sender, conversationID, text)
	if err != nil {
		return result, fmt.Errorf("bff send message: %w", err)
	}
	result.SendMessage = sent
	notify, err := waitNotify(ctx, cfg, conn, conversationID, sent.ConversationSeq, sent.MessageID)
	if err != nil {
		return result, fmt.Errorf("wait push notify: %w", err)
	}
	result.Notify = notify
	pull, err := waitPullInbox(ctx, cfg, receiver, conversationID, sent.ConversationSeq)
	if err != nil {
		return result, fmt.Errorf("bff pull inbox: %w", err)
	}
	result.PullInbox = pull
	conversations, err := waitConversationList(ctx, cfg, receiver, conversationID, sent.ConversationSeq)
	if err != nil {
		return result, fmt.Errorf("bff list conversations: %w", err)
	}
	result.ListConversations = conversations
	ack, err := bffAckDelivery(ctx, cfg, receiver, conversationID, pull.MaxSeq)
	if err != nil {
		return result, fmt.Errorf("bff ack delivery: %w", err)
	}
	result.AckDelivery = ack
	if err := waitDeliveryCursor(ctx, pool, cfg, conversationID, pull.MaxSeq); err != nil {
		return result, err
	}
	postgres, err := postgresStats(ctx, pool, cfg, conversationID)
	if err != nil {
		return result, err
	}
	result.Postgres = postgres
	return result, nil
}

func bffSendMessage(ctx context.Context, cfg config, session authSession, conversationID string, text string) (sendSummary, error) {
	payload, err := structpb.NewStruct(map[string]any{"text": text})
	if err != nil {
		return sendSummary{}, err
	}
	var response struct {
		MessageID       string          `json:"message_id"`
		ConversationID  string          `json:"conversation_id"`
		ConversationSeq json.RawMessage `json:"conversation_seq"`
	}
	err = bffJSON(ctx, cfg, http.MethodPost, "/api/messages/send", session.GatewayToken, map[string]any{
		"conversation_id": conversationID,
		"client_msg_id":   fmt.Sprintf("client-web-smoke-%d", time.Now().UnixNano()),
		"message_type":    "TEXT",
		"payload":         payload.AsMap(),
		"attachment_ids":  []string{},
	}, &response)
	if err != nil {
		return sendSummary{}, err
	}
	seq, err := int64JSON(response.ConversationSeq)
	if err != nil {
		return sendSummary{}, fmt.Errorf("parse conversation_seq: %w", err)
	}
	if response.MessageID == "" || response.ConversationID != conversationID || seq <= 0 {
		return sendSummary{}, fmt.Errorf("BFF send returned invalid response: %+v seq=%d", response, seq)
	}
	return sendSummary{MessageID: response.MessageID, ConversationSeq: seq}, nil
}

func waitPullInbox(ctx context.Context, cfg config, session authSession, conversationID string, minSeq int64) (pullSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last pullSummary
	for {
		pull, err := bffPullInbox(ctx, cfg, session, conversationID, 0)
		if err != nil {
			return pullSummary{}, err
		}
		last = pull
		if pull.MaxSeq >= minSeq {
			return pull, nil
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("pull inbox max seq %d before expected seq %d", last.MaxSeq, minSeq)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func bffPullInbox(ctx context.Context, cfg config, session authSession, conversationID string, afterSeq int64) (pullSummary, error) {
	var response struct {
		Items []struct {
			ConversationID  string          `json:"conversation_id"`
			ConversationSeq json.RawMessage `json:"conversation_seq"`
			EventID         string          `json:"event_id"`
			EventType       string          `json:"event_type"`
			MessageID       string          `json:"message_id"`
			SenderID        string          `json:"sender_id"`
		} `json:"items"`
		NextSeq json.RawMessage `json:"next_seq"`
	}
	path := fmt.Sprintf("/api/conversations/%s/messages?after_seq=%d&limit=100", url.PathEscape(conversationID), afterSeq)
	if err := bffJSON(ctx, cfg, http.MethodGet, path, session.GatewayToken, nil, &response); err != nil {
		return pullSummary{}, err
	}
	result := pullSummary{Items: []inboxItem{}}
	for _, item := range response.Items {
		if item.ConversationID != conversationID {
			return pullSummary{}, fmt.Errorf("pull inbox returned conversation_id %s, want %s", item.ConversationID, conversationID)
		}
		seq, err := int64JSON(item.ConversationSeq)
		if err != nil {
			return pullSummary{}, fmt.Errorf("parse inbox seq: %w", err)
		}
		if seq > result.MaxSeq {
			result.MaxSeq = seq
		}
		result.Items = append(result.Items, inboxItem{
			ConversationSeq: seq,
			EventID:         item.EventID,
			EventType:       item.EventType,
			MessageID:       item.MessageID,
			SenderID:        item.SenderID,
		})
	}
	result.ItemCount = len(result.Items)
	return result, nil
}

func waitConversationList(ctx context.Context, cfg config, session authSession, conversationID string, minSeq int64) (conversationSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last conversationSummary
	for {
		list, err := bffListConversations(ctx, cfg, session)
		if err != nil {
			return conversationSummary{}, err
		}
		last = list
		for _, item := range list.Items {
			if item.ConversationID == conversationID && item.LastVisibleSeq >= minSeq {
				return list, nil
			}
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("conversation list did not reach seq %d", minSeq)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func bffListConversations(ctx context.Context, cfg config, session authSession) (conversationSummary, error) {
	var response struct {
		Items []struct {
			ConversationID string          `json:"conversation_id"`
			LastVisibleSeq json.RawMessage `json:"last_visible_seq"`
			LastMessageID  string          `json:"last_message_id"`
			UnreadCount    json.RawMessage `json:"unread_count"`
		} `json:"items"`
	}
	if err := bffJSON(ctx, cfg, http.MethodGet, "/api/conversations?limit=20", session.GatewayToken, nil, &response); err != nil {
		return conversationSummary{}, err
	}
	result := conversationSummary{Items: []conversationSummaryItem{}}
	for _, item := range response.Items {
		seq, err := int64JSON(item.LastVisibleSeq)
		if err != nil {
			return conversationSummary{}, fmt.Errorf("parse last visible seq: %w", err)
		}
		unread, err := int64JSON(item.UnreadCount)
		if err != nil {
			return conversationSummary{}, fmt.Errorf("parse unread count: %w", err)
		}
		result.Items = append(result.Items, conversationSummaryItem{
			ConversationID: item.ConversationID,
			LastVisibleSeq: seq,
			LastMessageID:  item.LastMessageID,
			UnreadCount:    unread,
		})
	}
	result.ItemCount = len(result.Items)
	return result, nil
}

func bffAckDelivery(ctx context.Context, cfg config, session authSession, conversationID string, seq int64) (ackSummary, error) {
	var response struct {
		ConversationID  string          `json:"conversation_id"`
		LastReceivedSeq json.RawMessage `json:"last_received_seq"`
	}
	err := bffJSON(ctx, cfg, http.MethodPost, "/api/delivery/ack", session.GatewayToken, map[string]any{
		"conversation_id": conversationID,
		"received_seq":    seq,
	}, &response)
	if err != nil {
		return ackSummary{}, err
	}
	acked, err := int64JSON(response.LastReceivedSeq)
	if err != nil {
		return ackSummary{}, fmt.Errorf("parse ack seq: %w", err)
	}
	if acked < seq {
		return ackSummary{}, fmt.Errorf("ack seq %d before expected %d", acked, seq)
	}
	if response.ConversationID != conversationID {
		return ackSummary{}, fmt.Errorf("ack returned conversation_id %s, want %s", response.ConversationID, conversationID)
	}
	return ackSummary{ConversationID: response.ConversationID, LastReceivedSeq: acked}, nil
}

func bffJSON(ctx context.Context, cfg config, method string, path string, bearerToken string, body any, output any) error {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(requestCtx, method, strings.TrimRight(cfg.bffBaseURL, "/")+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-NexusIM-Request-ID", "client-web-smoke")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(bearerToken) != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("BFF %s %s returned %d: %s", method, path, response.StatusCode, strings.TrimSpace(string(data)))
	}
	if output == nil {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode BFF response: %w body=%s", err, string(data))
	}
	return nil
}

func connectPush(ctx context.Context, cfg config, session authSession) (*nhooyr.Conn, serverFrame, error) {
	u, err := url.Parse(cfg.pushURL)
	if err != nil {
		return nil, serverFrame{}, err
	}
	query := u.Query()
	query.Set("token", session.PushToken)
	query.Set("tenant_id", session.TenantID)
	query.Set("user_id", session.UserID)
	query.Set("device_id", session.DeviceID)
	u.RawQuery = query.Encode()
	options := &nhooyr.DialOptions{}
	if u.Scheme == "wss" {
		options.HTTPClient = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	conn, _, err := nhooyr.Dial(requestCtx, u.String(), options)
	if err != nil {
		return nil, serverFrame{}, err
	}
	if err := wsjson.Write(requestCtx, conn, clientFrame{
		Op:        opClientHello,
		RequestID: "client-web-smoke-hello",
		DeviceID:  session.DeviceID,
	}); err != nil {
		conn.CloseNow()
		return nil, serverFrame{}, err
	}
	var hello serverFrame
	if err := wsjson.Read(requestCtx, conn, &hello); err != nil {
		conn.CloseNow()
		return nil, serverFrame{}, err
	}
	if hello.Op != opServerHello {
		conn.CloseNow()
		return nil, serverFrame{}, fmt.Errorf("unexpected hello frame: %+v", hello)
	}
	return conn, hello, nil
}

func waitNotify(ctx context.Context, cfg config, conn *nhooyr.Conn, conversationID string, seq int64, messageID string) (serverFrame, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last serverFrame
	for time.Now().Before(deadline) {
		requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
		var frame serverFrame
		err := wsjson.Read(requestCtx, conn, &frame)
		cancel()
		if err != nil {
			if time.Now().After(deadline) {
				return last, err
			}
			continue
		}
		last = frame
		if frame.Op == opDeliveryNotify &&
			frame.ConversationID == conversationID &&
			frame.ConversationSeq == seq &&
			frame.MessageID == messageID {
			return frame, nil
		}
	}
	return last, fmt.Errorf("did not receive delivery.notify conversation_id=%s seq=%d message_id=%s", conversationID, seq, messageID)
}

func waitMembership(ctx context.Context, pool *pgxpool.Pool, cfg config, conversationID string) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var count int
		err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_membership_projection
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
  AND status = 'ACTIVE'
`, cfg.tenantID, conversationID, cfg.receiverUserID).Scan(&count)
		if err == nil && count > 0 {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return fmt.Errorf("delivery membership projection not ready for %s in %s", cfg.receiverUserID, conversationID)
}

func waitDeliveryCursor(ctx context.Context, pool *pgxpool.Pool, cfg config, conversationID string, seq int64) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var got int64
		err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_delivery_cursors
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
  AND device_id = $4
`, cfg.tenantID, conversationID, cfg.receiverUserID, cfg.receiverDeviceID).Scan(&got)
		if err == nil && got >= seq {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return fmt.Errorf("delivery cursor for %s did not reach seq %d", conversationID, seq)
}

func postgresStats(ctx context.Context, pool *pgxpool.Pool, cfg config, conversationID string) (postgresSummary, error) {
	var result postgresSummary
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_inbox WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3`, cfg.tenantID, conversationID, cfg.receiverUserID).Scan(&result.UserInboxCount); err != nil {
		return result, err
	}
	err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_delivery_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3 AND device_id = $4
`, cfg.tenantID, conversationID, cfg.receiverUserID, cfg.receiverDeviceID).Scan(&result.DeviceDeliveryCursorSeq)
	return result, err
}

func loginSummaryFromSession(session authSession) loginSummary {
	return loginSummary{
		SessionID:       session.SessionID,
		TokenType:       session.TokenType,
		GatewayTokenSet: session.GatewayToken != "",
		PushTokenSet:    session.PushToken != "",
		RefreshTokenSet: session.RefreshToken != "",
	}
}

func int64JSON(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if strings.TrimSpace(asString) == "" {
			return 0, nil
		}
		var parsed int64
		if _, err := fmt.Sscan(asString, &parsed); err != nil {
			return 0, err
		}
		return parsed, nil
	}
	var asNumber int64
	if err := json.Unmarshal(raw, &asNumber); err != nil {
		return 0, err
	}
	return asNumber, nil
}

func finish(cfg config, result *summary, runErr error) error {
	result.FinishedAt = time.Now().UTC()
	if runErr != nil {
		result.Error = runErr.Error()
	}
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.resultDir, "client-web-summary.json"), payload, 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	if runErr != nil {
		return runErr
	}
	return nil
}

func gitOutput(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
