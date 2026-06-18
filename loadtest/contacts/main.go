package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	gatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/gateway/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	contacteventsv1 "github.com/qsyy0921/IM/schemas/kafka/contacts/v1"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const defaultTenantID = "tenant-contacts-smoke"

func main() {
	cfg := parseConfig()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	cfg.gatewayAuthMode = strings.ToLower(strings.TrimSpace(cfg.gatewayAuthMode))
	cfg.gatewayAuthAudience = normalizedGatewayAuthAudience(cfg.gatewayAuthAudience)
	startedAt := time.Now().UTC()
	if cfg.duration > 0 && cfg.tenantID == defaultTenantID {
		cfg.tenantID = defaultTenantID + "-" + startedAt.Format("20060102150405")
	}
	if cfg.gatewayFacade && cfg.gatewayAuthMode == "" {
		return fmt.Errorf("--gateway-facade requires --gateway-auth-mode")
	}
	if cfg.gatewayAuthMode != "" && cfg.verifiedMetadata {
		return fmt.Errorf("--gateway-auth-mode and --verified-auth-metadata cannot be combined")
	}
	if cfg.gatewayAuthMode == "hmac" && strings.TrimSpace(cfg.gatewayAuthHMACSecret) == "" {
		return fmt.Errorf("--gateway-auth-hmac-secret is required when --gateway-auth-mode=hmac")
	}
	if cfg.gatewayAuthMode != "" && cfg.gatewayAuthMode != "mock" && cfg.gatewayAuthMode != "hmac" {
		return fmt.Errorf("unsupported gateway auth mode: %s", cfg.gatewayAuthMode)
	}
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	s := summary{
		Commit:                    gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:                gitOutput("rev-parse", "HEAD"),
		GitStatusShort:            gitOutput("status", "--short"),
		Target:                    cfg.target,
		TLSEnabled:                cfg.tls.Enabled(),
		ResultDir:                 cfg.resultDir,
		TenantID:                  cfg.tenantID,
		SenderUserID:              cfg.senderUserID,
		ReceiverUserID:            cfg.receiverUserID,
		Scenario:                  cfg.scenario,
		ContactTopic:              cfg.contactTopic,
		CapacityMode:              cfg.duration > 0,
		VUs:                       cfg.vus,
		ConfiguredDurationSeconds: cfg.duration.Seconds(),
		VerifiedAuthMetadata:      cfg.verifiedMetadata,
		GatewayFacade:             cfg.gatewayFacade,
		GatewayAuthMode:           cfg.gatewayAuthMode,
		GatewayAuthAudience:       gatewayAuthAudienceSummary(cfg.gatewayAuthMode, cfg.gatewayAuthAudience),
		StartedAt:                 startedAt,
		LatenciesMS:               map[string]float64{},
	}
	s.GitDirty = strings.TrimSpace(s.GitStatusShort) != ""
	defer func() {
		s.FinishedAt = time.Now().UTC()
		s.Capacity = buildCapacitySummary(s)
		_ = writeSummary(cfg.resultDir, s)
	}()

	var pool *pgxpool.Pool
	if cfg.pgDSN != "" {
		var err error
		pool, err = pgxpool.New(context.Background(), cfg.pgDSN)
		if err != nil {
			s.Error = err.Error()
			return fmt.Errorf("open postgres: %w", err)
		}
		defer pool.Close()
		if cfg.cleanup {
			if err := cleanupTenant(context.Background(), pool, cfg.tenantID); err != nil {
				s.Error = err.Error()
				return err
			}
		}
	}
	if pool != nil {
		defer pool.Close()
	}

	dialOption, err := grpctls.DialOption(cfg.tls, "contacts-tls")
	if err != nil {
		s.Error = err.Error()
		return fmt.Errorf("configure contacts-service TLS: %w", err)
	}
	conn, err := grpc.NewClient(cfg.target, dialOption)
	if err != nil {
		s.Error = err.Error()
		return fmt.Errorf("dial contacts service: %w", err)
	}
	defer conn.Close()
	var client contactsClient = contactsv1.NewContactsServiceClient(conn)
	if cfg.gatewayFacade {
		client = gatewayv1.NewGatewayServiceClient(conn)
	}
	if cfg.duration > 0 {
		if err := runCapacity(cfg, client, pool, &s); err != nil {
			s.Error = err.Error()
			return err
		}
		s.Success = true
		return nil
	}

	requestIDSuffix := time.Now().UTC().Format("20060102150405")
	sendResult, elapsed, err := sendContactRequest(cfg, client, requestIDSuffix)
	s.LatenciesMS["send_contact_request"] = elapsed
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.SendContactRequest = sendResult

	pendingBefore, elapsed, err := listContactRequests(cfg, client, cfg.receiverUserID, contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_INCOMING, contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_PENDING)
	s.LatenciesMS["list_receiver_pending_requests_before_respond"] = elapsed
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.ReceiverPendingBeforeRespond = pendingBefore

	if cfg.scenario == "cancel" {
		cancelResult, elapsed, err := cancelContactRequest(cfg, client, sendResult.RequestID, requestIDSuffix)
		s.LatenciesMS["cancel_contact_request"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.CancelContactRequest = cancelResult
		pendingAfterCancel, elapsed, err := listContactRequests(cfg, client, cfg.receiverUserID, contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_INCOMING, contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_PENDING)
		s.LatenciesMS["list_receiver_pending_requests_after_cancel"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.ReceiverPendingAfterCancel = pendingAfterCancel
		canceledOutgoing, elapsed, err := listContactRequests(cfg, client, cfg.senderUserID, contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_OUTGOING, contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_CANCELED)
		s.LatenciesMS["list_sender_canceled_requests_after_cancel"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.SenderCanceledAfterCancel = canceledOutgoing
	} else {
		respondResult, elapsed, err := respondContactRequest(cfg, client, sendResult.RequestID, requestIDSuffix)
		s.LatenciesMS["respond_contact_request"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.RespondContactRequest = respondResult
		pendingAfter, elapsed, err := listContactRequests(cfg, client, cfg.receiverUserID, contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_INCOMING, contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_PENDING)
		s.LatenciesMS["list_receiver_pending_requests_after_respond"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.ReceiverPendingAfterRespond = pendingAfter
		terminalAfter, elapsed, err := listContactRequests(cfg, client, cfg.receiverUserID, contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_INCOMING, terminalRequestStatusForScenario(cfg.scenario))
		s.LatenciesMS["list_receiver_terminal_requests_after_respond"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.ReceiverTerminalAfterRespond = terminalAfter
	}

	switch cfg.scenario {
	case "delete":
		deleteResult, elapsed, err := deleteContact(cfg, client, requestIDSuffix)
		s.LatenciesMS["delete_contact"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.DeleteContact = deleteResult
	case "block":
		blockResult, elapsed, err := blockContact(cfg, client, requestIDSuffix)
		s.LatenciesMS["block_contact"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.BlockContact = blockResult
	case "unblock":
		blockResult, elapsed, err := blockContact(cfg, client, requestIDSuffix)
		s.LatenciesMS["block_contact"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.BlockContact = blockResult
		unblockResult, elapsed, err := unblockContact(cfg, client, requestIDSuffix)
		s.LatenciesMS["unblock_contact"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.UnblockContact = unblockResult
	case "remark":
		remarkResult, elapsed, err := updateContactRemark(cfg, client, requestIDSuffix)
		s.LatenciesMS["update_contact_remark"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.UpdateContactRemark = remarkResult
	case "readd":
		deleteResult, elapsed, err := deleteContact(cfg, client, requestIDSuffix)
		s.LatenciesMS["delete_contact"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.DeleteContact = deleteResult

		secondSuffix := "second-" + requestIDSuffix
		secondSendResult, elapsed, err := sendContactRequest(cfg, client, secondSuffix)
		s.LatenciesMS["second_send_contact_request"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.SecondSendContactRequest = secondSendResult

		secondRespondResult, elapsed, err := respondContactRequest(cfg, client, secondSendResult.RequestID, secondSuffix)
		s.LatenciesMS["second_respond_contact_request"] = elapsed
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.SecondRespondContactRequest = secondRespondResult
	}

	senderList, elapsed, err := listContacts(cfg, client, cfg.senderUserID)
	s.LatenciesMS["list_sender_contacts"] = elapsed
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.SenderList = senderList
	receiverList, elapsed, err := listContacts(cfg, client, cfg.receiverUserID)
	s.LatenciesMS["list_receiver_contacts"] = elapsed
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.ReceiverList = receiverList

	senderState, elapsed, err := getContactState(cfg, client, cfg.senderUserID, cfg.receiverUserID)
	s.LatenciesMS["get_sender_state"] = elapsed
	if err != nil && cfg.scenario == "accept" {
		s.Error = err.Error()
		return err
	}
	s.SenderState = senderState
	receiverState, elapsed, err := getContactState(cfg, client, cfg.receiverUserID, cfg.senderUserID)
	s.LatenciesMS["get_receiver_state"] = elapsed
	if err != nil && cfg.scenario == "accept" {
		s.Error = err.Error()
		return err
	}
	s.ReceiverState = receiverState

	if cfg.pgDSN != "" {
		pool, err := pgxpool.New(context.Background(), cfg.pgDSN)
		if err != nil {
			s.Error = err.Error()
			return fmt.Errorf("open postgres for stats: %w", err)
		}
		defer pool.Close()
		wantEvents := expectedEventCount(cfg.scenario)
		outbox, err := waitOutboxPublished(context.Background(), pool, cfg, int64(wantEvents))
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.ContactsOutbox = outbox
	}

	events, err := readContactEvents(context.Background(), cfg, expectedEventCount(cfg.scenario))
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.ContactKafkaEvents = events

	if err := validateSummary(s); err != nil {
		s.Error = err.Error()
		return err
	}
	s.Success = true
	return nil
}

func sendContactRequest(cfg config, client contactsClient, suffix string) (sendSummary, float64, error) {
	ctx, cancel, auth := requestContext(cfg, cfg.senderUserID, cfg.senderDeviceID, "contact-send-"+suffix, "trace-contact-"+suffix)
	defer cancel()
	begin := time.Now()
	resp, err := client.SendContactRequest(ctx, &contactsv1.SendContactRequestRequest{
		AuthContext:    auth,
		TargetUserId:   cfg.receiverUserID,
		IdempotencyKey: "send-" + suffix,
		Message:        "hello from contacts smoke",
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return sendSummary{}, elapsed, fmt.Errorf("send contact request: %w", err)
	}
	return sendSummary{
		RequestID:        resp.GetRequestId(),
		Status:           resp.GetStatus().String(),
		IdempotentReplay: resp.GetIdempotentReplay(),
	}, elapsed, nil
}

func runCapacity(cfg config, client contactsClient, pool *pgxpool.Pool, s *summary) error {
	if cfg.scenario != "accept" {
		return fmt.Errorf("--duration capacity mode currently supports accept scenario only, got %q", cfg.scenario)
	}
	if pool == nil {
		return fmt.Errorf("--pg-dsn is required for contacts capacity mode")
	}
	if len(cfg.kafkaBrokers) == 0 || cfg.contactTopic == "" {
		return fmt.Errorf("--kafka-brokers and --contact-topic are required for contacts capacity mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.duration)
	defer cancel()

	var mu sync.Mutex
	var firstErr error
	successPairs := 0
	errorCount := 0
	latencySamples := make([]float64, 0, cfg.vus*64)
	runSuffix := s.StartedAt.Format("20060102150405")

	recordError := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		errorCount++
		if firstErr == nil {
			firstErr = err
			cancel()
		}
	}
	recordSuccess := func(sendMS, respondMS float64) {
		mu.Lock()
		defer mu.Unlock()
		successPairs++
		latencySamples = append(latencySamples, sendMS, respondMS)
	}

	var wg sync.WaitGroup
	for vu := 0; vu < cfg.vus; vu++ {
		vu := vu
		wg.Add(1)
		go func() {
			defer wg.Done()
			iteration := 0
			for ctx.Err() == nil {
				localCfg := cfg
				suffix := fmt.Sprintf("cap-%s-vu%02d-%06d", runSuffix, vu, iteration)
				localCfg.senderUserID = fmt.Sprintf("%s-vu%02d-%06d-s", cfg.senderUserID, vu, iteration)
				localCfg.receiverUserID = fmt.Sprintf("%s-vu%02d-%06d-r", cfg.receiverUserID, vu, iteration)
				localCfg.senderDeviceID = fmt.Sprintf("%s-vu%02d-s", cfg.senderDeviceID, vu)
				localCfg.receiverDeviceID = fmt.Sprintf("%s-vu%02d-r", cfg.receiverDeviceID, vu)

				sendResult, sendMS, err := sendContactRequest(localCfg, client, suffix)
				if err != nil {
					recordError(err)
					return
				}
				_, respondMS, err := respondContactRequest(localCfg, client, sendResult.RequestID, suffix)
				if err != nil {
					recordError(err)
					return
				}
				recordSuccess(sendMS, respondMS)
				iteration++
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	s.CapacityOperationCount = successPairs * 2
	s.CapacityErrorCount = errorCount
	s.CapacityLatencySamplesMS = append([]float64(nil), latencySamples...)
	err := firstErr
	mu.Unlock()
	if err != nil {
		return err
	}
	if successPairs == 0 {
		return fmt.Errorf("contacts capacity run completed with no successful contact request pairs")
	}

	wantEvents := successPairs * expectedEventCount(cfg.scenario)
	outbox, err := waitOutboxPublished(context.Background(), pool, cfg, int64(wantEvents))
	if err != nil {
		return err
	}
	s.ContactsOutbox = outbox
	eventCount, err := readContactEventCount(context.Background(), cfg, wantEvents)
	if err != nil {
		return err
	}
	s.CapacityContactEventCount = eventCount
	return nil
}

func respondContactRequest(cfg config, client contactsClient, requestID string, suffix string) (respondSummary, float64, error) {
	ctx, cancel, auth := requestContext(cfg, cfg.receiverUserID, cfg.receiverDeviceID, "contact-respond-"+suffix, "trace-contact-"+suffix)
	defer cancel()
	begin := time.Now()
	decision := contactsv1.ContactDecision_CONTACT_DECISION_ACCEPT
	if cfg.scenario == "decline" {
		decision = contactsv1.ContactDecision_CONTACT_DECISION_DECLINE
	}
	resp, err := client.RespondContactRequest(ctx, &contactsv1.RespondContactRequestRequest{
		AuthContext:    auth,
		RequestId:      requestID,
		Decision:       decision,
		IdempotencyKey: "respond-" + suffix,
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return respondSummary{}, elapsed, fmt.Errorf("respond contact request: %w", err)
	}
	return respondSummary{
		RequestID:        resp.GetRequestId(),
		Status:           resp.GetStatus().String(),
		IdempotentReplay: resp.GetIdempotentReplay(),
	}, elapsed, nil
}

func cancelContactRequest(cfg config, client contactsClient, requestID string, suffix string) (respondSummary, float64, error) {
	ctx, cancel, auth := requestContext(cfg, cfg.senderUserID, cfg.senderDeviceID, "contact-cancel-"+suffix, "trace-contact-"+suffix)
	defer cancel()
	begin := time.Now()
	resp, err := client.CancelContactRequest(ctx, &contactsv1.CancelContactRequestRequest{
		AuthContext:    auth,
		RequestId:      requestID,
		IdempotencyKey: "cancel-" + suffix,
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return respondSummary{}, elapsed, fmt.Errorf("cancel contact request: %w", err)
	}
	return respondSummary{
		RequestID:        resp.GetRequestId(),
		Status:           resp.GetStatus().String(),
		IdempotentReplay: resp.GetIdempotentReplay(),
	}, elapsed, nil
}

func deleteContact(cfg config, client contactsClient, suffix string) (edgeActionSummary, float64, error) {
	ctx, cancel, auth := requestContext(cfg, cfg.senderUserID, cfg.senderDeviceID, "contact-delete-"+suffix, "trace-contact-"+suffix)
	defer cancel()
	begin := time.Now()
	resp, err := client.DeleteContact(ctx, &contactsv1.DeleteContactRequest{
		AuthContext:    auth,
		ContactUserId:  cfg.receiverUserID,
		IdempotencyKey: "delete-" + suffix,
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return edgeActionSummary{}, elapsed, fmt.Errorf("delete contact: %w", err)
	}
	return edgeActionSummary{
		Status:           resp.GetStatus().String(),
		Version:          resp.GetVersion(),
		IdempotentReplay: resp.GetIdempotentReplay(),
	}, elapsed, nil
}

func blockContact(cfg config, client contactsClient, suffix string) (edgeActionSummary, float64, error) {
	ctx, cancel, auth := requestContext(cfg, cfg.senderUserID, cfg.senderDeviceID, "contact-block-"+suffix, "trace-contact-"+suffix)
	defer cancel()
	begin := time.Now()
	resp, err := client.BlockContact(ctx, &contactsv1.BlockContactRequest{
		AuthContext:    auth,
		ContactUserId:  cfg.receiverUserID,
		IdempotencyKey: "block-" + suffix,
		Reason:         "contacts smoke block",
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return edgeActionSummary{}, elapsed, fmt.Errorf("block contact: %w", err)
	}
	return edgeActionSummary{
		Status:           resp.GetStatus().String(),
		Version:          resp.GetVersion(),
		IdempotentReplay: resp.GetIdempotentReplay(),
	}, elapsed, nil
}

func unblockContact(cfg config, client contactsClient, suffix string) (edgeActionSummary, float64, error) {
	ctx, cancel, auth := requestContext(cfg, cfg.senderUserID, cfg.senderDeviceID, "contact-unblock-"+suffix, "trace-contact-"+suffix)
	defer cancel()
	begin := time.Now()
	resp, err := client.UnblockContact(ctx, &contactsv1.UnblockContactRequest{
		AuthContext:    auth,
		ContactUserId:  cfg.receiverUserID,
		IdempotencyKey: "unblock-" + suffix,
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return edgeActionSummary{}, elapsed, fmt.Errorf("unblock contact: %w", err)
	}
	return edgeActionSummary{
		Status:           resp.GetStatus().String(),
		Version:          resp.GetVersion(),
		IdempotentReplay: resp.GetIdempotentReplay(),
	}, elapsed, nil
}

func updateContactRemark(cfg config, client contactsClient, suffix string) (edgeActionSummary, float64, error) {
	ctx, cancel, auth := requestContext(cfg, cfg.senderUserID, cfg.senderDeviceID, "contact-remark-"+suffix, "trace-contact-"+suffix)
	defer cancel()
	begin := time.Now()
	resp, err := client.UpdateContactRemark(ctx, &contactsv1.UpdateContactRemarkRequest{
		AuthContext:    auth,
		ContactUserId:  cfg.receiverUserID,
		Remark:         "smoke remark",
		IdempotencyKey: "remark-" + suffix,
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return edgeActionSummary{}, elapsed, fmt.Errorf("update contact remark: %w", err)
	}
	return edgeActionSummary{
		Status:           resp.GetStatus().String(),
		Version:          resp.GetVersion(),
		Remark:           resp.GetRemark(),
		IdempotentReplay: resp.GetIdempotentReplay(),
	}, elapsed, nil
}

func listContacts(cfg config, client contactsClient, userID string) (listSummary, float64, error) {
	ctx, cancel, auth := requestContext(cfg, userID, deviceIDForUser(cfg, userID), "contact-list-"+userID, "trace-contact-list")
	defer cancel()
	begin := time.Now()
	resp, err := client.ListContacts(ctx, &contactsv1.ListContactsRequest{
		AuthContext: auth,
		PageSize:    10,
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return listSummary{}, elapsed, fmt.Errorf("list contacts for %s: %w", userID, err)
	}
	result := listSummary{
		OwnerUserID:    resp.GetOwnerUserId(),
		ContactCount:   len(resp.GetContacts()),
		ContactUserIDs: make([]string, 0, len(resp.GetContacts())),
	}
	for _, item := range resp.GetContacts() {
		result.ContactUserIDs = append(result.ContactUserIDs, item.GetContactUserId())
	}
	return result, elapsed, nil
}

func listContactRequests(
	cfg config,
	client contactsClient,
	userID string,
	direction contactsv1.ContactRequestListDirection,
	statusValue contactsv1.ContactRequestStatus,
) (requestListSummary, float64, error) {
	ctx, cancel, auth := requestContext(cfg, userID, deviceIDForUser(cfg, userID), "contact-request-list-"+userID, "trace-contact-request-list")
	defer cancel()
	begin := time.Now()
	resp, err := client.ListContactRequests(ctx, &contactsv1.ListContactRequestsRequest{
		AuthContext: auth,
		Direction:   direction,
		Status:      statusValue,
		PageSize:    10,
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return requestListSummary{}, elapsed, fmt.Errorf("list contact requests for %s: %w", userID, err)
	}
	result := requestListSummary{
		UserID:          resp.GetUserId(),
		Direction:       resp.GetDirection().String(),
		Status:          resp.GetStatus().String(),
		RequestCount:    len(resp.GetRequests()),
		RequestIDs:      make([]string, 0, len(resp.GetRequests())),
		SenderUserIDs:   make([]string, 0, len(resp.GetRequests())),
		ReceiverUserIDs: make([]string, 0, len(resp.GetRequests())),
	}
	for _, item := range resp.GetRequests() {
		result.RequestIDs = append(result.RequestIDs, item.GetRequestId())
		result.SenderUserIDs = append(result.SenderUserIDs, item.GetSenderUserId())
		result.ReceiverUserIDs = append(result.ReceiverUserIDs, item.GetReceiverUserId())
	}
	return result, elapsed, nil
}

func getContactState(cfg config, client contactsClient, userID string, otherUserID string) (stateSummary, float64, error) {
	ctx, cancel, auth := requestContext(cfg, userID, deviceIDForUser(cfg, userID), "contact-state-"+userID, "trace-contact-state")
	defer cancel()
	begin := time.Now()
	resp, err := client.GetContactState(ctx, &contactsv1.GetContactStateRequest{
		AuthContext: auth,
		OtherUserId: otherUserID,
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return stateSummary{
			OwnerUserID:   userID,
			ContactUserID: otherUserID,
			Error:         status.Code(err).String(),
		}, elapsed, fmt.Errorf("get contact state %s -> %s: %w", userID, otherUserID, err)
	}
	return stateSummary{
		OwnerUserID:     resp.GetOwnerUserId(),
		ContactUserID:   resp.GetContactUserId(),
		Status:          resp.GetStatus().String(),
		SourceRequestID: resp.GetSourceRequestId(),
		Version:         resp.GetVersion(),
		Remark:          resp.GetRemark(),
	}, elapsed, nil
}

func waitOutboxPublished(ctx context.Context, pool *pgxpool.Pool, cfg config, wantPublished int64) (outboxStats, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		stats, err := queryOutboxStats(ctx, pool, cfg.tenantID)
		if err != nil {
			return outboxStats{}, err
		}
		if stats.Published >= wantPublished && stats.Pending == 0 && stats.DLQ == 0 {
			return stats, nil
		}
		if time.Now().After(deadline) {
			return stats, fmt.Errorf("contacts outbox did not drain: %+v", stats)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func queryOutboxStats(ctx context.Context, pool *pgxpool.Pool, tenantID string) (outboxStats, error) {
	var stats outboxStats
	err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING'),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ')
FROM contacts_outbox
WHERE tenant_id = $1
`, tenantID).Scan(&stats.Total, &stats.Pending, &stats.Published, &stats.DLQ)
	return stats, err
}

func readContactEvents(ctx context.Context, cfg config, want int) ([]contactKafkaEvent, error) {
	if len(cfg.kafkaBrokers) == 0 || cfg.contactTopic == "" {
		return nil, nil
	}
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:   cfg.kafkaBrokers,
		Topic:     cfg.contactTopic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  10e6,
	})
	defer reader.Close()
	if err := reader.SetOffset(kafkago.FirstOffset); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(cfg.waitTimeout)
	events := make([]contactKafkaEvent, 0, want)
	seen := map[string]bool{}
	for len(events) < want && time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(ctx, cfg.pollInterval)
		message, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			continue
		}
		var event contacteventsv1.ContactEvent
		if err := proto.Unmarshal(message.Value, &event); err != nil {
			continue
		}
		if event.GetTenantId() != cfg.tenantID || seen[event.GetEventId()] {
			continue
		}
		seen[event.GetEventId()] = true
		events = append(events, summarizeContactEvent(&event))
	}
	if len(events) < want {
		return events, fmt.Errorf("expected %d contact Kafka events, got %d", want, len(events))
	}
	return events, nil
}

func readContactEventCount(ctx context.Context, cfg config, want int) (int, error) {
	if want <= 0 {
		return 0, nil
	}
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:   cfg.kafkaBrokers,
		Topic:     cfg.contactTopic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  10e6,
	})
	defer reader.Close()
	if err := reader.SetOffset(kafkago.FirstOffset); err != nil {
		return 0, err
	}
	deadline := time.Now().Add(cfg.waitTimeout)
	seen := map[string]bool{}
	for len(seen) < want && time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(ctx, cfg.pollInterval)
		message, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			continue
		}
		var event contacteventsv1.ContactEvent
		if err := proto.Unmarshal(message.Value, &event); err != nil {
			continue
		}
		if event.GetTenantId() != cfg.tenantID || seen[event.GetEventId()] {
			continue
		}
		seen[event.GetEventId()] = true
	}
	if len(seen) < want {
		return len(seen), fmt.Errorf("expected %d contact Kafka events, got %d", want, len(seen))
	}
	return len(seen), nil
}

func summarizeContactEvent(event *contacteventsv1.ContactEvent) contactKafkaEvent {
	result := contactKafkaEvent{
		EventID:          event.GetEventId(),
		EventType:        event.GetEventType(),
		AggregateVersion: event.GetAggregateVersion(),
		PartitionKey:     event.GetPartitionKey(),
	}
	switch payload := event.GetPayload().(type) {
	case *contacteventsv1.ContactEvent_RequestCreated:
		result.RequestID = payload.RequestCreated.GetRequestId()
		result.SenderUserID = payload.RequestCreated.GetSenderUserId()
		result.ReceiverUserID = payload.RequestCreated.GetReceiverUserId()
		result.Status = payload.RequestCreated.GetStatus()
	case *contacteventsv1.ContactEvent_RequestAccepted:
		result.RequestID = payload.RequestAccepted.GetRequestId()
		result.SenderUserID = payload.RequestAccepted.GetSenderUserId()
		result.ReceiverUserID = payload.RequestAccepted.GetReceiverUserId()
		result.Status = payload.RequestAccepted.GetStatus()
	case *contacteventsv1.ContactEvent_RequestDeclined:
		result.RequestID = payload.RequestDeclined.GetRequestId()
		result.SenderUserID = payload.RequestDeclined.GetSenderUserId()
		result.ReceiverUserID = payload.RequestDeclined.GetReceiverUserId()
		result.Status = payload.RequestDeclined.GetStatus()
	case *contacteventsv1.ContactEvent_RequestCanceled:
		result.RequestID = payload.RequestCanceled.GetRequestId()
		result.SenderUserID = payload.RequestCanceled.GetSenderUserId()
		result.ReceiverUserID = payload.RequestCanceled.GetReceiverUserId()
		result.Status = payload.RequestCanceled.GetStatus()
	case *contacteventsv1.ContactEvent_EdgeDeleted:
		result.OwnerUserID = payload.EdgeDeleted.GetOwnerUserId()
		result.ContactUserID = payload.EdgeDeleted.GetContactUserId()
		result.Status = payload.EdgeDeleted.GetStatus()
	case *contacteventsv1.ContactEvent_EdgeBlocked:
		result.OwnerUserID = payload.EdgeBlocked.GetOwnerUserId()
		result.ContactUserID = payload.EdgeBlocked.GetContactUserId()
		result.Status = payload.EdgeBlocked.GetStatus()
		result.Reason = payload.EdgeBlocked.GetReason()
	case *contacteventsv1.ContactEvent_EdgeUnblocked:
		result.OwnerUserID = payload.EdgeUnblocked.GetOwnerUserId()
		result.ContactUserID = payload.EdgeUnblocked.GetContactUserId()
		result.Status = payload.EdgeUnblocked.GetStatus()
	case *contacteventsv1.ContactEvent_EdgeRemarkUpdated:
		result.OwnerUserID = payload.EdgeRemarkUpdated.GetOwnerUserId()
		result.ContactUserID = payload.EdgeRemarkUpdated.GetContactUserId()
		result.Status = payload.EdgeRemarkUpdated.GetStatus()
		result.Remark = payload.EdgeRemarkUpdated.GetRemark()
	}
	return result
}

func validateSummary(s summary) error {
	switch s.Scenario {
	case "accept":
		return validateAcceptSummary(s)
	case "decline":
		return validateDeclineSummary(s)
	case "cancel":
		return validateCancelSummary(s)
	case "delete":
		return validateDeleteSummary(s)
	case "block":
		return validateBlockSummary(s)
	case "unblock":
		return validateUnblockSummary(s)
	case "remark":
		return validateRemarkSummary(s)
	case "readd":
		return validateReaddSummary(s)
	default:
		return fmt.Errorf("unsupported scenario %q", s.Scenario)
	}
}

func validateDeleteSummary(s summary) error {
	if err := validateAcceptPrefix(s); err != nil {
		return err
	}
	if s.DeleteContact.Status != "CONTACT_EDGE_STATUS_DELETED" {
		return fmt.Errorf("delete status=%s, want DELETED", s.DeleteContact.Status)
	}
	if s.SenderList.ContactCount != 0 {
		return fmt.Errorf("sender list should hide deleted edge: %+v", s.SenderList)
	}
	if !contains(s.ReceiverList.ContactUserIDs, s.SenderUserID) {
		return fmt.Errorf("receiver list missing sender after sender delete: %+v", s.ReceiverList)
	}
	if s.SenderState.Status != "CONTACT_EDGE_STATUS_DELETED" || s.ReceiverState.Status != "CONTACT_EDGE_STATUS_ACTIVE" {
		return fmt.Errorf("unexpected delete states: sender=%+v receiver=%+v", s.SenderState, s.ReceiverState)
	}
	return validateOutboxAndEvents(s, "contact.edge.deleted.v1", expectedEventCount(s.Scenario))
}

func validateCancelSummary(s summary) error {
	if s.SendContactRequest.Status != "CONTACT_REQUEST_STATUS_PENDING" {
		return fmt.Errorf("send status=%s, want PENDING", s.SendContactRequest.Status)
	}
	if s.CancelContactRequest.Status != "CONTACT_REQUEST_STATUS_CANCELED" {
		return fmt.Errorf("cancel status=%s, want CANCELED", s.CancelContactRequest.Status)
	}
	if s.ReceiverPendingBeforeRespond.RequestCount != 1 ||
		!contains(s.ReceiverPendingBeforeRespond.RequestIDs, s.SendContactRequest.RequestID) {
		return fmt.Errorf("pending contact request before cancel not visible: %+v", s.ReceiverPendingBeforeRespond)
	}
	if s.ReceiverPendingAfterCancel.RequestCount != 0 {
		return fmt.Errorf("receiver pending request should disappear after cancel: %+v", s.ReceiverPendingAfterCancel)
	}
	if s.SenderCanceledAfterCancel.Status != "CONTACT_REQUEST_STATUS_CANCELED" ||
		s.SenderCanceledAfterCancel.RequestCount != 1 ||
		!contains(s.SenderCanceledAfterCancel.RequestIDs, s.SendContactRequest.RequestID) {
		return fmt.Errorf("sender canceled request after cancel not visible: %+v", s.SenderCanceledAfterCancel)
	}
	if s.SenderList.ContactCount != 0 || s.ReceiverList.ContactCount != 0 {
		return fmt.Errorf("cancel should not create contacts: sender=%+v receiver=%+v", s.SenderList, s.ReceiverList)
	}
	if s.SenderState.Error != codes.NotFound.String() || s.ReceiverState.Error != codes.NotFound.String() {
		return fmt.Errorf("cancel should leave no contact state: sender=%+v receiver=%+v", s.SenderState, s.ReceiverState)
	}
	return validateOutboxAndEvents(s, "contact.request.canceled.v1", expectedEventCount(s.Scenario))
}

func validateBlockSummary(s summary) error {
	if err := validateAcceptPrefix(s); err != nil {
		return err
	}
	if s.BlockContact.Status != "CONTACT_EDGE_STATUS_BLOCKED" {
		return fmt.Errorf("block status=%s, want BLOCKED", s.BlockContact.Status)
	}
	if s.SenderList.ContactCount != 0 {
		return fmt.Errorf("sender list should hide blocked edge: %+v", s.SenderList)
	}
	if !contains(s.ReceiverList.ContactUserIDs, s.SenderUserID) {
		return fmt.Errorf("receiver list missing sender after sender block: %+v", s.ReceiverList)
	}
	if s.SenderState.Status != "CONTACT_EDGE_STATUS_BLOCKED" || s.ReceiverState.Status != "CONTACT_EDGE_STATUS_ACTIVE" {
		return fmt.Errorf("unexpected block states: sender=%+v receiver=%+v", s.SenderState, s.ReceiverState)
	}
	return validateOutboxAndEvents(s, "contact.edge.blocked.v1", expectedEventCount(s.Scenario))
}

func validateUnblockSummary(s summary) error {
	if err := validateAcceptPrefix(s); err != nil {
		return err
	}
	if s.BlockContact.Status != "CONTACT_EDGE_STATUS_BLOCKED" {
		return fmt.Errorf("block status=%s, want BLOCKED", s.BlockContact.Status)
	}
	if s.UnblockContact.Status != "CONTACT_EDGE_STATUS_ACTIVE" {
		return fmt.Errorf("unblock status=%s, want ACTIVE", s.UnblockContact.Status)
	}
	if !contains(s.SenderList.ContactUserIDs, s.ReceiverUserID) || !contains(s.ReceiverList.ContactUserIDs, s.SenderUserID) {
		return fmt.Errorf("unblocked contact should be visible to both sides: sender=%+v receiver=%+v", s.SenderList, s.ReceiverList)
	}
	if s.SenderState.Status != "CONTACT_EDGE_STATUS_ACTIVE" || s.ReceiverState.Status != "CONTACT_EDGE_STATUS_ACTIVE" {
		return fmt.Errorf("unexpected unblock states: sender=%+v receiver=%+v", s.SenderState, s.ReceiverState)
	}
	if err := validateOutboxAndEvents(s, "contact.edge.blocked.v1", expectedEventCount(s.Scenario)); err != nil {
		return err
	}
	return validateOutboxAndEvents(s, "contact.edge.unblocked.v1", expectedEventCount(s.Scenario))
}

func validateRemarkSummary(s summary) error {
	if err := validateAcceptPrefix(s); err != nil {
		return err
	}
	if s.UpdateContactRemark.Status != "CONTACT_EDGE_STATUS_ACTIVE" || s.UpdateContactRemark.Remark != "smoke remark" {
		return fmt.Errorf("unexpected remark result: %+v", s.UpdateContactRemark)
	}
	if s.SenderState.Remark != "smoke remark" {
		return fmt.Errorf("sender state missing remark: %+v", s.SenderState)
	}
	return validateOutboxAndEvents(s, "contact.edge.remark_updated.v1", expectedEventCount(s.Scenario))
}

func validateReaddSummary(s summary) error {
	if err := validateAcceptPrefix(s); err != nil {
		return err
	}
	if s.DeleteContact.Status != "CONTACT_EDGE_STATUS_DELETED" {
		return fmt.Errorf("delete status=%s, want DELETED", s.DeleteContact.Status)
	}
	if s.SecondSendContactRequest.Status != "CONTACT_REQUEST_STATUS_PENDING" {
		return fmt.Errorf("second send status=%s, want PENDING", s.SecondSendContactRequest.Status)
	}
	if s.SecondRespondContactRequest.Status != "CONTACT_REQUEST_STATUS_ACCEPTED" {
		return fmt.Errorf("second respond status=%s, want ACCEPTED", s.SecondRespondContactRequest.Status)
	}
	if !contains(s.SenderList.ContactUserIDs, s.ReceiverUserID) || !contains(s.ReceiverList.ContactUserIDs, s.SenderUserID) {
		return fmt.Errorf("re-added contact should be visible to both sides: sender=%+v receiver=%+v", s.SenderList, s.ReceiverList)
	}
	if s.SenderState.Status != "CONTACT_EDGE_STATUS_ACTIVE" || s.ReceiverState.Status != "CONTACT_EDGE_STATUS_ACTIVE" {
		return fmt.Errorf("unexpected readd states: sender=%+v receiver=%+v", s.SenderState, s.ReceiverState)
	}
	if err := validateOutboxAndEvents(s, "contact.edge.deleted.v1", expectedEventCount(s.Scenario)); err != nil {
		return err
	}
	if len(s.ContactKafkaEvents) > 0 {
		counts := map[string]int{}
		versions := make([]int64, 0, len(s.ContactKafkaEvents))
		for _, event := range s.ContactKafkaEvents {
			counts[event.EventType]++
			versions = append(versions, event.AggregateVersion)
		}
		if counts["contact.request.created.v1"] != 2 || counts["contact.request.accepted.v1"] != 2 || counts["contact.edge.deleted.v1"] != 1 {
			return fmt.Errorf("unexpected readd event counts: %+v", counts)
		}
		wantVersions := []int64{1, 2, 3, 4, 5}
		for index, want := range wantVersions {
			if index >= len(versions) || versions[index] != want {
				return fmt.Errorf("unexpected readd aggregate versions: got %v want %v", versions, wantVersions)
			}
		}
	}
	return nil
}

func validateAcceptPrefix(s summary) error {
	if s.SendContactRequest.Status != "CONTACT_REQUEST_STATUS_PENDING" {
		return fmt.Errorf("send status=%s, want PENDING", s.SendContactRequest.Status)
	}
	if s.RespondContactRequest.Status != "CONTACT_REQUEST_STATUS_ACCEPTED" {
		return fmt.Errorf("respond status=%s, want ACCEPTED", s.RespondContactRequest.Status)
	}
	if err := validateContactRequestLists(s, "CONTACT_REQUEST_STATUS_ACCEPTED"); err != nil {
		return err
	}
	return nil
}

func validateAcceptSummary(s summary) error {
	if s.SendContactRequest.Status != "CONTACT_REQUEST_STATUS_PENDING" {
		return fmt.Errorf("send status=%s, want PENDING", s.SendContactRequest.Status)
	}
	if s.RespondContactRequest.Status != "CONTACT_REQUEST_STATUS_ACCEPTED" {
		return fmt.Errorf("respond status=%s, want ACCEPTED", s.RespondContactRequest.Status)
	}
	if err := validateContactRequestLists(s, "CONTACT_REQUEST_STATUS_ACCEPTED"); err != nil {
		return err
	}
	if !contains(s.SenderList.ContactUserIDs, s.ReceiverUserID) {
		return fmt.Errorf("sender list missing receiver: %+v", s.SenderList)
	}
	if !contains(s.ReceiverList.ContactUserIDs, s.SenderUserID) {
		return fmt.Errorf("receiver list missing sender: %+v", s.ReceiverList)
	}
	if s.SenderState.Status != "CONTACT_EDGE_STATUS_ACTIVE" || s.ReceiverState.Status != "CONTACT_EDGE_STATUS_ACTIVE" {
		return fmt.Errorf("contact state not active: sender=%s receiver=%s", s.SenderState.Status, s.ReceiverState.Status)
	}
	if s.ContactsOutbox.Total > 0 && (s.ContactsOutbox.Pending != 0 || s.ContactsOutbox.DLQ != 0 || s.ContactsOutbox.Published < int64(expectedEventCount(s.Scenario))) {
		return fmt.Errorf("unexpected outbox stats: %+v", s.ContactsOutbox)
	}
	eventTypes := map[string]bool{}
	for _, event := range s.ContactKafkaEvents {
		eventTypes[event.EventType] = true
	}
	if len(s.ContactKafkaEvents) > 0 && (!eventTypes["contact.request.created.v1"] || !eventTypes["contact.request.accepted.v1"]) {
		return fmt.Errorf("missing expected contact Kafka events: %+v", s.ContactKafkaEvents)
	}
	return nil
}

func validateDeclineSummary(s summary) error {
	if s.SendContactRequest.Status != "CONTACT_REQUEST_STATUS_PENDING" {
		return fmt.Errorf("send status=%s, want PENDING", s.SendContactRequest.Status)
	}
	if s.RespondContactRequest.Status != "CONTACT_REQUEST_STATUS_DECLINED" {
		return fmt.Errorf("respond status=%s, want DECLINED", s.RespondContactRequest.Status)
	}
	if err := validateContactRequestLists(s, "CONTACT_REQUEST_STATUS_DECLINED"); err != nil {
		return err
	}
	if s.SenderList.ContactCount != 0 || s.ReceiverList.ContactCount != 0 {
		return fmt.Errorf("decline should not create contacts: sender=%+v receiver=%+v", s.SenderList, s.ReceiverList)
	}
	if s.SenderState.Error != codes.NotFound.String() || s.ReceiverState.Error != codes.NotFound.String() {
		return fmt.Errorf("decline should leave no contact state: sender=%+v receiver=%+v", s.SenderState, s.ReceiverState)
	}
	if s.ContactsOutbox.Total > 0 && (s.ContactsOutbox.Pending != 0 || s.ContactsOutbox.DLQ != 0 || s.ContactsOutbox.Published < 2) {
		return fmt.Errorf("unexpected outbox stats: %+v", s.ContactsOutbox)
	}
	eventTypes := map[string]bool{}
	for _, event := range s.ContactKafkaEvents {
		eventTypes[event.EventType] = true
	}
	if len(s.ContactKafkaEvents) > 0 && (!eventTypes["contact.request.created.v1"] || !eventTypes["contact.request.declined.v1"]) {
		return fmt.Errorf("missing expected contact Kafka events: %+v", s.ContactKafkaEvents)
	}
	return nil
}

func validateContactRequestLists(s summary, terminalStatus string) error {
	if s.ReceiverPendingBeforeRespond.RequestCount != 1 ||
		!contains(s.ReceiverPendingBeforeRespond.RequestIDs, s.SendContactRequest.RequestID) ||
		!contains(s.ReceiverPendingBeforeRespond.SenderUserIDs, s.SenderUserID) ||
		!contains(s.ReceiverPendingBeforeRespond.ReceiverUserIDs, s.ReceiverUserID) {
		return fmt.Errorf("pending contact request before respond not visible: %+v", s.ReceiverPendingBeforeRespond)
	}
	if s.ReceiverPendingAfterRespond.RequestCount != 0 {
		return fmt.Errorf("pending contact request should disappear after respond: %+v", s.ReceiverPendingAfterRespond)
	}
	if s.ReceiverTerminalAfterRespond.Status != terminalStatus ||
		s.ReceiverTerminalAfterRespond.RequestCount != 1 ||
		!contains(s.ReceiverTerminalAfterRespond.RequestIDs, s.SendContactRequest.RequestID) {
		return fmt.Errorf("terminal contact request after respond not visible: %+v", s.ReceiverTerminalAfterRespond)
	}
	return nil
}

func validateOutboxAndEvents(s summary, requiredEventType string, wantEvents int) error {
	if s.ContactsOutbox.Total > 0 && (s.ContactsOutbox.Pending != 0 || s.ContactsOutbox.DLQ != 0 || s.ContactsOutbox.Published < int64(wantEvents)) {
		return fmt.Errorf("unexpected outbox stats: %+v", s.ContactsOutbox)
	}
	eventTypes := map[string]bool{}
	for _, event := range s.ContactKafkaEvents {
		eventTypes[event.EventType] = true
	}
	requiresAccepted := requiredEventType != "contact.request.declined.v1" && requiredEventType != "contact.request.canceled.v1"
	if len(s.ContactKafkaEvents) > 0 && (!eventTypes["contact.request.created.v1"] || (requiresAccepted && !eventTypes["contact.request.accepted.v1"]) || !eventTypes[requiredEventType]) {
		return fmt.Errorf("missing expected contact Kafka events: %+v", s.ContactKafkaEvents)
	}
	return nil
}

func terminalRequestStatusForScenario(scenario string) contactsv1.ContactRequestStatus {
	if scenario == "decline" {
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_DECLINED
	}
	if scenario == "cancel" {
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_CANCELED
	}
	return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_ACCEPTED
}

func expectedEventCount(scenario string) int {
	switch scenario {
	case "delete", "block", "remark":
		return 3
	case "unblock":
		return 4
	case "readd":
		return 5
	default:
		return 2
	}
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	statements := []string{
		`DELETE FROM contacts_outbox WHERE tenant_id = $1`,
		`DELETE FROM contact_command_idempotency WHERE tenant_id = $1`,
		`DELETE FROM contact_edges WHERE tenant_id = $1`,
		`DELETE FROM contact_requests WHERE tenant_id = $1`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, tenantID); err != nil {
			return err
		}
	}
	return nil
}

func writeSummary(resultDir string, s summary) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(resultDir, "contacts-summary.json"), raw, 0o644)
}

func buildCapacitySummary(s summary) *capacitySummary {
	duration := s.FinishedAt.Sub(s.StartedAt).Seconds()
	if duration <= 0 {
		return nil
	}
	operationCount := len(s.LatenciesMS)
	eventCount := len(s.ContactKafkaEvents)
	errorCount := 0
	vus := 0
	var latencyP95 float64
	var latencyP99 float64
	if s.CapacityMode {
		operationCount = s.CapacityOperationCount
		eventCount = s.CapacityContactEventCount
		errorCount = s.CapacityErrorCount
		vus = s.VUs
		latencyP95 = latencySliceQuantile(s.CapacityLatencySamplesMS, 0.95)
		latencyP99 = latencySliceQuantile(s.CapacityLatencySamplesMS, 0.99)
	} else {
		latencyP95 = latencyQuantile(s.LatenciesMS, 0.95)
		latencyP99 = latencyQuantile(s.LatenciesMS, 0.99)
	}
	return &capacitySummary{
		DurationSeconds:       duration,
		Scenario:              s.Scenario,
		OperationCount:        operationCount,
		ContactEventCount:     eventCount,
		ErrorCount:            errorCount,
		VUs:                   vus,
		ContactsOutboxTotal:   s.ContactsOutbox.Total,
		ContactsOutboxPending: s.ContactsOutbox.Pending,
		ContactsOutboxDLQ:     s.ContactsOutbox.DLQ,
		OperationsPerSecond:   ratePerSecond(operationCount, duration),
		EventsPerSecond:       ratePerSecond(eventCount, duration),
		LatencyP95MS:          latencyP95,
		LatencyP99MS:          latencyP99,
	}
}

func ratePerSecond(count int, durationSeconds float64) float64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	return float64(count) / durationSeconds
}

func latencyQuantile(values map[string]float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, 0, len(values))
	for _, value := range values {
		sorted = append(sorted, value)
	}
	return latencySortedQuantile(sorted, quantile)
}

func latencySliceQuantile(values []float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	return latencySortedQuantile(sorted, quantile)
}

func latencySortedQuantile(sorted []float64, quantile float64) float64 {
	sort.Float64s(sorted)
	index := int(math.Ceil(quantile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func gitOutput(args ...string) string {
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func elapsedMS(begin time.Time) float64 {
	return float64(time.Since(begin).Microseconds()) / 1000.0
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
