package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	nhooyr "nhooyr.io/websocket"
)

func runMessageChangeNotifyScenario(
	ctx context.Context,
	cfg config,
	pool *pgxpool.Pool,
	conversationClient conversationv1.ConversationServiceClient,
	messageClient messagev1.MessageServiceClient,
	deliveryClient deliveryv1.DeliveryServiceClient,
	result *summary,
) error {
	expectedSourceType, err := sourceEventTypeForAction(cfg.messageChangeAction)
	if err != nil {
		return finish(cfg, result, err)
	}
	conn, hello, err := connectWebSocket(ctx, cfg, cfg.receiverDeviceID)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("connect websocket: %w", err))
	}
	defer conn.Close(nhooyr.StatusNormalClosure, "")
	result.ServerHello = snapshotFrame(hello)
	result.DeviceNotifications = append(result.DeviceNotifications, deviceSummary{
		DeviceID:    cfg.receiverDeviceID,
		ServerHello: snapshotFrame(hello),
	})

	begin := time.Now()
	join, err := createReceiverJoin(ctx, cfg, conversationClient)
	result.Latencies["create_member_join"] = elapsedMS(begin)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("create receiver join: %w", err))
	}
	result.MemberJoin = memberJoinSummary{
		ChangeID:          join.GetChangeId(),
		BoundarySeq:       join.GetBoundarySeq(),
		MemberVersion:     join.GetMemberVersion(),
		PermissionVersion: join.GetPermissionVersion(),
	}
	if err := waitMembership(ctx, pool, cfg); err != nil {
		return finish(cfg, result, err)
	}

	begin = time.Now()
	send, err := sendMessage(ctx, cfg, messageClient, 1)
	result.Latencies["send_message"] = elapsedMS(begin)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("send message: %w", err))
	}
	result.SendMessage = sendSummary{MessageID: send.GetMessageId(), ConversationSeq: send.GetConversationSeq()}
	notify, err := waitNotify(ctx, cfg, conn)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("wait persisted notify: %w", err))
	}
	result.DeliveryNotify = snapshotFrame(notify)
	result.DeviceNotifications[0].DeliveryNotify = snapshotFrame(notify)
	if notify.ConversationSeq != send.GetConversationSeq() ||
		notify.MessageID != send.GetMessageId() ||
		notify.SourceEventType != "message.persisted.v1" {
		return finish(cfg, result, fmt.Errorf("persisted notify mismatch: notify=%+v send=%+v", notify, send))
	}

	begin = time.Now()
	change, err := changeMessage(ctx, cfg, messageClient, send.GetMessageId())
	result.Latencies["message_change"] = elapsedMS(begin)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("%s message: %w", cfg.messageChangeAction, err))
	}
	result.MessageChange = messageChangeSummary{
		Action:          cfg.messageChangeAction,
		MessageID:       change.GetMessageId(),
		ConversationSeq: change.GetConversationSeq(),
		ChangeVersion:   change.GetChangeVersion(),
		SourceEventType: expectedSourceType,
	}
	changeNotify, err := waitNotify(ctx, cfg, conn)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("wait change notify: %w", err))
	}
	result.ChangeDeliveryNotify = snapshotFrame(changeNotify)
	if changeNotify.ConversationSeq != change.GetConversationSeq() ||
		changeNotify.MessageID != send.GetMessageId() ||
		changeNotify.SourceEventType != expectedSourceType {
		return finish(cfg, result, fmt.Errorf("change notify mismatch: notify=%+v change=%+v", changeNotify, change))
	}

	changePull, err := pullInboxUntilEvent(ctx, cfg, deliveryClient, send.GetConversationSeq(), expectedSourceType, send.GetMessageId(), change.GetConversationSeq())
	if err != nil {
		return finish(cfg, result, fmt.Errorf("pull inbox after change: %w", err))
	}
	result.ChangePullInbox = changePull
	result.PullInbox = changePull

	ackOK, err := ackViaWebSocket(ctx, cfg, conn, cfg.receiverDeviceID, change.GetConversationSeq())
	if err != nil {
		return finish(cfg, result, fmt.Errorf("websocket ack change: %w", err))
	}
	result.DeliveryAckOK = snapshotFrame(ackOK)
	result.DeviceNotifications[0].DeliveryAckOK = snapshotFrame(ackOK)
	if ackOK.LastReceivedSeq != change.GetConversationSeq() {
		return finish(cfg, result, fmt.Errorf("ack seq mismatch: %+v", ackOK))
	}
	if err := waitCursor(ctx, pool, cfg, cfg.receiverDeviceID, change.GetConversationSeq()); err != nil {
		return finish(cfg, result, err)
	}
	cursor, err := queryCursor(ctx, pool, cfg, cfg.receiverDeviceID)
	if err != nil {
		return finish(cfg, result, err)
	}
	result.CursorLastReceivedSeq = &cursor
	result.DeviceNotifications[0].CursorLastReceivedSeq = &cursor
	if err := waitDeliveryOutboxDrain(ctx, pool, cfg); err != nil {
		return finish(cfg, result, err)
	}
	if err := fillPostgresStats(ctx, pool, cfg, result); err != nil {
		return finish(cfg, result, err)
	}
	result.Success = true
	return finish(cfg, result, nil)
}

