package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

func main() {
	cfg := parseConfig()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	if cfg.pgDSN == "" {
		return errors.New("pg-dsn is required")
	}
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return fmt.Errorf("open pg pool: %w", err)
	}
	defer pool.Close()
	if cfg.cleanup {
		if err := cleanupTenant(ctx, pool, cfg); err != nil {
			return err
		}
	}
	if err := seedConversation(ctx, pool, cfg); err != nil {
		return err
	}

	conversationDialOption, err := grpctls.DialOption(cfg.conversationTLS, "conversation-tls")
	if err != nil {
		return fmt.Errorf("configure conversation-service TLS: %w", err)
	}
	conversationConn, err := grpc.NewClient(cfg.conversationTarget, conversationDialOption)
	if err != nil {
		return fmt.Errorf("dial conversation-service: %w", err)
	}
	defer conversationConn.Close()
	messageDialOption, err := grpctls.DialOption(cfg.messageTLS, "message-tls")
	if err != nil {
		return fmt.Errorf("configure message-service TLS: %w", err)
	}
	messageConn, err := grpc.NewClient(cfg.messageTarget, messageDialOption)
	if err != nil {
		return fmt.Errorf("dial message-service: %w", err)
	}
	defer messageConn.Close()
	deliveryDialOption, err := grpctls.DialOption(cfg.deliveryTLS, "delivery-tls")
	if err != nil {
		return fmt.Errorf("configure delivery-service TLS: %w", err)
	}
	deliveryConn, err := grpc.NewClient(cfg.deliveryTarget, deliveryDialOption)
	if err != nil {
		return fmt.Errorf("dial delivery-service: %w", err)
	}
	defer deliveryConn.Close()
	receiptDialOption, err := grpctls.DialOption(cfg.receiptTLS, "receipt-tls")
	if err != nil {
		return fmt.Errorf("configure receipt-service TLS: %w", err)
	}
	receiptConn, err := grpc.NewClient(cfg.receiptTarget, receiptDialOption)
	if err != nil {
		return fmt.Errorf("dial receipt-service: %w", err)
	}
	defer receiptConn.Close()

	conversationClient := conversationv1.NewConversationServiceClient(conversationConn)
	messageClient := messagev1.NewMessageServiceClient(messageConn)
	deliveryClient := deliveryv1.NewDeliveryServiceClient(deliveryConn)
	receiptClient := receiptv1.NewReceiptServiceClient(receiptConn)

	result := summary{
		Commit:                 shortCommit(),
		CommitFull:             fullCommit(),
		GitDirty:               gitDirty(),
		GitStatusShort:         gitStatusShort(),
		ConversationTarget:     cfg.conversationTarget,
		MessageTarget:          cfg.messageTarget,
		DeliveryTarget:         cfg.deliveryTarget,
		ReceiptTarget:          cfg.receiptTarget,
		ConversationTLSEnabled: cfg.conversationTLS.Enabled(),
		MessageTLSEnabled:      cfg.messageTLS.Enabled(),
		DeliveryTLSEnabled:     cfg.deliveryTLS.Enabled(),
		ReceiptTLSEnabled:      cfg.receiptTLS.Enabled(),
		VerifiedAuthMetadata:   cfg.verifiedAuthMetadata,
		ResultDir:              cfg.resultDir,
		TenantID:               cfg.tenantID,
		ConversationID:         cfg.conversationID,
		OwnerUserID:            cfg.ownerUserID,
		ReceiverUserID:         cfg.receiverUserID,
		ReceiverDeviceID:       cfg.receiverDeviceID,
		DeliveryConsumerGroup:  cfg.deliveryGroup,
		ReceiptConsumerGroup:   cfg.receiptGroup,
		ReceiptEventsTopic:     cfg.receiptEventsTopic,
		ReceiptEventsGroup:     cfg.receiptEventsGroup,
		StartedAt:              time.Now().UTC(),
		LatenciesMS:            map[string]float64{},
	}

	if err := executeSmoke(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, receiptClient, &result); err != nil {
		result.Error = err.Error()
	}
	result.FinishedAt = time.Now().UTC()
	result.Success = result.Error == ""
	if err := writeSummary(cfg, result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("receipt smoke failed: %s", result.Error)
	}
	return nil
}

