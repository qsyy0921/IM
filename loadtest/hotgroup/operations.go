package main

import (
	"context"
	"fmt"
	"time"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

type serviceClients struct {
	conversationConn *grpc.ClientConn
	messageConn      *grpc.ClientConn
	deliveryConn     *grpc.ClientConn
	conversation     conversationv1.ConversationServiceClient
	message          messagev1.MessageServiceClient
	delivery         deliveryv1.DeliveryServiceClient
}

func newServiceClients(cfg config) (*serviceClients, error) {
	conversationOption, err := grpctls.DialOption(cfg.ConversationTLS, "conversation-tls")
	if err != nil {
		return nil, err
	}
	messageOption, err := grpctls.DialOption(cfg.MessageTLS, "message-tls")
	if err != nil {
		return nil, err
	}
	deliveryOption, err := grpctls.DialOption(cfg.DeliveryTLS, "delivery-tls")
	if err != nil {
		return nil, err
	}
	conversationConn, err := grpc.NewClient(cfg.ConversationTarget, conversationOption)
	if err != nil {
		return nil, err
	}
	messageConn, err := grpc.NewClient(cfg.MessageTarget, messageOption)
	if err != nil {
		_ = conversationConn.Close()
		return nil, err
	}
	deliveryConn, err := grpc.NewClient(cfg.DeliveryTarget, deliveryOption)
	if err != nil {
		_ = conversationConn.Close()
		_ = messageConn.Close()
		return nil, err
	}
	return &serviceClients{
		conversationConn: conversationConn,
		messageConn:      messageConn,
		deliveryConn:     deliveryConn,
		conversation:     conversationv1.NewConversationServiceClient(conversationConn),
		message:          messagev1.NewMessageServiceClient(messageConn),
		delivery:         deliveryv1.NewDeliveryServiceClient(deliveryConn),
	}, nil
}

func (clients *serviceClients) close() {
	if clients == nil {
		return
	}
	if clients.conversationConn != nil {
		_ = clients.conversationConn.Close()
	}
	if clients.messageConn != nil {
		_ = clients.messageConn.Close()
	}
	if clients.deliveryConn != nil {
		_ = clients.deliveryConn.Close()
	}
}

func createGroup(ctx context.Context, cfg config, clients *serviceClients, plan userPlan) (int64, error) {
	auth := userAuth(cfg, plan.Owner, "hotgroup-create", "hotgroup-create")
	callCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	callCtx = withVerifiedAuthMetadata(callCtx, cfg, auth)
	response, err := clients.conversation.CreateConversation(callCtx, &conversationv1.CreateConversationRequest{
		AuthContext:      conversationAuth(auth),
		ConversationId:   cfg.ConversationID,
		ConversationType: conversationv1.ConversationType_CONVERSATION_TYPE_GROUP,
		IdempotencyKey:   cfg.RunName + "-create-group",
	})
	if err != nil {
		return 0, fmt.Errorf("create group: %w", err)
	}
	return response.GetMemberVersion(), nil
}

func joinMembers(ctx context.Context, cfg config, clients *serviceClients, plan userPlan, memberVersion int64) (int64, error) {
	ownerAuth := userAuth(cfg, plan.Owner, "hotgroup-join", "hotgroup-join")
	for _, member := range append(plan.Senders, plan.Receivers...) {
		callCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
		requestID := "hotgroup-join-" + member.UserID
		auth := ownerAuth
		auth.requestID = requestID
		auth.traceID = requestID
		callCtx = withVerifiedAuthMetadata(callCtx, cfg, auth)
		response, err := clients.conversation.CreateMemberChange(callCtx, &conversationv1.CreateMemberChangeRequest{
			AuthContext:           conversationAuth(auth),
			ConversationId:        cfg.ConversationID,
			TargetUserId:          member.UserID,
			ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN,
			TargetRole:            conversationv1.MemberRole_MEMBER_ROLE_MEMBER,
			ExpectedMemberVersion: memberVersion,
			IdempotencyKey:        cfg.RunName + "-join-" + member.UserID,
			ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
			Reason:                "hotgroup loadtest member seed",
		})
		cancel()
		if err != nil {
			return memberVersion, fmt.Errorf("join member %s: %w", member.UserID, err)
		}
		memberVersion = response.GetMemberVersion()
	}
	return memberVersion, nil
}

func sendMessages(ctx context.Context, cfg config, clients *serviceClients, plan userPlan, messageCount int) sendStats {
	stats := sendStats{StartedAt: time.Now().UTC()}
	if messageCount <= 0 {
		stats.FinishedAt = time.Now().UTC()
		return stats
	}
	latencies := make([]float64, 0, messageCount)
	interval := time.Duration(float64(time.Second) / cfg.MessageRate)
	if interval <= 0 {
		interval = time.Millisecond
	}
	next := time.Now()
	for index := 0; index < messageCount; index++ {
		if wait := time.Until(next); wait > 0 {
			select {
			case <-ctx.Done():
				stats.Errors = append(stats.Errors, ctx.Err().Error())
				stats.FinishedAt = time.Now().UTC()
				return finalizeSendStats(stats, latencies)
			case <-time.After(wait):
			}
		}
		sender := plan.Senders[index%len(plan.Senders)]
		latency, seq, err := sendOneMessage(ctx, cfg, clients, sender, index+1)
		latencies = append(latencies, latency)
		if err != nil {
			stats.ErrorCount++
			if len(stats.Errors) < 20 {
				stats.Errors = append(stats.Errors, err.Error())
			}
		} else {
			stats.SuccessCount++
			if seq > stats.MaxSeq {
				stats.MaxSeq = seq
			}
		}
		next = next.Add(interval)
	}
	stats.FinishedAt = time.Now().UTC()
	return finalizeSendStats(stats, latencies)
}

func sendOneMessage(ctx context.Context, cfg config, clients *serviceClients, sender loadUser, index int) (float64, int64, error) {
	requestID := fmt.Sprintf("hotgroup-send-%06d", index)
	auth := userAuth(cfg, sender, requestID, requestID)
	callCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	callCtx = withVerifiedAuthMetadata(callCtx, cfg, auth)
	payload, err := structpb.NewStruct(map[string]any{
		"text":      fmt.Sprintf("NexusIM hotgroup message %06d from %s", index, sender.UserID),
		"run_name":  cfg.RunName,
		"hot_group": true,
	})
	if err != nil {
		return 0, 0, err
	}
	begin := time.Now()
	response, err := clients.message.SendMessage(callCtx, &messagev1.SendMessageRequest{
		AuthContext:    messageAuth(auth),
		ConversationId: cfg.ConversationID,
		ClientMsgId:    fmt.Sprintf("%s-%s-%06d", cfg.RunName, sender.UserID, index),
		MessageType:    "TEXT",
		Payload:        payload,
	})
	latency := float64(time.Since(begin).Microseconds()) / 1000
	if err != nil {
		return latency, 0, fmt.Errorf("send message %d: %w", index, err)
	}
	return latency, response.GetConversationSeq(), nil
}

func pullAndAckSample(ctx context.Context, cfg config, clients *serviceClients, plan userPlan, maxSeq int64) receiverStats {
	receivers := sampledReceivers(plan, cfg.ReceiverSampleCount)
	stats := receiverStats{SampledReceivers: len(receivers)}
	latencies := make([]float64, 0, len(receivers))
	ackLimit := int(float64(len(receivers))*cfg.ACKRatio + 0.000001)
	for index, receiver := range receivers {
		latency, pulledSeq, err := pullReceiver(ctx, cfg, clients, receiver)
		latencies = append(latencies, latency)
		if err != nil {
			stats.PullErrorCount++
			if len(stats.Errors) < 20 {
				stats.Errors = append(stats.Errors, err.Error())
			}
			continue
		}
		stats.PullSuccessCount++
		if pulledSeq > stats.MaxPulledSeq {
			stats.MaxPulledSeq = pulledSeq
		}
		if index < ackLimit && pulledSeq > 0 {
			if err := ackReceiver(ctx, cfg, clients, receiver, pulledSeq); err != nil {
				stats.AckErrorCount++
				if len(stats.Errors) < 20 {
					stats.Errors = append(stats.Errors, err.Error())
				}
				continue
			}
			stats.AckSuccessCount++
		}
	}
	stats.PullLatencyAvgMS, stats.PullLatencyP95MS, stats.PullLatencyP99MS = summarizeLatencies(latencies)
	if maxSeq > 0 && stats.MaxPulledSeq > maxSeq {
		stats.MaxPulledSeq = maxSeq
	}
	return stats
}

func pullReceiver(ctx context.Context, cfg config, clients *serviceClients, receiver loadUser) (float64, int64, error) {
	requestID := "hotgroup-pull-" + receiver.UserID
	auth := userAuth(cfg, receiver, requestID, requestID)
	callCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	callCtx = withVerifiedAuthMetadata(callCtx, cfg, auth)
	begin := time.Now()
	response, err := clients.delivery.PullInbox(callCtx, &deliveryv1.PullInboxRequest{
		AuthContext:    deliveryAuth(auth),
		ConversationId: cfg.ConversationID,
		AfterSeq:       0,
		Limit:          cfg.PullLimit,
	})
	latency := float64(time.Since(begin).Microseconds()) / 1000
	if err != nil {
		return latency, 0, fmt.Errorf("pull receiver %s: %w", receiver.UserID, err)
	}
	return latency, response.GetNextSeq(), nil
}

func ackReceiver(ctx context.Context, cfg config, clients *serviceClients, receiver loadUser, seq int64) error {
	requestID := "hotgroup-ack-" + receiver.UserID
	auth := userAuth(cfg, receiver, requestID, requestID)
	callCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	callCtx = withVerifiedAuthMetadata(callCtx, cfg, auth)
	_, err := clients.delivery.AckDelivery(callCtx, &deliveryv1.AckDeliveryRequest{
		AuthContext:    deliveryAuth(auth),
		ConversationId: cfg.ConversationID,
		ReceivedSeq:    seq,
	})
	if err != nil {
		return fmt.Errorf("ack receiver %s: %w", receiver.UserID, err)
	}
	return nil
}

func finalizeSendStats(stats sendStats, latencies []float64) sendStats {
	stats.LatencyAvgMS, stats.LatencyP95MS, stats.LatencyP99MS = summarizeLatencies(latencies)
	return stats
}