func runSlowClientScenario(
	ctx context.Context,
	cfg config,
	pool *pgxpool.Pool,
	conversationClient conversationv1.ConversationServiceClient,
	messageClient messagev1.MessageServiceClient,
	deliveryClient deliveryv1.DeliveryServiceClient,
	result *summary,
) error {
	conn, hello, err := connectWebSocket(ctx, cfg, cfg.receiverDeviceID)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("connect slow websocket: %w", err))
	}
	result.ServerHello = snapshotFrame(hello)

	begin := time.Now()
	join, err := createReceiverJoin(ctx, cfg, conversationClient)
	result.Latencies["create_member_join"] = elapsedMS(begin)
	if err != nil {
		conn.CloseNow()
		return finish(cfg, result, fmt.Errorf("create receiver join: %w", err))
	}
	result.MemberJoin = memberJoinSummary{
		ChangeID:          join.GetChangeId(),
		BoundarySeq:       join.GetBoundarySeq(),
		MemberVersion:     join.GetMemberVersion(),
		PermissionVersion: join.GetPermissionVersion(),
	}
	if err := waitMembership(ctx, pool, cfg); err != nil {
		conn.CloseNow()
		return finish(cfg, result, err)
	}

	beforeMetrics, _ := fetchPushMetrics(ctx, cfg.pushMetricsURL)
	result.PushMetricsBefore = &beforeMetrics

	var firstSeq int64
	var lastSeq int64
	begin = time.Now()
	for i := 1; i <= cfg.slowMessageCount; i++ {
		send, err := sendMessage(ctx, cfg, messageClient, i)
		if err != nil {
			conn.CloseNow()
			return finish(cfg, result, fmt.Errorf("send slow message %d: %w", i, err))
		}
		if firstSeq == 0 {
			firstSeq = send.GetConversationSeq()
			result.SendMessage = sendSummary{MessageID: send.GetMessageId(), ConversationSeq: send.GetConversationSeq()}
		}
		lastSeq = send.GetConversationSeq()
	}
	result.Latencies["send_messages"] = elapsedMS(begin)

	afterMetrics, err := waitPushEviction(ctx, cfg, beforeMetrics.SlowSessionEvictedCount)
	if err != nil {
		conn.CloseNow()
		return finish(cfg, result, err)
	}
	result.PushMetricsAfter = &afterMetrics

	readResult := readUntilResumeHintOrClose(ctx, cfg, conn)
	_ = conn.Close(nhooyr.StatusNormalClosure, "slow done")

	pull, err := pullInboxAtLeast(ctx, cfg, deliveryClient, 0, cfg.slowMessageCount+10, cfg.slowMessageCount, lastSeq)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("pull inbox after slow close: %w", err))
	}
	if pull.ItemCount < cfg.slowMessageCount || pull.MaxSeq < lastSeq {
		return finish(cfg, result, fmt.Errorf("pull inbox did not recover slow messages: count=%d max_seq=%d want_count=%d want_seq=%d", pull.ItemCount, pull.MaxSeq, cfg.slowMessageCount, lastSeq))
	}
	if err := waitDeliveryOutboxDrain(ctx, pool, cfg); err != nil {
		return finish(cfg, result, err)
	}

	reconnected, reconnectedHello, err := connectWebSocketWithResume(
		ctx,
		cfg,
		cfg.receiverDeviceID,
		hello.ResumeToken,
		[]cursor{{ConversationID: cfg.conversationID, Seq: pull.MaxSeq}},
	)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("reconnect websocket: %w", err))
	}
	defer reconnected.Close(nhooyr.StatusNormalClosure, "")

	ackOK, replayCount, err := ackViaWebSocketWithSkipped(ctx, cfg, reconnected, cfg.receiverDeviceID, pull.MaxSeq)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("ack after slow close: %w", err))
	}
	if err := waitCursor(ctx, pool, cfg, cfg.receiverDeviceID, pull.MaxSeq); err != nil {
		return finish(cfg, result, err)
	}
	cursorSeq, err := queryCursor(ctx, pool, cfg, cfg.receiverDeviceID)
	if err != nil {
		return finish(cfg, result, err)
	}
	result.CursorLastReceivedSeq = &cursorSeq
	if err := waitDeliveryOutboxDrain(ctx, pool, cfg); err != nil {
		return finish(cfg, result, err)
	}
	if err := fillPostgresStats(ctx, pool, cfg, result); err != nil {
		return finish(cfg, result, err)
	}

	result.SlowClient = &slowClientSummary{
		MessageCount:       cfg.slowMessageCount,
		FirstSeq:           firstSeq,
		LastSeq:            lastSeq,
		NotifyFramesRead:   readResult.notifyFrames,
		ResumeHintReceived: readResult.resumeHint.Op == opResumeHint,
		ResumeHint:         snapshotFrame(readResult.resumeHint),
		CloseStatus:        readResult.closeStatus,
		ReconnectedHello:   snapshotFrame(reconnectedHello),
		ReplayFramesRead:   replayCount,
		RecoveryPullInbox:  pull,
		AckOK:              snapshotFrame(ackOK),
	}
	result.PullInbox = pull
	result.DeliveryAckOK = snapshotFrame(ackOK)
	result.Success = true
	return finish(cfg, result, nil)
}

