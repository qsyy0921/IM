package grpc

import (
	"context"
	"errors"
	"time"

	controlv1 "github.com/qsyy0921/IM/api/proto/nexusim/controlplane/v1"
	"github.com/qsyy0921/IM/services/control-plane-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PublishConfigVersionExecutor interface {
	Execute(context.Context, types.PublishConfigVersionCommand) (types.ConfigVersion, error)
}

type RollbackConfigVersionExecutor interface {
	Execute(context.Context, types.RollbackConfigVersionCommand) (types.ConfigVersion, bool, error)
}

type GetConfigSnapshotExecutor interface {
	Execute(context.Context, types.GetConfigSnapshotCommand) (types.ConfigSnapshot, error)
}

type AckAppliedConfigVersionExecutor interface {
	Execute(context.Context, types.AckAppliedConfigVersionCommand) (types.AppliedConfigVersion, error)
}

type Server struct {
	controlv1.UnimplementedControlPlaneServiceServer
	publishConfigVersion    PublishConfigVersionExecutor
	rollbackConfigVersion   RollbackConfigVersionExecutor
	getConfigSnapshot       GetConfigSnapshotExecutor
	ackAppliedConfigVersion AckAppliedConfigVersionExecutor
}

func NewServer(
	publishConfigVersion PublishConfigVersionExecutor,
	rollbackConfigVersion RollbackConfigVersionExecutor,
	getConfigSnapshot GetConfigSnapshotExecutor,
	ackAppliedConfigVersion AckAppliedConfigVersionExecutor,
) *Server {
	return &Server{
		publishConfigVersion:    publishConfigVersion,
		rollbackConfigVersion:   rollbackConfigVersion,
		getConfigSnapshot:       getConfigSnapshot,
		ackAppliedConfigVersion: ackAppliedConfigVersion,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	controlv1.RegisterControlPlaneServiceServer(registrar, server)
}

func (server *Server) PublishConfigVersion(
	ctx context.Context,
	request *controlv1.PublishConfigVersionRequest,
) (*controlv1.PublishConfigVersionResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	version, err := server.publishConfigVersion.Execute(ctx, types.PublishConfigVersionCommand{
		AuthContext:    auth,
		Environment:    request.GetEnvironment(),
		ConfigKind:     request.GetConfigKind(),
		BundleKey:      request.GetBundleKey(),
		Version:        request.GetVersion(),
		SchemaVersion:  request.GetSchemaVersion(),
		PayloadJSON:    request.GetPayloadJson(),
		EffectiveAt:    unixMillisToTime(request.GetEffectiveAtUnixMs()),
		ExpiresAt:      unixMillisToTime(request.GetExpiresAtUnixMs()),
		ApprovalRef:    request.GetApprovalRef(),
		OperatorRef:    request.GetOperatorRef(),
		ReasonRef:      request.GetReasonRef(),
		IdempotencyKey: request.GetIdempotencyKey(),
		CorrelationID:  request.GetCorrelationId(),
		CausationID:    request.GetCausationId(),
		TraceID:        request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &controlv1.PublishConfigVersionResponse{
		Version: versionToProto(version),
		Snapshot: snapshotToProto(types.ConfigSnapshot{
			TenantID:        version.TenantID,
			Environment:     version.Environment,
			ConfigKind:      version.ConfigKind,
			BundleKey:       version.BundleKey,
			Version:         version.Version,
			SchemaVersion:   version.SchemaVersion,
			PayloadJSON:     version.PayloadJSON,
			PayloadChecksum: version.PayloadChecksum,
			GeneratedAt:     time.Now().UTC(),
			EffectiveAt:     version.EffectiveAt,
			ExpiresAt:       version.ExpiresAt,
			RolloutDecision: "MATCH",
		}),
	}, nil
}

func (server *Server) RollbackConfigVersion(
	ctx context.Context,
	request *controlv1.RollbackConfigVersionRequest,
) (*controlv1.RollbackConfigVersionResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	version, replayed, err := server.rollbackConfigVersion.Execute(ctx, types.RollbackConfigVersionCommand{
		AuthContext:    auth,
		Environment:    request.GetEnvironment(),
		ConfigKind:     request.GetConfigKind(),
		BundleKey:      request.GetBundleKey(),
		TargetVersion:  request.GetTargetVersion(),
		ApprovalRef:    request.GetApprovalRef(),
		OperatorRef:    request.GetOperatorRef(),
		ReasonRef:      request.GetReasonRef(),
		IdempotencyKey: request.GetIdempotencyKey(),
		CorrelationID:  request.GetCorrelationId(),
		CausationID:    request.GetCausationId(),
		TraceID:        request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &controlv1.RollbackConfigVersionResponse{
		Version: versionToProto(version),
		Snapshot: snapshotToProto(types.ConfigSnapshot{
			TenantID:        version.TenantID,
			Environment:     version.Environment,
			ConfigKind:      version.ConfigKind,
			BundleKey:       version.BundleKey,
			Version:         version.Version,
			SchemaVersion:   version.SchemaVersion,
			PayloadJSON:     version.PayloadJSON,
			PayloadChecksum: version.PayloadChecksum,
			GeneratedAt:     time.Now().UTC(),
			EffectiveAt:     version.EffectiveAt,
			ExpiresAt:       version.ExpiresAt,
			RolloutDecision: "MATCH",
		}),
		Replayed: replayed,
	}, nil
}

func (server *Server) GetConfigSnapshot(
	ctx context.Context,
	request *controlv1.GetConfigSnapshotRequest,
) (*controlv1.GetConfigSnapshotResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	snapshot, err := server.getConfigSnapshot.Execute(ctx, types.GetConfigSnapshotCommand{
		AuthContext:    auth,
		Environment:    request.GetEnvironment(),
		ServiceName:    request.GetServiceName(),
		ConfigKind:     request.GetConfigKind(),
		BundleKey:      request.GetBundleKey(),
		CurrentVersion: request.GetCurrentVersion(),
		InstanceRef:    request.GetInstanceRef(),
		Ring:           request.GetRing(),
		ServiceVersion: request.GetServiceVersion(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &controlv1.GetConfigSnapshotResponse{
		Snapshot:    snapshotToProto(snapshot),
		NotModified: snapshot.NotModified,
	}, nil
}

func (server *Server) AckAppliedConfigVersion(
	ctx context.Context,
	request *controlv1.AckAppliedConfigVersionRequest,
) (*controlv1.AckAppliedConfigVersionResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	applied, err := server.ackAppliedConfigVersion.Execute(ctx, types.AckAppliedConfigVersionCommand{
		AuthContext:    auth,
		Environment:    request.GetEnvironment(),
		ServiceName:    request.GetServiceName(),
		InstanceRef:    request.GetInstanceRef(),
		ConfigKind:     request.GetConfigKind(),
		BundleKey:      request.GetBundleKey(),
		Version:        request.GetVersion(),
		ServiceVersion: request.GetServiceVersion(),
		Status:         request.GetStatus(),
		LastErrorClass: request.GetLastErrorClass(),
		CorrelationID:  request.GetCorrelationId(),
		CausationID:    request.GetCausationId(),
		TraceID:        request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &controlv1.AckAppliedConfigVersionResponse{Applied: appliedToProto(applied)}, nil
}

func authFromProto(ctx context.Context, auth *controlv1.AuthContext) (types.AuthContext, bool) {
	if verified, ok := verifiedAuthFromContext(ctx); ok {
		return verified, true
	}
	if auth == nil {
		return types.AuthContext{}, false
	}
	return types.AuthContext{
		TenantID:    types.TenantID(auth.GetTenantId()),
		UserID:      auth.GetUserId(),
		ServiceName: auth.GetServiceName(),
		InstanceRef: auth.GetInstanceRef(),
		TraceID:     auth.GetTraceId(),
		RequestID:   auth.GetRequestId(),
	}, true
}

func versionToProto(version types.ConfigVersion) *controlv1.ConfigVersion {
	return &controlv1.ConfigVersion{
		TenantId:          string(version.TenantID),
		Environment:       version.Environment,
		ConfigKind:        version.ConfigKind,
		BundleKey:         version.BundleKey,
		Version:           version.Version,
		SchemaVersion:     version.SchemaVersion,
		PayloadChecksum:   version.PayloadChecksum,
		Status:            version.Status,
		EffectiveAtUnixMs: timeToUnixMillis(version.EffectiveAt),
		ExpiresAtUnixMs:   timeToUnixMillis(version.ExpiresAt),
		PublishedAtUnixMs: timeToUnixMillis(version.PublishedAt),
	}
}

func snapshotToProto(snapshot types.ConfigSnapshot) *controlv1.ConfigSnapshot {
	return &controlv1.ConfigSnapshot{
		TenantId:          string(snapshot.TenantID),
		Environment:       snapshot.Environment,
		ServiceName:       snapshot.ServiceName,
		ConfigKind:        snapshot.ConfigKind,
		BundleKey:         snapshot.BundleKey,
		Version:           snapshot.Version,
		SchemaVersion:     snapshot.SchemaVersion,
		PayloadJson:       snapshot.PayloadJSON,
		PayloadChecksum:   snapshot.PayloadChecksum,
		GeneratedAtUnixMs: timeToUnixMillis(snapshot.GeneratedAt),
		EffectiveAtUnixMs: timeToUnixMillis(snapshot.EffectiveAt),
		ExpiresAtUnixMs:   timeToUnixMillis(snapshot.ExpiresAt),
		RolloutDecision:   snapshot.RolloutDecision,
		PreviousVersion:   snapshot.PreviousVersion,
	}
}

func appliedToProto(applied types.AppliedConfigVersion) *controlv1.AppliedConfigVersion {
	return &controlv1.AppliedConfigVersion{
		TenantId:        string(applied.TenantID),
		Environment:     applied.Environment,
		ServiceName:     applied.ServiceName,
		InstanceRef:     applied.InstanceRef,
		ConfigKind:      applied.ConfigKind,
		BundleKey:       applied.BundleKey,
		Version:         applied.Version,
		ServiceVersion:  applied.ServiceVersion,
		Status:          applied.Status,
		LastErrorClass:  applied.LastErrorClass,
		AppliedAtUnixMs: timeToUnixMillis(applied.AppliedAt),
	}
}

func unixMillisToTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}

func timeToUnixMillis(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().UnixMilli()
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, "config version already exists")
	case errors.Is(err, types.ErrNotFound):
		return status.Error(codes.NotFound, "config snapshot not found")
	case errors.Is(err, types.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, "control-plane precondition failed")
	case errors.Is(err, types.ErrDBReadFailed):
		return status.Error(codes.Unavailable, "control-plane read failed")
	case errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "control-plane write failed")
	default:
		return status.Error(codes.Internal, "control-plane internal error")
	}
}