func executeSmoke(
	ctx context.Context,
	cfg config,
	pool *pgxpool.Pool,
	conversationClient conversationv1.ConversationServiceClient,
	messageClient messagev1.MessageServiceClient,
	deliveryClient deliveryv1.DeliveryServiceClient,
	receiptClient receiptv1.ReceiptServiceClient,
	result *summary,
) error {
	begin := time.Now()
	join, err := createReceiverJoin(ctx, cfg, conversationClient)
	result.LatenciesMS["create_member_join"] = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("create receiver join: %w", err)
	}
	result.MemberJoin = memberJoinSummary{
		ChangeID:          join.GetChangeId(),
		BoundarySeq:       join.GetBoundarySeq(),
		MemberVersion:     join.GetMemberVersion(),
		PermissionVersion: join.GetPermissionVersion(),
	}
	if err := waitMembership(ctx, pool, cfg); err != nil {
		return err
	}

	begin = time.Now()
	send, err := sendMessage(ctx, cfg, messageClient, 1)
	result.LatenciesMS["send_message"] = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	result.SendMessage = sendSummary{
		MessageID:       send.GetMessageId(),
		ConversationSeq: send.GetConversationSeq(),
	}

	pull, err := pullInboxAtLeast(ctx, cfg, deliveryClient, send.GetConversationSeq())
	if err != nil {
		return fmt.Errorf("pull inbox: %w", err)
	}
	result.PullInbox = pull
	if pull.MaxSeq < send.GetConversationSeq() {
		return fmt.Errorf("pull inbox max seq %d did not reach sent seq %d", pull.MaxSeq, send.GetConversationSeq())
	}

	begin = time.Now()
	ackResponse, err := ackDelivery(ctx, cfg, deliveryClient, send.GetConversationSeq())
	result.AckDelivery.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("ack delivery: %w", err)
	}
	result.AckDelivery.LastReceivedSeq = ackResponse.GetLastReceivedSeq()
	if result.AckDelivery.LastReceivedSeq < send.GetConversationSeq() {
		return fmt.Errorf("ack last_received_seq %d did not reach sent seq %d", result.AckDelivery.LastReceivedSeq, send.GetConversationSeq())
	}

	if err := waitReceiptReceived(ctx, pool, cfg, send.GetConversationSeq()); err != nil {
		return err
	}
	before, err := getReceiptBySeq(ctx, cfg, receiptClient, send.GetConversationSeq())
	if err != nil {
		return fmt.Errorf("get receipt before read by seq: %w", err)
	}
	result.ReceiptBeforeReadBySeq = before
	receiverBefore := findReceiver(before, cfg.receiverUserID)
	if receiverBefore.ReceivedSeq != send.GetConversationSeq() || receiverBefore.ReadSeq != 0 {
		return fmt.Errorf("unexpected receipt before read receiver=%+v", receiverBefore)
	}
	begin = time.Now()
	conversationListBefore, err := listConversations(ctx, cfg, receiptClient, false, false)
	conversationListBefore.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations before read: %w", err)
	}
	result.ConversationListBefore = conversationListBefore
	if err := assertConversationListState(conversationListBefore, cfg.conversationID, send.GetConversationSeq(), 1, 0); err != nil {
		return fmt.Errorf("conversation list before read: %w", err)
	}

	begin = time.Now()
	conversationListUnreadBefore, err := listConversations(ctx, cfg, receiptClient, false, true)
	conversationListUnreadBefore.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list unread conversations before read: %w", err)
	}
	result.ConversationListUnreadBeforeRead = conversationListUnreadBefore
	if err := assertConversationListState(conversationListUnreadBefore, cfg.conversationID, send.GetConversationSeq(), 1, 0); err != nil {
		return fmt.Errorf("unread conversation list before read: %w", err)
	}

	begin = time.Now()
	markResponse, err := markRead(ctx, cfg, receiptClient, send.GetConversationSeq())
	result.MarkRead.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	result.MarkRead.LastReadSeq = markResponse.GetLastReadSeq()
	if result.MarkRead.LastReadSeq != send.GetConversationSeq() {
		return fmt.Errorf("mark read last_read_seq %d did not match sent seq %d", result.MarkRead.LastReadSeq, send.GetConversationSeq())
	}

	afterSeq, err := getReceiptBySeq(ctx, cfg, receiptClient, send.GetConversationSeq())
	if err != nil {
		return fmt.Errorf("get receipt after read by seq: %w", err)
	}
	result.ReceiptAfterReadBySeq = afterSeq
	afterMessageID, err := getReceiptByMessageID(ctx, cfg, receiptClient, send.GetMessageId())
	if err != nil {
		return fmt.Errorf("get receipt after read by message_id: %w", err)
	}
	result.ReceiptAfterReadByMsgID = afterMessageID
	receiverAfter := findReceiver(afterSeq, cfg.receiverUserID)
	if receiverAfter.ReadSeq != send.GetConversationSeq() {
		return fmt.Errorf("unexpected receipt after read receiver=%+v", receiverAfter)
	}
	begin = time.Now()
	conversationListAfter, err := listConversations(ctx, cfg, receiptClient, false, false)
	conversationListAfter.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after read: %w", err)
	}
	result.ConversationListAfter = conversationListAfter
	if err := assertConversationListState(conversationListAfter, cfg.conversationID, send.GetConversationSeq(), 0, send.GetConversationSeq()); err != nil {
		return fmt.Errorf("conversation list after read: %w", err)
	}

	begin = time.Now()
	conversationListUnreadAfter, err := listConversations(ctx, cfg, receiptClient, false, true)
	conversationListUnreadAfter.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list unread conversations after read: %w", err)
	}
	result.ConversationListUnreadAfterRead = conversationListUnreadAfter
	if err := assertConversationListHidden(conversationListUnreadAfter); err != nil {
		return fmt.Errorf("unread conversation list after read: %w", err)
	}

	begin = time.Now()
	archiveResponse, err := archiveConversation(ctx, cfg, receiptClient, true)
	result.ArchiveConversation.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("archive conversation: %w", err)
	}
	result.ArchiveConversation.Archived = archiveResponse.GetConversation().GetArchived()
	if !result.ArchiveConversation.Archived {
		return fmt.Errorf("archive response did not mark conversation archived: %+v", archiveResponse.GetConversation())
	}

	begin = time.Now()
	archivedDefault, err := listConversations(ctx, cfg, receiptClient, false, false)
	archivedDefault.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after archive default: %w", err)
	}
	result.ConversationListArchivedDefault = archivedDefault
	if err := assertConversationListHidden(archivedDefault); err != nil {
		return fmt.Errorf("conversation list archive default: %w", err)
	}

	begin = time.Now()
	archivedIncluded, err := listConversations(ctx, cfg, receiptClient, true, false)
	archivedIncluded.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after archive include_archived: %w", err)
	}
	result.ConversationListArchivedIncluded = archivedIncluded
	if err := assertConversationListArchived(archivedIncluded, cfg.conversationID, send.GetConversationSeq(), true); err != nil {
		return fmt.Errorf("conversation list archive included: %w", err)
	}

	begin = time.Now()
	sendWhileArchived, err := sendMessage(ctx, cfg, messageClient, 2)
	result.LatenciesMS["send_message_while_archived"] = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("send message while archived: %w", err)
	}
	result.SendMessageWhileArchived = sendSummary{
		MessageID:       sendWhileArchived.GetMessageId(),
		ConversationSeq: sendWhileArchived.GetConversationSeq(),
	}

	pullWhileArchived, err := pullInboxAtLeast(ctx, cfg, deliveryClient, sendWhileArchived.GetConversationSeq())
	if err != nil {
		return fmt.Errorf("pull inbox while archived: %w", err)
	}
	result.PullInboxWhileArchived = pullWhileArchived
	if pullWhileArchived.MaxSeq < sendWhileArchived.GetConversationSeq() {
		return fmt.Errorf("pull inbox while archived max seq %d did not reach sent seq %d", pullWhileArchived.MaxSeq, sendWhileArchived.GetConversationSeq())
	}

	begin = time.Now()
	ackWhileArchived, err := ackDelivery(ctx, cfg, deliveryClient, sendWhileArchived.GetConversationSeq())
	result.AckDeliveryWhileArchived.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("ack delivery while archived: %w", err)
	}
	result.AckDeliveryWhileArchived.LastReceivedSeq = ackWhileArchived.GetLastReceivedSeq()
	if result.AckDeliveryWhileArchived.LastReceivedSeq < sendWhileArchived.GetConversationSeq() {
		return fmt.Errorf("ack while archived last_received_seq %d did not reach sent seq %d", result.AckDeliveryWhileArchived.LastReceivedSeq, sendWhileArchived.GetConversationSeq())
	}
	if err := waitReceiptReceived(ctx, pool, cfg, sendWhileArchived.GetConversationSeq()); err != nil {
		return err
	}

	begin = time.Now()
	archivedNewDefault, err := listConversations(ctx, cfg, receiptClient, false, false)
	archivedNewDefault.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after archived new message default: %w", err)
	}
	result.ConversationListAfterArchivedNewDefault = archivedNewDefault
	if err := assertConversationListHidden(archivedNewDefault); err != nil {
		return fmt.Errorf("conversation list after archived new message default: %w", err)
	}

	begin = time.Now()
	archivedNewIncluded, err := listConversations(ctx, cfg, receiptClient, true, false)
	archivedNewIncluded.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after archived new message include_archived: %w", err)
	}
	result.ConversationListAfterArchivedNewIncluded = archivedNewIncluded
	if err := assertConversationListArchived(archivedNewIncluded, cfg.conversationID, sendWhileArchived.GetConversationSeq(), true); err != nil {
		return fmt.Errorf("conversation list after archived new message included: %w", err)
	}

	begin = time.Now()
	unarchiveResponse, err := archiveConversation(ctx, cfg, receiptClient, false)
	result.UnarchiveConversation.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("unarchive conversation: %w", err)
	}
	result.UnarchiveConversation.Archived = unarchiveResponse.GetConversation().GetArchived()
	if result.UnarchiveConversation.Archived {
		return fmt.Errorf("unarchive response still marked conversation archived: %+v", unarchiveResponse.GetConversation())
	}

	begin = time.Now()
	afterUnarchive, err := listConversations(ctx, cfg, receiptClient, false, false)
	afterUnarchive.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after unarchive: %w", err)
	}
	result.ConversationListAfterUnarchive = afterUnarchive
	if err := assertConversationListArchived(afterUnarchive, cfg.conversationID, sendWhileArchived.GetConversationSeq(), false); err != nil {
		return fmt.Errorf("conversation list after unarchive: %w", err)
	}

	begin = time.Now()
	pinResponse, err := pinConversation(ctx, cfg, receiptClient, true)
	result.PinConversation.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("pin conversation: %w", err)
	}
	result.PinConversation.Pinned = pinResponse.GetConversation().GetPinned()
	if !result.PinConversation.Pinned {
		return fmt.Errorf("pin response did not mark conversation pinned: %+v", pinResponse.GetConversation())
	}

	begin = time.Now()
	afterPin, err := listConversations(ctx, cfg, receiptClient, false, false)
	afterPin.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after pin: %w", err)
	}
	result.ConversationListAfterPin = afterPin
	if err := assertConversationListPinned(afterPin, cfg.conversationID, sendWhileArchived.GetConversationSeq(), true); err != nil {
		return fmt.Errorf("conversation list after pin: %w", err)
	}

	begin = time.Now()
	unpinResponse, err := pinConversation(ctx, cfg, receiptClient, false)
	result.UnpinConversation.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("unpin conversation: %w", err)
	}
	result.UnpinConversation.Pinned = unpinResponse.GetConversation().GetPinned()
	if result.UnpinConversation.Pinned {
		return fmt.Errorf("unpin response still marked conversation pinned: %+v", unpinResponse.GetConversation())
	}

	begin = time.Now()
	afterUnpin, err := listConversations(ctx, cfg, receiptClient, false, false)
	afterUnpin.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after unpin: %w", err)
	}
	result.ConversationListAfterUnpin = afterUnpin
	if err := assertConversationListPinned(afterUnpin, cfg.conversationID, sendWhileArchived.GetConversationSeq(), false); err != nil {
		return fmt.Errorf("conversation list after unpin: %w", err)
	}

	begin = time.Now()
	muteResponse, err := muteConversation(ctx, cfg, receiptClient, true)
	result.MuteConversation.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("mute conversation: %w", err)
	}
	result.MuteConversation.Muted = muteResponse.GetConversation().GetMuted()
	if !result.MuteConversation.Muted {
		return fmt.Errorf("mute response did not mark conversation muted: %+v", muteResponse.GetConversation())
	}

	begin = time.Now()
	afterMute, err := listConversations(ctx, cfg, receiptClient, false, false)
	afterMute.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after mute: %w", err)
	}
	result.ConversationListAfterMute = afterMute
	if err := assertConversationListMuted(afterMute, cfg.conversationID, sendWhileArchived.GetConversationSeq(), 1, send.GetConversationSeq(), true); err != nil {
		return fmt.Errorf("conversation list after mute: %w", err)
	}

	begin = time.Now()
	unmuteResponse, err := muteConversation(ctx, cfg, receiptClient, false)
	result.UnmuteConversation.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("unmute conversation: %w", err)
	}
	result.UnmuteConversation.Muted = unmuteResponse.GetConversation().GetMuted()
	if result.UnmuteConversation.Muted {
		return fmt.Errorf("unmute response still marked conversation muted: %+v", unmuteResponse.GetConversation())
	}

	begin = time.Now()
	afterUnmute, err := listConversations(ctx, cfg, receiptClient, false, false)
	afterUnmute.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after unmute: %w", err)
	}
	result.ConversationListAfterUnmute = afterUnmute
	if err := assertConversationListMuted(afterUnmute, cfg.conversationID, sendWhileArchived.GetConversationSeq(), 1, send.GetConversationSeq(), false); err != nil {
		return fmt.Errorf("conversation list after unmute: %w", err)
	}

	tooFar, err := markRead(ctx, cfg, receiptClient, sendWhileArchived.GetConversationSeq()+1)
	if err == nil {
		return fmt.Errorf("mark read too far unexpectedly succeeded: %+v", tooFar)
	}
	statusErr, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("mark read too far returned non-gRPC error: %w", err)
	}
	result.MarkReadTooFar = negativeCallSummary{
		Code:    statusErr.Code().String(),
		Message: statusErr.Message(),
		Passed:  statusErr.Code() == codes.FailedPrecondition,
	}
	if !result.MarkReadTooFar.Passed {
		return fmt.Errorf("mark read too far code=%s message=%s", statusErr.Code(), statusErr.Message())
	}

	if err := waitReceiptOutboxPublished(ctx, pool, cfg, 3); err != nil {
		return err
	}
	if err := fillPostgresStats(ctx, pool, cfg, result); err != nil {
		return err
	}
	events, err := readReceiptEvents(ctx, cfg, result.ReceiptOutbox.ByEventType)
	if err != nil {
		return err
	}
	result.ReceiptKafkaEvents = events
	return nil
}