func runResumeReplayScenario(
	ctx context.Context,
	cfg config,
	pool *pgxpool.Pool,
	conversationClient conversationv1.ConversationServiceClient,
	messageClient messagev1.MessageServiceClient,
	deliveryClient deliveryv1.DeliveryServiceClient,
	result *summary,
) error {
	conn, hello, err := connectWebSocket(ctx, cfg, cfg.receiverDeviceID)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("connect websocket: %w", err))
	}
	result.ServerHello = snapshotFrame(hello)
	result.DeviceNotifications = []deviceSummary{{
		DeviceID:    cfg.receiverDeviceID,
		ServerHello: snapshotFrame(hello),
	}}

	begin := time.Now()
	join, err := createReceiverJoin(ctx, cfg, conversationClient)
	result.Latencies["create_member_join"] = elapsedMS(begin)
	if err != nil {
		conn.CloseNow()
		return finish(cfg, result, fmt.Errorf("create receiver join: %w", err))
	}
	result.MemberJoin = memberJoinSummary{
		ChangeID:          join.GetChangeId(),
		BoundarySeq:       join.GetBoundarySeq(),
		MemberVersion:     join.GetMemberVersion(),
		PermissionVersion: join.GetPermissionVersion(),
	}
	if err := waitMembership(ctx, pool, cfg); err != nil {
		conn.CloseNow()
		return finish(cfg, result, err)
	}

	if cfg.scenario == "redis-sentinel-failover" || cfg.scenario == "redis-sentinel-master-stop" {
		if strings.TrimSpace(cfg.redisFaultCommand) == "" {
			conn.CloseNow()
			return finish(cfg, result, fmt.Errorf("redis-fault-command is required for %s scenario", cfg.scenario))
		}
		output, err := executeCommand(ctx, cfg, cfg.redisFaultCommand)
		result.RedisFault = &redisFaultSummary{
			FaultCommand:    cfg.redisFaultCommand,
			CommandOutput:   output,
			NotifyReceived:  true,
			NotifyWaitError: "online notify is expected after failover command reports a new master",
		}
		if err != nil {
			conn.CloseNow()
			return finish(cfg, result, fmt.Errorf("execute redis sentinel failover command: %w", err))
		}
	}

	beforeMetrics, _ := fetchPushMetrics(ctx, cfg.pushMetricsURL)
	result.PushMetricsBefore = &beforeMetrics

	begin = time.Now()
	send, err := sendMessage(ctx, cfg, messageClient, 1)
	result.Latencies["send_message"] = elapsedMS(begin)
	if err != nil {
		conn.CloseNow()
		return finish(cfg, result, fmt.Errorf("send message: %w", err))
	}
	result.SendMessage = sendSummary{MessageID: send.GetMessageId(), ConversationSeq: send.GetConversationSeq()}

	notify, err := waitNotify(ctx, cfg, conn)
	if err != nil {
		conn.CloseNow()
		return finish(cfg, result, fmt.Errorf("wait original notify: %w", err))
	}
	if notify.ConversationSeq != send.GetConversationSeq() || notify.MessageID != send.GetMessageId() {
		conn.CloseNow()
		return finish(cfg, result, fmt.Errorf("original notify mismatch: notify=%+v send=%+v", notify, send))
	}
	result.DeliveryNotify = snapshotFrame(notify)
	result.DeviceNotifications[0].DeliveryNotify = snapshotFrame(notify)
	_ = conn.Close(nhooyr.StatusNormalClosure, "resume replay")

	replayMetricsBefore := fetchResumeGatewayMetrics(ctx, cfg)
	reconnectCfg := cfg
	reconnectCfg.pushURL = cfg.reconnectPushURL
	reconnected, reconnectedHello, err := connectWebSocketWithResume(
		ctx,
		reconnectCfg,
		cfg.receiverDeviceID,
		hello.ResumeToken,
		[]cursor{{ConversationID: cfg.conversationID, Seq: join.GetBoundarySeq()}},
	)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("reconnect websocket: %w", err))
	}
	defer reconnected.Close(nhooyr.StatusNormalClosure, "")

	replayed, err := waitNotify(ctx, reconnectCfg, reconnected)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("wait replayed notify: %w", err))
	}
	if replayed.EventID != notify.EventID ||
		replayed.ConversationSeq != notify.ConversationSeq ||
		replayed.MessageID != notify.MessageID {
		return finish(cfg, result, fmt.Errorf("replayed notify mismatch: original=%+v replayed=%+v", notify, replayed))
	}
	replayMetricsAfter := fetchResumeGatewayMetrics(ctx, cfg)

	pull, err := pullInbox(ctx, cfg, deliveryClient)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("pull inbox after replay: %w", err))
	}
	if pull.ItemCount == 0 || pull.MaxSeq < send.GetConversationSeq() {
		return finish(cfg, result, fmt.Errorf("pull inbox did not include replayed seq: %+v", pull))
	}
	result.PullInbox = pull

	ackOK, skipped, err := ackViaWebSocketWithSkipped(ctx, cfg, reconnected, cfg.receiverDeviceID, send.GetConversationSeq())
	if err != nil {
		return finish(cfg, result, fmt.Errorf("ack after replay: %w", err))
	}
	if skipped != 0 {
		return finish(cfg, result, fmt.Errorf("unexpected extra frames while acking after replay: skipped=%d", skipped))
	}
	if err := waitCursor(ctx, pool, cfg, cfg.receiverDeviceID, send.GetConversationSeq()); err != nil {
		return finish(cfg, result, err)
	}
	cursorSeq, err := queryCursor(ctx, pool, cfg, cfg.receiverDeviceID)
	if err != nil {
		return finish(cfg, result, err)
	}
	result.CursorLastReceivedSeq = &cursorSeq
	if err := waitDeliveryOutboxDrain(ctx, pool, cfg); err != nil {
		return finish(cfg, result, err)
	}
	if err := fillPostgresStats(ctx, pool, cfg, result); err != nil {
		return finish(cfg, result, err)
	}

	result.DeliveryAckOK = snapshotFrame(ackOK)
	result.DeviceNotifications[0].DeliveryAckOK = snapshotFrame(ackOK)
	result.DeviceNotifications[0].CursorLastReceivedSeq = &cursorSeq
	result.ResumeReplay = &resumeReplaySummary{
		OriginalHello:       snapshotFrame(hello),
		OriginalNotify:      snapshotFrame(notify),
		ReconnectedHello:    snapshotFrame(reconnectedHello),
		ReplayedNotify:      snapshotFrame(replayed),
		LastReceivedSeq:     join.GetBoundarySeq(),
		ReplayMetricsBefore: &replayMetricsBefore,
		ReplayMetricsAfter:  &replayMetricsAfter,
		PullInbox:           pull,
		AckOK:               snapshotFrame(ackOK),
	}
	result.Success = true
	return finish(cfg, result, nil)
}

