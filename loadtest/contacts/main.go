package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	gatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/gateway/v1"
	"github.com/qsyy0921/IM/internal/gatewayauth"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	contacteventsv1 "github.com/qsyy0921/IM/schemas/kafka/contacts/v1"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	metadataTenantID  = "x-nexusim-tenant-id"
	metadataUserID    = "x-nexusim-user-id"
	metadataDeviceID  = "x-nexusim-device-id"
	metadataSessionID = "x-nexusim-session-id"
	metadataTraceID   = "x-nexusim-trace-id"
	metadataRequestID = "x-nexusim-request-id"
	metadataToken     = "x-nexusim-gateway-token"
)

type config struct {
	target                string
	tls                   grpctls.Config
	resultDir             string
	pgDSN                 string
	kafkaBrokers          []string
	contactTopic          string
	requestTimeout        time.Duration
	waitTimeout           time.Duration
	pollInterval          time.Duration
	tenantID              string
	senderUserID          string
	receiverUserID        string
	senderDeviceID        string
	receiverDeviceID      string
	scenario              string
	cleanup               bool
	verifiedMetadata      bool
	gatewayFacade         bool
	gatewayAuthMode       string
	gatewayAuthHMACSecret string
	gatewayAuthAudience   string
	gatewayAuthTokenTTL   time.Duration
}

type summary struct {
	Commit                       string              `json:"commit"`
	CommitFull                   string              `json:"commit_full"`
	GitDirty                     bool                `json:"git_dirty"`
	GitStatusShort               string              `json:"git_status_short,omitempty"`
	Target                       string              `json:"target"`
	TLSEnabled                   bool                `json:"tls_enabled"`
	ResultDir                    string              `json:"result_dir"`
	TenantID                     string              `json:"tenant_id"`
	SenderUserID                 string              `json:"sender_user_id"`
	ReceiverUserID               string              `json:"receiver_user_id"`
	Scenario                     string              `json:"scenario"`
	ContactTopic                 string              `json:"contact_topic"`
	VerifiedAuthMetadata         bool                `json:"verified_auth_metadata"`
	GatewayFacade                bool                `json:"gateway_facade"`
	GatewayAuthMode              string              `json:"gateway_auth_mode,omitempty"`
	GatewayAuthAudience          string              `json:"gateway_auth_audience,omitempty"`
	StartedAt                    time.Time           `json:"started_at"`
	FinishedAt                   time.Time           `json:"finished_at"`
	Success                      bool                `json:"success"`
	Error                        string              `json:"error,omitempty"`
	SendContactRequest           sendSummary         `json:"send_contact_request"`
	RespondContactRequest        respondSummary      `json:"respond_contact_request"`
	CancelContactRequest         respondSummary      `json:"cancel_contact_request,omitempty"`
	ReceiverPendingBeforeRespond requestListSummary  `json:"receiver_incoming_pending_before_respond"`
	ReceiverPendingAfterRespond  requestListSummary  `json:"receiver_incoming_pending_after_respond"`
	ReceiverTerminalAfterRespond requestListSummary  `json:"receiver_incoming_terminal_after_respond"`
	ReceiverPendingAfterCancel   requestListSummary  `json:"receiver_incoming_pending_after_cancel,omitempty"`
	SenderCanceledAfterCancel    requestListSummary  `json:"sender_outgoing_canceled_after_cancel,omitempty"`
	SecondSendContactRequest     sendSummary         `json:"second_send_contact_request,omitempty"`
	SecondRespondContactRequest  respondSummary      `json:"second_respond_contact_request,omitempty"`
	DeleteContact                edgeActionSummary   `json:"delete_contact,omitempty"`
	BlockContact                 edgeActionSummary   `json:"block_contact,omitempty"`
	UnblockContact               edgeActionSummary   `json:"unblock_contact,omitempty"`
	UpdateContactRemark          edgeActionSummary   `json:"update_contact_remark,omitempty"`
	SenderList                   listSummary         `json:"sender_list"`
	ReceiverList                 listSummary         `json:"receiver_list"`
	SenderState                  stateSummary        `json:"sender_state"`
	ReceiverState                stateSummary        `json:"receiver_state"`
	ContactsOutbox               outboxStats         `json:"contacts_outbox"`
	ContactKafkaEvents           []contactKafkaEvent `json:"contact_kafka_events"`
	LatenciesMS                  map[string]float64  `json:"latencies_ms"`
}