func createReceiverJoin(
	ctx context.Context,
	cfg config,
	client conversationv1.ConversationServiceClient,
) (*conversationv1.CreateMemberChangeResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := ownerAuth(cfg, "receipt-smoke-join", "receipt-smoke-join")
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.CreateMemberChange(requestCtx, &conversationv1.CreateMemberChangeRequest{
		AuthContext:           conversationAuth(auth),
		ConversationId:        cfg.conversationID,
		TargetUserId:          cfg.receiverUserID,
		ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN,
		TargetRole:            conversationv1.MemberRole_MEMBER_ROLE_MEMBER,
		ExpectedMemberVersion: 1,
		IdempotencyKey:        "receipt-smoke-join-receiver",
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                "receipt smoke receiver join",
	})
}

func sendMessage(ctx context.Context, cfg config, client messagev1.MessageServiceClient, index int) (*messagev1.SendMessageResponse, error) {
	payload, err := structpb.NewStruct(map[string]any{"text": fmt.Sprintf("receipt smoke %d", index)})
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := ownerAuth(cfg, "receipt-smoke-send", fmt.Sprintf("receipt-smoke-send-%d", index))
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.SendMessage(requestCtx, &messagev1.SendMessageRequest{
		AuthContext:    messageAuth(auth),
		ConversationId: cfg.conversationID,
		ClientMsgId:    fmt.Sprintf("receipt-smoke-client-message-%d", index),
		MessageType:    "TEXT",
		Payload:        payload,
	})
}