func fetchResumeGatewayMetrics(ctx context.Context, cfg config) pushMetrics {
	metricsURL := cfg.pushMetricsURL
	if cfg.reconnectMetricsURL != "" {
		metricsURL = cfg.reconnectMetricsURL
	}
	metrics, _ := fetchPushMetrics(ctx, metricsURL)
	return metrics
}

func runRedisFaultScenario(
	ctx context.Context,
	cfg config,
	pool *pgxpool.Pool,
	conversationClient conversationv1.ConversationServiceClient,
	messageClient messagev1.MessageServiceClient,
	deliveryClient deliveryv1.DeliveryServiceClient,
	result *summary,
) error {
	if strings.TrimSpace(cfg.redisFaultCommand) == "" {
		return finish(cfg, result, errors.New("redis-fault-command is required for redis-fault scenario"))
	}
	conn, hello, err := connectWebSocket(ctx, cfg, cfg.receiverDeviceID)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("connect websocket before redis fault: %w", err))
	}
	defer conn.Close(nhooyr.StatusNormalClosure, "")
	result.ServerHello = snapshotFrame(hello)
	result.DeviceNotifications = []deviceSummary{{
		DeviceID:    cfg.receiverDeviceID,
		ServerHello: snapshotFrame(hello),
	}}

	begin := time.Now()
	join, err := createReceiverJoin(ctx, cfg, conversationClient)
	result.Latencies["create_member_join"] = elapsedMS(begin)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("create receiver join: %w", err))
	}
	result.MemberJoin = memberJoinSummary{
		ChangeID:          join.GetChangeId(),
		BoundarySeq:       join.GetBoundarySeq(),
		MemberVersion:     join.GetMemberVersion(),
		PermissionVersion: join.GetPermissionVersion(),
	}
	if err := waitMembership(ctx, pool, cfg); err != nil {
		return finish(cfg, result, err)
	}

	output, err := executeCommand(ctx, cfg, cfg.redisFaultCommand)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("execute redis fault command: %w", err))
	}

	begin = time.Now()
	send, err := sendMessage(ctx, cfg, messageClient, 1)
	result.Latencies["send_message"] = elapsedMS(begin)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("send message after redis fault: %w", err))
	}
	result.SendMessage = sendSummary{MessageID: send.GetMessageId(), ConversationSeq: send.GetConversationSeq()}

	fault := &redisFaultSummary{FaultCommand: cfg.redisFaultCommand, CommandOutput: output}
	notify, notifyErr := waitNotifyFor(ctx, cfg, conn, time.Second)
	if notifyErr == nil {
		fault.NotifyReceived = true
		fault.UnexpectedNotify = snapshotFrame(notify)
		fault.NotifyWaitError = "online notify still arrived within 1s during redis-fault observation window"
	} else {
		fault.NotifyReceived = false
		fault.NotifyWaitError = notifyErr.Error()
	}

	pull, err := pullInboxAtLeast(ctx, cfg, deliveryClient, 0, 100, 1, send.GetConversationSeq())
	if err != nil {
		return finish(cfg, result, fmt.Errorf("pull inbox after redis fault: %w", err))
	}
	result.PullInbox = pull
	fault.RecoveryPullInbox = pull
	if pull.ItemCount == 0 || pull.MaxSeq < send.GetConversationSeq() {
		result.RedisFault = fault
		return finish(cfg, result, fmt.Errorf("pull inbox did not recover redis fault message: %+v", pull))
	}

	if strings.TrimSpace(cfg.redisRestoreCommand) != "" {
		if _, err := executeCommand(ctx, cfg, cfg.redisRestoreCommand); err != nil {
			return finish(cfg, result, fmt.Errorf("execute redis restore command: %w", err))
		}
	}
	conn.CloseNow()
	reconnected, reconnectedHello, err := connectWebSocketWithResume(
		ctx,
		cfg,
		cfg.receiverDeviceID,
		hello.ResumeToken,
		[]cursor{{ConversationID: cfg.conversationID, Seq: pull.MaxSeq}},
	)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("reconnect websocket after redis restore: %w", err))
	}
	defer reconnected.Close(nhooyr.StatusNormalClosure, "")
	result.ServerHello = snapshotFrame(reconnectedHello)
	result.DeviceNotifications[0].ServerHello = snapshotFrame(reconnectedHello)

	ackOK, skipped, err := ackViaWebSocketWithSkipped(ctx, cfg, reconnected, cfg.receiverDeviceID, pull.MaxSeq)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("ack after redis fault: %w", err))
	}
	if skipped > 0 {
		return finish(cfg, result, fmt.Errorf("unexpected pushed frames while acking after redis fault: skipped=%d", skipped))
	}
	result.DeliveryAckOK = snapshotFrame(ackOK)
	result.DeviceNotifications[0].DeliveryAckOK = snapshotFrame(ackOK)
	fault.AckOK = snapshotFrame(ackOK)
	if err := waitCursor(ctx, pool, cfg, cfg.receiverDeviceID, pull.MaxSeq); err != nil {
		return finish(cfg, result, err)
	}
	cursorSeq, err := queryCursor(ctx, pool, cfg, cfg.receiverDeviceID)
	if err != nil {
		return finish(cfg, result, err)
	}
	result.CursorLastReceivedSeq = &cursorSeq
	result.DeviceNotifications[0].CursorLastReceivedSeq = &cursorSeq
	if err := waitDeliveryOutboxDrain(ctx, pool, cfg); err != nil {
		return finish(cfg, result, err)
	}
	if err := fillPostgresStats(ctx, pool, cfg, result); err != nil {
		return finish(cfg, result, err)
	}
	if result.DeliveryOutboxTotal != nil {
		fault.DeliveryOutboxTotal = *result.DeliveryOutboxTotal
	}
	result.RedisFault = fault
	result.Success = true
	return finish(cfg, result, nil)
}

