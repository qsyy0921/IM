package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	auditv1 "github.com/qsyy0921/IM/api/proto/nexusim/audit/v1"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type AuditExportExecutor struct {
	client  auditv1.AuditServiceClient
	timeout time.Duration
}

type auditExportPayload struct {
	AuditStream      string `json:"audit_stream"`
	RecordType       string `json:"record_type"`
	SourceService    string `json:"source_service"`
	FilterHash       string `json:"filter_hash"`
	RedactionProfile string `json:"redaction_profile"`
	RequestedByRef   string `json:"requested_by_ref"`
}

func NewAuditExportExecutor(client auditv1.AuditServiceClient, timeout time.Duration) AuditExportExecutor {
	if timeout <= 0 {
		timeout = time.Second
	}
	return AuditExportExecutor{client: client, timeout: timeout}
}

func DialAuditExportExecutor(
	_ context.Context,
	addr string,
	timeout time.Duration,
) (AuditExportExecutor, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return AuditExportExecutor{}, nil, errors.New("audit-service address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return AuditExportExecutor{}, nil, err
	}
	return NewAuditExportExecutor(auditv1.NewAuditServiceClient(conn), timeout), conn.Close, nil
}

func (executor AuditExportExecutor) Execute(
	ctx context.Context,
	operation types.AdminOperation,
) (types.OperationExecutionResult, error) {
	if executor.client == nil {
		return types.OperationExecutionResult{}, types.NewUnavailable("audit export executor is not configured")
	}
	payload, err := parseAuditExportPayload(operation.PayloadJSON, operation.RequestedBy)
	if err != nil {
		return types.OperationExecutionResult{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, executor.timeout)
	defer cancel()
	response, err := executor.client.CreateAuditExport(callCtx, &auditv1.CreateAuditExportRequest{
		AuthContext: &auditv1.AuthContext{
			TenantId:  string(operation.TenantID),
			UserId:    operation.RequestedBy,
			DeviceId:  "operation-worker",
			TraceId:   operation.TraceID,
			RequestId: operation.OperationID,
		},
		AuditStream:      payload.AuditStream,
		RecordType:       payload.RecordType,
		SourceService:    payload.SourceService,
		FilterHash:       payload.FilterHash,
		RedactionProfile: payload.RedactionProfile,
		RequestedByRef:   payload.RequestedByRef,
		IdempotencyKey:   "admin-audit-export:" + operation.OperationID,
		CorrelationId:    operation.CorrelationID,
		CausationId:      firstNonEmpty(operation.CausationID, operation.OperationID),
		TraceId:          operation.TraceID,
	})
	if err != nil {
		return types.OperationExecutionResult{}, mapAuditError(err)
	}
	job := response.GetExportJob()
	if job == nil || strings.TrimSpace(job.GetExportId()) == "" {
		return types.OperationExecutionResult{}, types.NewUnavailable("audit export response is incomplete")
	}
	return types.OperationExecutionResult{
		DownstreamService:    "audit-service",
		DownstreamRequestRef: fmt.Sprintf("audit-export:%s", job.GetExportId()),
		Status:               types.OperationStatusSucceeded,
	}, nil
}

func parseAuditExportPayload(raw string, fallbackRequestedBy string) (auditExportPayload, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return auditExportPayload{}, types.NewInvalidArgument("audit export payload is required")
	}
	var payload auditExportPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return auditExportPayload{}, types.NewInvalidArgument("audit export payload is malformed")
	}
	payload.AuditStream = strings.TrimSpace(payload.AuditStream)
	payload.RecordType = strings.TrimSpace(payload.RecordType)
	payload.SourceService = strings.TrimSpace(payload.SourceService)
	payload.FilterHash = strings.TrimSpace(payload.FilterHash)
	payload.RedactionProfile = strings.TrimSpace(payload.RedactionProfile)
	payload.RequestedByRef = strings.TrimSpace(payload.RequestedByRef)
	if payload.RequestedByRef == "" {
		payload.RequestedByRef = strings.TrimSpace(fallbackRequestedBy)
	}
	if payload.FilterHash == "" || payload.RedactionProfile == "" || payload.RequestedByRef == "" {
		return auditExportPayload{}, types.NewInvalidArgument("audit export payload is incomplete")
	}
	return payload, nil
}

func mapAuditError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return types.NewUnavailable("audit-service temporarily unavailable")
	}
	st, ok := status.FromError(err)
	if !ok {
		return types.NewUnavailable("audit-service temporarily unavailable")
	}
	switch st.Code() {
	case codes.InvalidArgument:
		return types.NewInvalidArgument("audit export request invalid")
	case codes.PermissionDenied:
		return types.NewPermissionDenied("audit permission denied")
	case codes.FailedPrecondition:
		return types.NewFailedPrecondition("audit precondition failed")
	case codes.AlreadyExists:
		return types.NewAlreadyExists("audit export already exists")
	case codes.NotFound:
		return types.NewNotFound("audit export target not found")
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.NewUnavailable("audit-service temporarily unavailable")
	default:
		return types.NewUnavailable("audit-service temporarily unavailable")
	}
}