func pullInboxAtLeast(
	ctx context.Context,
	cfg config,
	client deliveryv1.DeliveryServiceClient,
	minSeq int64,
) (pullSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	latencies := make([]float64, 0, 8)
	for {
		requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
		auth := receiverAuth(cfg, "receipt-smoke-pull", "receipt-smoke-pull")
		requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
		begin := time.Now()
		response, err := client.PullInbox(requestCtx, &deliveryv1.PullInboxRequest{
			AuthContext:    deliveryAuth(auth),
			ConversationId: cfg.conversationID,
			AfterSeq:       0,
			Limit:          100,
		})
		latencies = append(latencies, elapsedMS(begin))
		cancel()
		if err != nil {
			return pullSummary{}, err
		}
		result := pullSummary{ItemCount: len(response.GetItems())}
		for _, inboxItem := range response.GetItems() {
			if inboxItem.GetConversationSeq() > result.MaxSeq {
				result.MaxSeq = inboxItem.GetConversationSeq()
			}
			result.Items = append(result.Items, pulledItem{
				ConversationSeq: inboxItem.GetConversationSeq(),
				EventID:         inboxItem.GetEventId(),
				MessageID:       inboxItem.GetMessageId(),
				SenderID:        inboxItem.GetSenderId(),
			})
		}
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].ConversationSeq < result.Items[j].ConversationSeq
		})
		result.P95MS = percentile(latencies, 0.95)
		result.P99MS = percentile(latencies, 0.99)
		if result.MaxSeq >= minSeq || time.Now().After(deadline) {
			if result.MaxSeq < minSeq {
				return result, fmt.Errorf("pull inbox timeout: max_seq=%d want>=%d", result.MaxSeq, minSeq)
			}
			return result, nil
		}
		time.Sleep(cfg.pollInterval)
	}
}

