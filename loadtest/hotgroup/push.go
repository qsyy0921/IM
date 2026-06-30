package main

import (
	"context"
	"fmt"
	"net/url"
	"time"

	nhooyr "nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	opClientHello             = "client.hello"
	opConversationSubscribe   = "conversation.subscribe"
	opConversationSubscribeOK = "conversation.subscribe.ok"
	opDeliveryNotify          = "delivery.notify"
	opServerHello             = "server.hello"
)

type hotgroupClientFrame struct {
	Op             string `json:"op"`
	RequestID      string `json:"request_id,omitempty"`
	DeviceID       string `json:"device_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
}

type hotgroupServerFrame struct {
	Op              string `json:"op"`
	RequestID       string `json:"request_id,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	EventID         string `json:"event_id,omitempty"`
	ConversationID  string `json:"conversation_id,omitempty"`
	ConversationSeq int64  `json:"conversation_seq,omitempty"`
	SourceEventType string `json:"source_event_type,omitempty"`
	PullRequired    bool   `json:"pull_required,omitempty"`
	Code            string `json:"code,omitempty"`
	Message         string `json:"message,omitempty"`
}

type pushSubscriber struct {
	user loadUser
	conn *nhooyr.Conn
}

type conversationSignalCollector struct {
	done            <-chan pushSignalResult
	cancel          context.CancelFunc
	subscriberCount int
	messageCount    int
	startedAt       time.Time
}

type pushSignalResult struct {
	userID             string
	deviceID           string
	count              int
	maxSeq             int64
	firstSignalAfterMS float64
	lastSignalAfterMS  float64
	completed          bool
	err                error
}

func openConversationSubscribers(ctx context.Context, cfg config, plan userPlan) ([]pushSubscriber, pushStats, error) {
	stats := pushStats{
		Enabled:              cfg.ConversationSubscriberCount > 0,
		PushURL:              cfg.PushURL,
		SubscriberTotalCount: cfg.ConversationSubscriberCount,
		SubscriberShardCount: cfg.SubscriberShardCount,
		SubscriberShardIndex: cfg.SubscriberShardIndex,
		StartedAt:            time.Now().UTC(),
	}
	if cfg.ConversationSubscriberCount == 0 {
		stats.FinishedAt = time.Now().UTC()
		return nil, stats, nil
	}
	receivers := shardReceivers(sampledReceivers(plan, cfg.ConversationSubscriberCount), cfg.SubscriberShardCount, cfg.SubscriberShardIndex)
	stats.SubscriberCount = len(receivers)
	if len(receivers) == 0 {
		stats.FinishedAt = time.Now().UTC()
		return nil, stats, fmt.Errorf("subscriber shard selects no receivers: total=%d shard_index=%d shard_count=%d", cfg.ConversationSubscriberCount, cfg.SubscriberShardIndex, cfg.SubscriberShardCount)
	}
	subscribers := make([]pushSubscriber, 0, len(receivers))
	for _, receiver := range receivers {
		conn, err := connectConversationSubscriber(ctx, cfg, receiver)
		if err != nil {
			stats.SubscribeErrorCount++
			if len(stats.Errors) < 20 {
				stats.Errors = append(stats.Errors, fmt.Sprintf("%s: %v", receiver.UserID, err))
			}
			closeConversationSubscribers(subscribers)
			stats.FinishedAt = time.Now().UTC()
			return nil, stats, err
		}
		stats.SubscribeSuccessCount++
		subscribers = append(subscribers, pushSubscriber{user: receiver, conn: conn})
	}
	stats.FinishedAt = time.Now().UTC()
	return subscribers, stats, nil
}

func shardReceivers(receivers []loadUser, shardCount int, shardIndex int) []loadUser {
	if shardCount <= 1 {
		return append([]loadUser(nil), receivers...)
	}
	if shardIndex < 0 || shardIndex >= shardCount {
		return nil
	}
	sharded := make([]loadUser, 0, (len(receivers)+shardCount-1)/shardCount)
	for index, receiver := range receivers {
		if index%shardCount == shardIndex {
			sharded = append(sharded, receiver)
		}
	}
	return sharded
}

