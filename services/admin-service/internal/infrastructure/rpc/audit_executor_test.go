package rpc

import (
	"context"
	"errors"
	"testing"
	"time"

	auditv1 "github.com/qsyy0921/IM/api/proto/nexusim/audit/v1"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAuditExportExecutorCreatesAuditExport(t *testing.T) {
	client := &fakeAuditClient{
		createResponse: &auditv1.CreateAuditExportResponse{
			ExportJob: &auditv1.AuditExportJob{ExportId: "audexp_1", Status: "PENDING"},
		},
	}
	executor := NewAuditExportExecutor(client, time.Second)

	result, err := executor.Execute(context.Background(), types.AdminOperation{
		TenantID:      "tenant-admin",
		OperationID:   "admop_audit_export",
		OperationType: "AUDIT_EXPORT_REQUEST",
		PayloadJSON: `{
			"audit_stream":"security",
			"record_type":"IDENTITY_AUTH",
			"source_service":"identity-service",
			"filter_hash":"sha256:filter",
			"redaction_profile":"ops-redacted"
		}`,
		ReasonRef:     "reason:audit-ticket-1",
		RequestedBy:   "admin:operator-1",
		CorrelationID: "corr-audit",
		CausationID:   "cause-audit",
		TraceID:       "trace-audit",
	})
	if err != nil {
		t.Fatalf("execute audit export: %v", err)
	}
	if result.DownstreamService != "audit-service" ||
		result.DownstreamRequestRef != "audit-export:audexp_1" ||
		result.Status != types.OperationStatusSucceeded {
		t.Fatalf("unexpected result: %+v", result)
	}
	request := client.createRequest
	if request.GetAuthContext().GetTenantId() != "tenant-admin" ||
		request.GetAuthContext().GetUserId() != "admin:operator-1" ||
		request.GetAuthContext().GetDeviceId() != "operation-worker" ||
		request.GetAuthContext().GetRequestId() != "admop_audit_export" {
		t.Fatalf("unexpected auth context: %+v", request.GetAuthContext())
	}
	if request.GetAuditStream() != "security" ||
		request.GetRecordType() != "IDENTITY_AUTH" ||
		request.GetSourceService() != "identity-service" ||
		request.GetFilterHash() != "sha256:filter" ||
		request.GetRedactionProfile() != "ops-redacted" ||
		request.GetRequestedByRef() != "admin:operator-1" ||
		request.GetIdempotencyKey() != "admin-audit-export:admop_audit_export" {
		t.Fatalf("unexpected create export request: %+v", request)
	}
}

func TestAuditExportExecutorRejectsMalformedPayload(t *testing.T) {
	client := &fakeAuditClient{}
	executor := NewAuditExportExecutor(client, time.Second)

	_, err := executor.Execute(context.Background(), types.AdminOperation{
		OperationID: "admop_audit_export_bad",
		PayloadJSON: `{"filter_hash":"sha256:filter"}`,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if client.createRequest != nil {
		t.Fatalf("malformed payload should not call audit-service")
	}
}

func TestAuditExportExecutorMapsPublicErrors(t *testing.T) {
	executor := NewAuditExportExecutor(&fakeAuditClient{
		err: status.Error(codes.PermissionDenied, "raw audit policy details"),
	}, time.Second)

	_, err := executor.Execute(context.Background(), types.AdminOperation{
		OperationID: "admop_audit_export_denied",
		RequestedBy: "admin:operator-1",
		PayloadJSON: `{
			"filter_hash":"sha256:filter",
			"redaction_profile":"ops-redacted"
		}`,
	})
	if !errors.Is(err, types.ErrPermissionDenied) || err.Error() != "audit permission denied" {
		t.Fatalf("unexpected error: %v", err)
	}
}

type fakeAuditClient struct {
	createRequest  *auditv1.CreateAuditExportRequest
	createResponse *auditv1.CreateAuditExportResponse
	err            error
}

func (client *fakeAuditClient) AppendAuditRecord(
	context.Context,
	*auditv1.AppendAuditRecordRequest,
	...grpc.CallOption,
) (*auditv1.AppendAuditRecordResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeAuditClient) QueryAuditRecords(
	context.Context,
	*auditv1.QueryAuditRecordsRequest,
	...grpc.CallOption,
) (*auditv1.QueryAuditRecordsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeAuditClient) CreateAuditExport(
	_ context.Context,
	request *auditv1.CreateAuditExportRequest,
	_ ...grpc.CallOption,
) (*auditv1.CreateAuditExportResponse, error) {
	client.createRequest = request
	if client.err != nil {
		return nil, client.err
	}
	return client.createResponse, nil
}

func (client *fakeAuditClient) GetAuditExport(
	context.Context,
	*auditv1.GetAuditExportRequest,
	...grpc.CallOption,
) (*auditv1.GetAuditExportResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeAuditClient) VerifyAuditProof(
	context.Context,
	*auditv1.VerifyAuditProofRequest,
	...grpc.CallOption,
) (*auditv1.VerifyAuditProofResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