func ackDelivery(
	ctx context.Context,
	cfg config,
	client deliveryv1.DeliveryServiceClient,
	seq int64,
) (*deliveryv1.AckDeliveryResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := receiverAuth(cfg, "receipt-smoke-ack", "receipt-smoke-ack")
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.AckDelivery(requestCtx, &deliveryv1.AckDeliveryRequest{
		AuthContext:    deliveryAuth(auth),
		ConversationId: cfg.conversationID,
		ReceivedSeq:    seq,
	})
}

func getReceiptBySeq(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	seq int64,
) (receiptStateSummary, error) {
	response, err := getReceipt(ctx, cfg, client, "", seq)
	if err != nil {
		return receiptStateSummary{}, err
	}
	return summarizeReceipt("conversation_seq", response), nil
}

func getReceiptByMessageID(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	messageID string,
) (receiptStateSummary, error) {
	response, err := getReceipt(ctx, cfg, client, messageID, 0)
	if err != nil {
		return receiptStateSummary{}, err
	}
	return summarizeReceipt("message_id", response), nil
}

func getReceipt(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	messageID string,
	seq int64,
) (*receiptv1.GetReceiptStateResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := ownerAuth(cfg, "receipt-smoke-get", "receipt-smoke-get")
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.GetReceiptState(requestCtx, &receiptv1.GetReceiptStateRequest{
		AuthContext:     receiptAuth(auth),
		ConversationId:  cfg.conversationID,
		MessageId:       messageID,
		ConversationSeq: seq,
	})
}