func connectConversationSubscriber(ctx context.Context, cfg config, user loadUser) (*nhooyr.Conn, error) {
	target, err := websocketURLForUser(cfg, user)
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	conn, _, err := nhooyr.Dial(requestCtx, target, nil)
	if err != nil {
		return nil, fmt.Errorf("dial websocket: %w", err)
	}
	helloRequestID := "hotgroup-hello-" + user.UserID
	if err := wsjson.Write(requestCtx, conn, hotgroupClientFrame{
		Op:        opClientHello,
		RequestID: helloRequestID,
		DeviceID:  user.DeviceID,
	}); err != nil {
		_ = conn.Close(nhooyr.StatusInternalError, "hello write failed")
		return nil, fmt.Errorf("write client hello: %w", err)
	}
	hello, err := readHotgroupFrame(requestCtx, conn)
	if err != nil {
		_ = conn.Close(nhooyr.StatusInternalError, "hello read failed")
		return nil, fmt.Errorf("read server hello: %w", err)
	}
	if hello.Op != opServerHello {
		_ = conn.Close(nhooyr.StatusInternalError, "unexpected hello")
		return nil, fmt.Errorf("unexpected hello frame op=%s code=%s message=%s", hello.Op, hello.Code, hello.Message)
	}
	subscribeRequestID := "hotgroup-subscribe-" + user.UserID
	if err := wsjson.Write(requestCtx, conn, hotgroupClientFrame{
		Op:             opConversationSubscribe,
		RequestID:      subscribeRequestID,
		ConversationID: cfg.ConversationID,
	}); err != nil {
		_ = conn.Close(nhooyr.StatusInternalError, "subscribe write failed")
		return nil, fmt.Errorf("write conversation subscribe: %w", err)
	}
	ack, err := readHotgroupFrame(requestCtx, conn)
	if err != nil {
		_ = conn.Close(nhooyr.StatusInternalError, "subscribe read failed")
		return nil, fmt.Errorf("read subscribe ack: %w", err)
	}
	if ack.Op != opConversationSubscribeOK || ack.ConversationID != cfg.ConversationID {
		_ = conn.Close(nhooyr.StatusInternalError, "unexpected subscribe ack")
		return nil, fmt.Errorf("unexpected subscribe ack op=%s conversation=%s code=%s message=%s", ack.Op, ack.ConversationID, ack.Code, ack.Message)
	}
	return conn, nil
}

func startConversationSignalCollection(ctx context.Context, cfg config, subscribers []pushSubscriber, messageCount int) *conversationSignalCollector {
	if len(subscribers) == 0 {
		return nil
	}
	collectorCtx, cancel := context.WithCancel(ctx)
	done := make(chan pushSignalResult, len(subscribers))
	startedAt := time.Now()
	for _, subscriber := range subscribers {
		go collectSubscriberSignals(collectorCtx, cfg, subscriber, messageCount, startedAt, done)
	}
	return &conversationSignalCollector{
		done:            done,
		cancel:          cancel,
		subscriberCount: len(subscribers),
		messageCount:    messageCount,
		startedAt:       startedAt,
	}
}

func collectSubscriberSignals(
	ctx context.Context,
	cfg config,
	subscriber pushSubscriber,
	messageCount int,
	startedAt time.Time,
	done chan<- pushSignalResult,
) {
	result := pushSignalResult{userID: subscriber.user.UserID, deviceID: subscriber.user.DeviceID}
	for result.count < messageCount {
		frame, err := readHotgroupFrame(ctx, subscriber.conn)
		if err != nil {
			result.err = err
			done <- result
			return
		}
		if frame.Op != opDeliveryNotify || frame.ConversationID != cfg.ConversationID {
			continue
		}
		if frame.ConversationSeq > result.maxSeq {
			result.maxSeq = frame.ConversationSeq
		}
		afterMS := float64(time.Since(startedAt).Microseconds()) / 1000
		if result.count == 0 {
			result.firstSignalAfterMS = afterMS
		}
		result.lastSignalAfterMS = afterMS
		result.count++
	}
	result.completed = true
	done <- result
}