func runIdentityRevokeScenario(ctx context.Context, cfg config, result *summary) error {
	if cfg.pushAuthMode != "hmac" {
		return finish(cfg, result, errors.New("identity-revoke scenario requires --push-auth-mode=hmac"))
	}
	if strings.TrimSpace(cfg.identityTarget) == "" {
		return finish(cfg, result, errors.New("identity-revoke scenario requires --identity-target"))
	}
	if cfg.identityRevokeScope != "device" && cfg.identityRevokeScope != "session" {
		return finish(cfg, result, fmt.Errorf("unsupported identity-revoke-scope: %s", cfg.identityRevokeScope))
	}
	token, err := gatewayTokenDetails(ctx, cfg, cfg.receiverDeviceID)
	if err != nil {
		return finish(cfg, result, err)
	}
	if cfg.identityRevokeScope == "session" && token.SessionID == "" {
		return finish(cfg, result, errors.New("identity-service returned empty session_id for session revoke smoke"))
	}
	conn, hello, err := connectWebSocketWithToken(ctx, cfg, cfg.receiverDeviceID, token.Token)
	if err != nil {
		return finish(cfg, result, fmt.Errorf("connect websocket before revoke: %w", err))
	}
	var (
		survivorToken gatewayTokenResult
		survivorConn  *nhooyr.Conn
		survivorHello serverFrame
	)
	if cfg.identityRevokeScope == "session" {
		survivorToken, err = gatewayTokenDetails(ctx, cfg, cfg.receiverDeviceID)
		if err != nil {
			conn.CloseNow()
			return finish(cfg, result, fmt.Errorf("issue survivor gateway token: %w", err))
		}
		survivorConn, survivorHello, err = connectWebSocketWithToken(ctx, cfg, cfg.receiverDeviceID, survivorToken.Token)
		if err != nil {
			conn.CloseNow()
			return finish(cfg, result, fmt.Errorf("connect survivor websocket before revoke: %w", err))
		}
		defer survivorConn.Close(nhooyr.StatusNormalClosure, "identity session revoke smoke survivor")
	}
	activeClose := make(chan slowReadResult, 1)
	go func() {
		activeClose <- readUntilResumeHintOrClose(ctx, cfg, conn)
	}()

	switch cfg.identityRevokeScope {
	case "device":
		err = revokeIdentityDevice(ctx, cfg)
	case "session":
		err = revokeIdentitySession(ctx, cfg, token.SessionID)
	}
	if err != nil {
		conn.CloseNow()
		return finish(cfg, result, err)
	}
	closeResult := <-activeClose
	if closeResult.resumeHint.Op != opResumeHint || closeResult.resumeHint.Reason != "identity_revoked" {
		conn.CloseNow()
		return finish(cfg, result, fmt.Errorf("expected identity revoked resume hint, got hint=%+v close=%s", closeResult.resumeHint, closeResult.closeStatus))
	}
	denied, attempts, err := waitWebSocketPermissionDenied(ctx, cfg, cfg.receiverDeviceID, token.Token)
	if err != nil {
		return finish(cfg, result, err)
	}
	identityRevoke := &identityRevokeSummary{
		InitialHello:           snapshotFrame(hello),
		Scope:                  cfg.identityRevokeScope,
		RevokedDeviceID:        cfg.receiverDeviceID,
		RevokedSessionID:       token.SessionID,
		ActiveCloseHint:        snapshotFrame(closeResult.resumeHint),
		ActiveCloseStatus:      closeResult.closeStatus,
		ActiveNotifyFramesRead: closeResult.notifyFrames,
		DeniedFrame:            snapshotFrame(denied),
		ReconnectAttempts:      attempts,
	}
	if cfg.identityRevokeScope == "session" {
		pong, err := pingWebSocket(ctx, cfg, survivorConn, "push-smoke-session-revoke-survivor-ping")
		if err != nil {
			return finish(cfg, result, fmt.Errorf("survivor session should remain connected: %w", err))
		}
		identityRevoke.SurvivorHello = snapshotFrame(survivorHello)
		identityRevoke.SurvivorPong = snapshotFrame(pong)
	}
	result.IdentityRevoke = identityRevoke
	result.ServerHello = snapshotFrame(hello)
	result.Success = true
	return finish(cfg, result, nil)
}
