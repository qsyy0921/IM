package main

import (
	"context"
	"time"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	"google.golang.org/grpc"
)

type contactsClient interface {
	SendContactRequest(context.Context, *contactsv1.SendContactRequestRequest, ...grpc.CallOption) (*contactsv1.SendContactRequestResponse, error)
	RespondContactRequest(context.Context, *contactsv1.RespondContactRequestRequest, ...grpc.CallOption) (*contactsv1.RespondContactRequestResponse, error)
	CancelContactRequest(context.Context, *contactsv1.CancelContactRequestRequest, ...grpc.CallOption) (*contactsv1.CancelContactRequestResponse, error)
	ListContactRequests(context.Context, *contactsv1.ListContactRequestsRequest, ...grpc.CallOption) (*contactsv1.ListContactRequestsResponse, error)
	ListContacts(context.Context, *contactsv1.ListContactsRequest, ...grpc.CallOption) (*contactsv1.ListContactsResponse, error)
	GetContactState(context.Context, *contactsv1.GetContactStateRequest, ...grpc.CallOption) (*contactsv1.GetContactStateResponse, error)
	DeleteContact(context.Context, *contactsv1.DeleteContactRequest, ...grpc.CallOption) (*contactsv1.DeleteContactResponse, error)
	BlockContact(context.Context, *contactsv1.BlockContactRequest, ...grpc.CallOption) (*contactsv1.BlockContactResponse, error)
	UnblockContact(context.Context, *contactsv1.UnblockContactRequest, ...grpc.CallOption) (*contactsv1.UnblockContactResponse, error)
	UpdateContactRemark(context.Context, *contactsv1.UpdateContactRemarkRequest, ...grpc.CallOption) (*contactsv1.UpdateContactRemarkResponse, error)
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
	Capacity                     *capacitySummary    `json:"capacity_summary,omitempty"`
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

type capacitySummary struct {
	DurationSeconds       float64 `json:"duration_seconds"`
	Scenario              string  `json:"scenario"`
	OperationCount        int     `json:"operation_count"`
	ContactEventCount     int     `json:"contact_event_count"`
	ContactsOutboxTotal   int64   `json:"contacts_outbox_total"`
	ContactsOutboxPending int64   `json:"contacts_outbox_pending"`
	ContactsOutboxDLQ     int64   `json:"contacts_outbox_dlq"`
	OperationsPerSecond   float64 `json:"operations_per_second"`
	EventsPerSecond       float64 `json:"events_per_second"`
	LatencyP95MS          float64 `json:"latency_p95_ms,omitempty"`
	LatencyP99MS          float64 `json:"latency_p99_ms,omitempty"`
}