func (collector *conversationSignalCollector) wait(cfg config, stats pushStats) (pushStats, error) {
	if collector == nil {
		stats.FinishedAt = time.Now().UTC()
		return stats, nil
	}
	defer collector.cancel()
	expected := collector.subscriberCount * collector.messageCount
	timer := time.NewTimer(cfg.WaitTimeout)
	defer timer.Stop()
	for received := 0; received < collector.subscriberCount; received++ {
		select {
		case result := <-collector.done:
			stats = recordPushSignalResult(stats, result)
			if result.err != nil && cfg.RequireConversationNotify {
				message := fmt.Sprintf("%s: read conversation signal: %v", result.userID, result.err)
				stats.FinishedAt = time.Now().UTC()
				return stats, fmt.Errorf("%s", message)
			}
		case <-timer.C:
			collector.cancel()
			stats = collector.drainPartialResults(stats, received)
			message := fmt.Sprintf(
				"timed out waiting for conversation signals: got=%d expected=%d",
				stats.ConversationSignalCount,
				expected,
			)
			stats.Errors = appendError(stats.Errors, message)
			stats.FinishedAt = time.Now().UTC()
			if cfg.RequireConversationNotify {
				return stats, fmt.Errorf("%s", message)
			}
			return stats, nil
		}
	}
	stats.FinishedAt = time.Now().UTC()
	if cfg.RequireConversationNotify && stats.ConversationSignalCount < expected {
		err := fmt.Errorf("conversation signal count = %d, want %d", stats.ConversationSignalCount, expected)
		stats.Errors = appendError(stats.Errors, err.Error())
		return stats, err
	}
	return stats, nil
}

func recordPushSignalResult(stats pushStats, result pushSignalResult) pushStats {
	stats.ConversationSignalCount += result.count
	if result.maxSeq > stats.MaxConversationSeq {
		stats.MaxConversationSeq = result.maxSeq
	}
	subscriber := pushSignalSubscriberStats{
		UserID:             result.userID,
		DeviceID:           result.deviceID,
		SignalCount:        result.count,
		MaxConversationSeq: result.maxSeq,
		FirstSignalAfterMS: result.firstSignalAfterMS,
		LastSignalAfterMS:  result.lastSignalAfterMS,
		Completed:          result.completed,
	}
	if result.err != nil {
		message := fmt.Sprintf("%s: read conversation signal: %v", result.userID, result.err)
		stats.Errors = appendError(stats.Errors, message)
		subscriber.Error = result.err.Error()
	}
	stats.SubscriberSignals = append(stats.SubscriberSignals, subscriber)
	return stats
}

func (collector *conversationSignalCollector) drainPartialResults(stats pushStats, alreadyReceived int) pushStats {
	timeout := time.NewTimer(500 * time.Millisecond)
	defer timeout.Stop()
	for received := alreadyReceived; received < collector.subscriberCount; {
		select {
		case result := <-collector.done:
			stats = recordPushSignalResult(stats, result)
			received++
		case <-timeout.C:
			return stats
		}
	}
	return stats
}

func websocketURLForUser(cfg config, user loadUser) (string, error) {
	parsed, err := url.Parse(cfg.PushURL)
	if err != nil {
		return "", fmt.Errorf("parse push url: %w", err)
	}
	query := parsed.Query()
	query.Set("tenant_id", cfg.TenantID)
	query.Set("user_id", user.UserID)
	query.Set("device_id", user.DeviceID)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func readHotgroupFrame(ctx context.Context, conn *nhooyr.Conn) (hotgroupServerFrame, error) {
	var frame hotgroupServerFrame
	if err := wsjson.Read(ctx, conn, &frame); err != nil {
		return hotgroupServerFrame{}, err
	}
	return frame, nil
}

func closeConversationSubscribers(subscribers []pushSubscriber) {
	for _, subscriber := range subscribers {
		_ = subscriber.conn.Close(nhooyr.StatusNormalClosure, "hotgroup done")
	}
}

func appendError(errors []string, value string) []string {
	if len(errors) >= 20 {
		return errors
	}
	return append(errors, value)
}