func listConversations(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	includeArchived bool,
	unreadOnly bool,
) (conversationListSummary, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := receiverAuth(cfg, "receipt-smoke-list", "receipt-smoke-list")
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	response, err := client.ListConversations(requestCtx, &receiptv1.ListConversationsRequest{
		AuthContext:     receiptAuth(auth),
		Limit:           10,
		IncludeArchived: includeArchived,
		UnreadOnly:      unreadOnly,
	})
	if err != nil {
		return conversationListSummary{}, err
	}
	return summarizeConversationList(response), nil
}

func archiveConversation(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	archived bool,
) (*receiptv1.ArchiveConversationResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := receiverAuth(cfg, "receipt-smoke-archive", fmt.Sprintf("receipt-smoke-archive-%v", archived))
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.ArchiveConversation(requestCtx, &receiptv1.ArchiveConversationRequest{
		AuthContext:    receiptAuth(auth),
		ConversationId: cfg.conversationID,
		Archived:       archived,
	})
}

func pinConversation(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	pinned bool,
) (*receiptv1.PinConversationResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := receiverAuth(cfg, "receipt-smoke-pin", fmt.Sprintf("receipt-smoke-pin-%v", pinned))
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.PinConversation(requestCtx, &receiptv1.PinConversationRequest{
		AuthContext:    receiptAuth(auth),
		ConversationId: cfg.conversationID,
		Pinned:         pinned,
	})
}

