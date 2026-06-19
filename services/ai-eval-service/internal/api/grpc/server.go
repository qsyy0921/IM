package grpc

import (
	"context"
	"errors"

	aievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/aieval/v1"
	"github.com/qsyy0921/IM/services/ai-eval-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RecordEvalRunExecutor interface {
	Execute(context.Context, types.RecordEvalRunCommand) (types.EvalRun, error)
}

type GetEvalRunExecutor interface {
	Execute(context.Context, types.GetEvalRunCommand) (types.EvalRun, error)
}

type ListEvalRunsExecutor interface {
	Execute(context.Context, types.ListEvalRunsCommand) (types.ListEvalRunsResult, error)
}

type Server struct {
	aievalv1.UnimplementedAIEvalServiceServer
	recordEvalRun RecordEvalRunExecutor
	getEvalRun    GetEvalRunExecutor
	listEvalRuns  ListEvalRunsExecutor
}

func NewServer(
	recordEvalRun RecordEvalRunExecutor,
	getEvalRun GetEvalRunExecutor,
	listEvalRuns ListEvalRunsExecutor,
) *Server {
	return &Server{
		recordEvalRun: recordEvalRun,
		getEvalRun:    getEvalRun,
		listEvalRuns:  listEvalRuns,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	aievalv1.RegisterAIEvalServiceServer(registrar, server)
}

func (server *Server) RecordEvalRun(
	ctx context.Context,
	request *aievalv1.RecordEvalRunRequest,
) (*aievalv1.RecordEvalRunResponse, error) {
	if request == nil || request.GetRun() == nil {
		return nil, status.Error(codes.InvalidArgument, "eval run is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	run := evalRunFromProto(request.GetRun())
	run.TenantID = auth.TenantID
	result, err := server.recordEvalRun.Execute(ctx, types.RecordEvalRunCommand{
		AuthContext: auth,
		Run:         run,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &aievalv1.RecordEvalRunResponse{Run: evalRunToProto(result)}, nil
}

func (server *Server) GetEvalRun(
	ctx context.Context,
	request *aievalv1.GetEvalRunRequest,
) (*aievalv1.GetEvalRunResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.getEvalRun.Execute(ctx, types.GetEvalRunCommand{
		AuthContext: auth,
		RunID:       request.GetRunId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &aievalv1.GetEvalRunResponse{Run: evalRunToProto(result)}, nil
}

func (server *Server) ListEvalRuns(
	ctx context.Context,
	request *aievalv1.ListEvalRunsRequest,
) (*aievalv1.ListEvalRunsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.listEvalRuns.Execute(ctx, types.ListEvalRunsCommand{
		AuthContext: auth,
		SuiteID:     request.GetSuiteId(),
		Status:      statusFromProto(request.GetStatus()),
		AfterRunID:  request.GetAfterRunId(),
		Limit:       int(request.GetLimit()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	runs := make([]*aievalv1.EvalRun, 0, len(result.Runs))
	for _, run := range result.Runs {
		runs = append(runs, evalRunToProto(run))
	}
	return &aievalv1.ListEvalRunsResponse{
		Runs:       runs,
		NextCursor: result.NextCursor,
	}, nil
}

func authFromProto(ctx context.Context, auth *aievalv1.AuthContext) (types.AuthContext, bool) {
	if verified, ok := verifiedAuthFromContext(ctx); ok {
		if auth != nil {
			if verified.TraceID == "" {
				verified.TraceID = auth.GetTraceId()
			}
			if verified.RequestID == "" {
				verified.RequestID = auth.GetRequestId()
			}
		}
		return verified, true
	}
	if auth == nil {
		return types.AuthContext{}, false
	}
	return types.AuthContext{
		TenantID:  types.TenantID(auth.GetTenantId()),
		UserID:    types.UserID(auth.GetUserId()),
		DeviceID:  auth.GetDeviceId(),
		SessionID: auth.GetSessionId(),
		TraceID:   auth.GetTraceId(),
		RequestID: auth.GetRequestId(),
	}, true
}

func evalRunFromProto(run *aievalv1.EvalRun) types.EvalRun {
	return types.EvalRun{
		TenantID:     types.TenantID(run.GetTenantId()),
		RunID:        run.GetRunId(),
		SuiteID:      run.GetSuiteId(),
		Stage:        run.GetStage(),
		Adapter:      run.GetAdapter(),
		Status:       statusFromProto(run.GetStatus()),
		CaseCount:    int(run.GetCaseCount()),
		PassedCount:  int(run.GetPassedCount()),
		FailedCount:  int(run.GetFailedCount()),
		SkippedCount: int(run.GetSkippedCount()),
		SummaryRef:   run.GetSummaryRef(),
		ReportRef:    run.GetReportRef(),
		MetadataJSON: run.GetMetadataJson(),
	}
}

func evalRunToProto(run types.EvalRun) *aievalv1.EvalRun {
	completedAtUnixMs := int64(0)
	if !run.CompletedAt.IsZero() {
		completedAtUnixMs = run.CompletedAt.UnixMilli()
	}
	return &aievalv1.EvalRun{
		TenantId:          string(run.TenantID),
		RunId:             run.RunID,
		SuiteId:           run.SuiteID,
		Stage:             run.Stage,
		Adapter:           run.Adapter,
		Status:            statusToProto(run.Status),
		CaseCount:         int32(run.CaseCount),
		PassedCount:       int32(run.PassedCount),
		FailedCount:       int32(run.FailedCount),
		SkippedCount:      int32(run.SkippedCount),
		SummaryRef:        run.SummaryRef,
		ReportRef:         run.ReportRef,
		MetadataJson:      run.MetadataJSON,
		CreatedAtUnixMs:   run.CreatedAt.UnixMilli(),
		UpdatedAtUnixMs:   run.UpdatedAt.UnixMilli(),
		CompletedAtUnixMs: completedAtUnixMs,
	}
}

func statusFromProto(status aievalv1.EvalRunStatus) string {
	switch status {
	case aievalv1.EvalRunStatus_EVAL_RUN_STATUS_PENDING:
		return types.EvalRunStatusPending
	case aievalv1.EvalRunStatus_EVAL_RUN_STATUS_RUNNING:
		return types.EvalRunStatusRunning
	case aievalv1.EvalRunStatus_EVAL_RUN_STATUS_PASSED:
		return types.EvalRunStatusPassed
	case aievalv1.EvalRunStatus_EVAL_RUN_STATUS_FAILED:
		return types.EvalRunStatusFailed
	default:
		return ""
	}
}

func statusToProto(status string) aievalv1.EvalRunStatus {
	switch status {
	case types.EvalRunStatusPending:
		return aievalv1.EvalRunStatus_EVAL_RUN_STATUS_PENDING
	case types.EvalRunStatusRunning:
		return aievalv1.EvalRunStatus_EVAL_RUN_STATUS_RUNNING
	case types.EvalRunStatusPassed:
		return aievalv1.EvalRunStatus_EVAL_RUN_STATUS_PASSED
	case types.EvalRunStatusFailed:
		return aievalv1.EvalRunStatus_EVAL_RUN_STATUS_FAILED
	default:
		return aievalv1.EvalRunStatus_EVAL_RUN_STATUS_UNSPECIFIED
	}
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrEvalRunNotFound):
		return status.Error(codes.NotFound, "eval run not found")
	case errors.Is(err, types.ErrDBReadFailed):
		return status.Error(codes.Unavailable, "ai eval read failed")
	case errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "ai eval write failed")
	default:
		return status.Error(codes.Internal, "ai eval internal error")
	}
}