type sendSummary struct {
	RequestID        string `json:"request_id"`
	Status           string `json:"status"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}

type respondSummary struct {
	RequestID        string `json:"request_id"`
	Status           string `json:"status"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}

type edgeActionSummary struct {
	Status           string `json:"status"`
	Version          int64  `json:"version"`
	Remark           string `json:"remark,omitempty"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}

type listSummary struct {
	OwnerUserID    string   `json:"owner_user_id"`
	ContactCount   int      `json:"contact_count"`
	ContactUserIDs []string `json:"contact_user_ids"`
}

type requestListSummary struct {
	UserID          string   `json:"user_id"`
	Direction       string   `json:"direction"`
	Status          string   `json:"status"`
	RequestCount    int      `json:"request_count"`
	RequestIDs      []string `json:"request_ids"`
	SenderUserIDs   []string `json:"sender_user_ids"`
	ReceiverUserIDs []string `json:"receiver_user_ids"`
}

type stateSummary struct {
	OwnerUserID     string `json:"owner_user_id"`
	ContactUserID   string `json:"contact_user_id"`
	Status          string `json:"status"`
	SourceRequestID string `json:"source_request_id"`
	Version         int64  `json:"version"`
	Remark          string `json:"remark,omitempty"`
	Error           string `json:"error,omitempty"`
}

type outboxStats struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Published int64 `json:"published"`
	DLQ       int64 `json:"dlq"`
}

type contactKafkaEvent struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	RequestID        string `json:"request_id"`
	SenderUserID     string `json:"sender_user_id"`
	ReceiverUserID   string `json:"receiver_user_id"`
	OwnerUserID      string `json:"owner_user_id,omitempty"`
	ContactUserID    string `json:"contact_user_id,omitempty"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
	Remark           string `json:"remark,omitempty"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
}

func main() {
	cfg := parseConfig()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseConfig() config {
	var cfg config
	var brokers string
	flag.StringVar(&cfg.target, "target", "127.0.0.1:10500", "contacts-service gRPC target")
	flag.StringVar(&cfg.tls.CAFile, "contacts-tls-ca-file", "", "CA PEM for contacts-service gRPC TLS")
	flag.StringVar(&cfg.tls.ServerName, "contacts-tls-server-name", "", "server name for contacts-service gRPC TLS")
	flag.StringVar(&cfg.tls.ClientCertFile, "contacts-tls-client-cert-file", "", "client certificate PEM for contacts-service mTLS")
	flag.StringVar(&cfg.tls.ClientKeyFile, "contacts-tls-client-key-file", "", "client private key PEM for contacts-service mTLS")
	flag.StringVar(&cfg.resultDir, "result-dir", "H:\\NexusIM\\loadtest-results\\contacts-smoke", "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flag.StringVar(&brokers, "kafka-brokers", "localhost:9092", "comma-separated Kafka brokers")
	flag.StringVar(&cfg.contactTopic, "contact-topic", "im.contact.events", "contacts Kafka topic")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "per-request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 10*time.Second, "wait timeout for outbox/Kafka")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-contacts-smoke", "tenant id")
	flag.StringVar(&cfg.senderUserID, "sender-user-id", "contact-sender", "sender user id")
	flag.StringVar(&cfg.receiverUserID, "receiver-user-id", "contact-receiver", "receiver user id")
	flag.StringVar(&cfg.senderDeviceID, "sender-device-id", "sender-device-1", "sender device id")
	flag.StringVar(&cfg.receiverDeviceID, "receiver-device-id", "receiver-device-1", "receiver device id")
	flag.StringVar(&cfg.scenario, "scenario", "accept", "scenario: accept, decline, cancel, delete, block, unblock, remark, or readd")
	flag.BoolVar(&cfg.cleanup, "cleanup", false, "delete tenant contacts rows before running")
	flag.BoolVar(&cfg.verifiedMetadata, "verified-auth-metadata", envBool("NEXUSIM_CONTACTS_LOADTEST_VERIFIED_AUTH_METADATA", false), "send gateway verified identity through contacts-service gRPC metadata")
	flag.BoolVar(&cfg.gatewayFacade, "gateway-facade", envBool("NEXUSIM_CONTACTS_LOADTEST_GATEWAY_FACADE", false), "use nexusim.gateway.v1.GatewayService for contacts user-facing RPCs")
	flag.StringVar(&cfg.gatewayAuthMode, "gateway-auth-mode", os.Getenv("NEXUSIM_CONTACTS_LOADTEST_GATEWAY_AUTH_MODE"), "api-gateway auth mode for contacts facade calls: mock or hmac")
	flag.StringVar(&cfg.gatewayAuthHMACSecret, "gateway-auth-hmac-secret", os.Getenv("NEXUSIM_CONTACTS_LOADTEST_GATEWAY_AUTH_HMAC_SECRET"), "HMAC secret used to sign api-gateway contacts token when --gateway-auth-mode=hmac")
	flag.StringVar(&cfg.gatewayAuthAudience, "gateway-auth-audience", envString("NEXUSIM_CONTACTS_LOADTEST_GATEWAY_AUTH_AUDIENCE", "api-gateway"), "audience claim used for generated api-gateway contacts token")
	flag.DurationVar(&cfg.gatewayAuthTokenTTL, "gateway-auth-token-ttl", 10*time.Minute, "TTL for generated HMAC api-gateway contacts token")
	flag.Parse()
	cfg.kafkaBrokers = splitCSV(brokers)
	cfg.scenario = strings.ToLower(strings.TrimSpace(cfg.scenario))
	if cfg.scenario == "" {
		cfg.scenario = "accept"
	}
	if cfg.pollInterval <= 0 {
		cfg.pollInterval = 200 * time.Millisecond
	}
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 5 * time.Second
	}
	if cfg.waitTimeout <= 0 {
		cfg.waitTimeout = 10 * time.Second
	}
	return cfg
}

func run(cfg config) error {
	cfg.gatewayAuthMode = strings.ToLower(strings.TrimSpace(cfg.gatewayAuthMode))
	cfg.gatewayAuthAudience = normalizedGatewayAuthAudience(cfg.gatewayAuthAudience)
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
	startedAt := time.Now().UTC()
	s := summary{
		Commit:               gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:           gitOutput("rev-parse", "HEAD"),
		GitStatusShort:       gitOutput("status", "--short"),
		Target:               cfg.target,
		TLSEnabled:           cfg.tls.Enabled(),
		ResultDir:            cfg.resultDir,
		TenantID:             cfg.tenantID,
		SenderUserID:         cfg.senderUserID,
		ReceiverUserID:       cfg.receiverUserID,
		Scenario:             cfg.scenario,
		ContactTopic:         cfg.contactTopic,
		VerifiedAuthMetadata: cfg.verifiedMetadata,
		GatewayFacade:        cfg.gatewayFacade,
		GatewayAuthMode:      cfg.gatewayAuthMode,
		GatewayAuthAudience:  gatewayAuthAudienceSummary(cfg.gatewayAuthMode, cfg.gatewayAuthAudience),
		StartedAt:            startedAt,
		LatenciesMS:          map[string]float64{},
	}
	s.GitDirty = strings.TrimSpace(s.GitStatusShort) != ""
	defer func() {
		s.FinishedAt = time.Now().UTC()
		_ = writeSummary(cfg.resultDir, s)
	}()

	if cfg.pgDSN != "" {
		pool, err := pgxpool.New(context.Background(), cfg.pgDSN)
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
	client := contactsv1.NewContactsServiceClient(conn)
	if cfg.gatewayFacade {
		client = gatewayv1.NewGatewayServiceClient(conn)
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

func envBool(name string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "":
		return fallback
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func requestContext(cfg config, userID string, deviceID string, requestID string, traceID string) (context.Context, context.CancelFunc, *contactsv1.AuthContext) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
	auth := &contactsv1.AuthContext{
		TenantId:  cfg.tenantID,
		UserId:    userID,
		DeviceId:  deviceID,
		SessionId: sessionIDForDevice(deviceID),
		RequestId: requestID,
		TraceId:   traceID,
	}
	if cfg.verifiedMetadata {
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
			metadataTenantID, auth.GetTenantId(),
			metadataUserID, auth.GetUserId(),
			metadataDeviceID, auth.GetDeviceId(),
			metadataSessionID, auth.GetSessionId(),
			metadataTraceID, auth.GetTraceId(),
			metadataRequestID, auth.GetRequestId(),
		))
	}
	if cfg.gatewayAuthMode != "" {
		ctx = withGatewayAuthMetadata(ctx, cfg, auth)
	}
	return ctx, cancel, auth
}

func withGatewayAuthMetadata(ctx context.Context, cfg config, auth *contactsv1.AuthContext) context.Context {
	switch cfg.gatewayAuthMode {
	case "mock":
		pairs := []string{
			metadataToken, auth.GetTenantId() + ":" + auth.GetUserId() + ":" + auth.GetDeviceId(),
		}
		if auth.GetTraceId() != "" {
			pairs = append(pairs, metadataTraceID, auth.GetTraceId())
		}
		if auth.GetRequestId() != "" {
			pairs = append(pairs, metadataRequestID, auth.GetRequestId())
		}
		return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
	case "hmac":
		token, err := signGatewayAuthToken(cfg.gatewayAuthHMACSecret, cfg.gatewayAuthTokenTTL, auth, cfg.gatewayAuthAudience)
		if err != nil {
			return metadata.NewOutgoingContext(ctx, metadata.Pairs("x-nexusim-loadtest-auth-error", err.Error()))
		}
		pairs := []string{"authorization", "Bearer " + token}
		if auth.GetRequestId() != "" {
			pairs = append(pairs, metadataRequestID, auth.GetRequestId())
		}
		return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
	default:
		return ctx
	}
}

func signGatewayAuthToken(secret string, ttl time.Duration, auth *contactsv1.AuthContext, audience string) (string, error) {
	return gatewayauth.SignGatewayToken(secret, map[string]string{
		"tenant_id":  auth.GetTenantId(),
		"user_id":    auth.GetUserId(),
		"device_id":  auth.GetDeviceId(),
		"session_id": auth.GetSessionId(),
		"trace_id":   auth.GetTraceId(),
		"aud":        strings.TrimSpace(audience),
	}, time.Now().Add(ttl))
}

func gatewayAuthAudienceSummary(mode string, audience string) string {
	if strings.TrimSpace(mode) == "" {
		return ""
	}
	return normalizedGatewayAuthAudience(audience)
}

func normalizedGatewayAuthAudience(audience string) string {
	audience = strings.TrimSpace(audience)
	if audience == "" {
		return "api-gateway"
	}
	return audience
}

func deviceIDForUser(cfg config, userID string) string {
	if userID == cfg.receiverUserID {
		return cfg.receiverDeviceID
	}
	return cfg.senderDeviceID
}

func sessionIDForDevice(deviceID string) string {
	if strings.TrimSpace(deviceID) == "" {
		return ""
	}
	return "contacts-smoke-" + deviceID
}

func sendContactRequest(cfg config, client contactsv1.ContactsServiceClient, suffix string) (sendSummary, float64, error) {
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

func respondContactRequest(cfg config, client contactsv1.ContactsServiceClient, requestID string, suffix string) (respondSummary, float64, error) {
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

func cancelContactRequest(cfg config, client contactsv1.ContactsServiceClient, requestID string, suffix string) (respondSummary, float64, error) {
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

func deleteContact(cfg config, client contactsv1.ContactsServiceClient, suffix string) (edgeActionSummary, float64, error) {
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

func blockContact(cfg config, client contactsv1.ContactsServiceClient, suffix string) (edgeActionSummary, float64, error) {
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

func unblockContact(cfg config, client contactsv1.ContactsServiceClient, suffix string) (edgeActionSummary, float64, error) {
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

func updateContactRemark(cfg config, client contactsv1.ContactsServiceClient, suffix string) (edgeActionSummary, float64, error) {
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

func listContacts(cfg config, client contactsv1.ContactsServiceClient, userID string) (listSummary, float64, error) {
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
	client contactsv1.ContactsServiceClient,
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

func getContactState(cfg config, client contactsv1.ContactsServiceClient, userID string, otherUserID string) (stateSummary, float64, error) {
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