func muteConversation(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	muted bool,
) (*receiptv1.MuteConversationResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := receiverAuth(cfg, "receipt-smoke-mute", fmt.Sprintf("receipt-smoke-mute-%v", muted))
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.MuteConversation(requestCtx, &receiptv1.MuteConversationRequest{
		AuthContext:    receiptAuth(auth),
		ConversationId: cfg.conversationID,
		Muted:          muted,
	})
}

func markRead(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	seq int64,
) (*receiptv1.MarkReadResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := receiverAuth(cfg, "receipt-smoke-mark-read", fmt.Sprintf("receipt-smoke-mark-read-%d", seq))
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.MarkRead(requestCtx, &receiptv1.MarkReadRequest{
		AuthContext:    receiptAuth(auth),
		ConversationId: cfg.conversationID,
		ReadSeq:        seq,
	})
}

func summarizeConversationList(response *receiptv1.ListConversationsResponse) conversationListSummary {
	result := conversationListSummary{
		ItemCount:      len(response.GetItems()),
		Items:          []conversationSummaryItem{},
		NextPageCursor: response.GetNextPageCursor(),
	}
	if watermark := response.GetProjectionWatermark(); watermark != nil {
		result.ProjectionWatermark = projectionWatermarkSummary{
			Source:          watermark.GetSource(),
			OffsetValue:     watermark.GetOffsetValue(),
			UpdatedAtUnixMS: watermark.GetUpdatedAtUnixMs(),
		}
	}
	for _, item := range response.GetItems() {
		result.Items = append(result.Items, conversationSummaryItem{
			ConversationID:  item.GetConversationId(),
			LastVisibleSeq:  item.GetLastVisibleSeq(),
			LastMessageID:   item.GetLastMessageId(),
			LastSenderID:    item.GetLastSenderId(),
			UnreadCount:     item.GetUnreadCount(),
			LastReadSeq:     item.GetLastReadSeq(),
			UpdatedAtUnixMS: item.GetUpdatedAtUnixMs(),
			Archived:        item.GetArchived(),
			Pinned:          item.GetPinned(),
			Muted:           item.GetMuted(),
		})
	}
	return result
}

func assertConversationListState(
	state conversationListSummary,
	conversationID string,
	seq int64,
	unread int64,
	readSeq int64,
) error {
	if len(state.Items) != 1 {
		return fmt.Errorf("expected 1 item, got %d", len(state.Items))
	}
	item := state.Items[0]
	if item.ConversationID != conversationID {
		return fmt.Errorf("conversation_id=%s want=%s", item.ConversationID, conversationID)
	}
	if item.LastVisibleSeq != seq || item.UnreadCount != unread || item.LastReadSeq != readSeq {
		return fmt.Errorf("unexpected item state: %+v", item)
	}
	if item.Archived {
		return fmt.Errorf("expected visible item not archived: %+v", item)
	}
	if item.LastMessageID == "" || item.LastSenderID == "" {
		return fmt.Errorf("missing last message fields: %+v", item)
	}
	return nil
}

func assertConversationListHidden(state conversationListSummary) error {
	if len(state.Items) != 0 {
		return fmt.Errorf("expected archived conversation hidden, got %+v", state.Items)
	}
	return nil
}

func assertConversationListArchived(
	state conversationListSummary,
	conversationID string,
	seq int64,
	archived bool,
) error {
	if len(state.Items) != 1 {
		return fmt.Errorf("expected 1 item, got %d", len(state.Items))
	}
	item := state.Items[0]
	if item.ConversationID != conversationID {
		return fmt.Errorf("conversation_id=%s want=%s", item.ConversationID, conversationID)
	}
	if item.LastVisibleSeq != seq || item.Archived != archived {
		return fmt.Errorf("unexpected archived item state: %+v", item)
	}
	return nil
}

func assertConversationListPinned(
	state conversationListSummary,
	conversationID string,
	seq int64,
	pinned bool,
) error {
	if len(state.Items) != 1 {
		return fmt.Errorf("expected 1 item, got %d", len(state.Items))
	}
	item := state.Items[0]
	if item.ConversationID != conversationID {
		return fmt.Errorf("conversation_id=%s want=%s", item.ConversationID, conversationID)
	}
	if item.LastVisibleSeq != seq || item.Pinned != pinned || item.Archived {
		return fmt.Errorf("unexpected pinned item state: %+v", item)
	}
	return nil
}

func assertConversationListMuted(
	state conversationListSummary,
	conversationID string,
	seq int64,
	unread int64,
	readSeq int64,
	muted bool,
) error {
	if len(state.Items) != 1 {
		return fmt.Errorf("expected 1 item, got %d", len(state.Items))
	}
	item := state.Items[0]
	if item.ConversationID != conversationID {
		return fmt.Errorf("conversation_id=%s want=%s", item.ConversationID, conversationID)
	}
	if item.LastVisibleSeq != seq || item.UnreadCount != unread || item.LastReadSeq != readSeq || item.Muted != muted || item.Archived || item.Pinned {
		return fmt.Errorf("unexpected muted item state: %+v", item)
	}
	return nil
}

func summarizeReceipt(requestBy string, response *receiptv1.GetReceiptStateResponse) receiptStateSummary {
	result := receiptStateSummary{
		RequestBy:         requestBy,
		ConversationSeq:   response.GetConversationSeq(),
		MessageID:         response.GetMessageId(),
		ReceivedUserCount: response.GetReceivedUserCount(),
		ReadUserCount:     response.GetReadUserCount(),
		VisibilityMode:    response.GetVisibilityMode().String(),
	}
	for _, receiver := range response.GetReceivers() {
		result.Receivers = append(result.Receivers, receiptUserState{
			UserID:           receiver.GetUserId(),
			ReceivedSeq:      receiver.GetReceivedSeq(),
			ReceivedAtUnixMS: receiver.GetReceivedAtUnixMs(),
			ReadSeq:          receiver.GetReadSeq(),
			ReadAtUnixMS:     receiver.GetReadAtUnixMs(),
		})
	}
	return result
}

func findReceiver(state receiptStateSummary, userID string) receiptUserState {
	for _, receiver := range state.Receivers {
		if receiver.UserID == userID {
			return receiver
		}
	}
	return receiptUserState{}
}

func writeSummary(cfg config, result summary) error {
	result.Capacity = buildCapacitySummary(&result)
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode summary: %w", err)
	}
	path := filepath.Join(cfg.resultDir, "receipt-summary.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
	return nil
}
