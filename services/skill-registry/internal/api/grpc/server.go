package grpc

import (
	"context"
	"errors"

	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	skillregistryv1 "github.com/qsyy0921/IM/api/proto/nexusim/skillregistry/v1"
	"github.com/qsyy0921/IM/services/skill-registry/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UpsertSkillExecutor interface {
	Execute(context.Context, types.UpsertSkillCommand) (types.SkillDefinition, error)
}

type GetSkillExecutor interface {
	Execute(context.Context, types.GetSkillCommand) (types.SkillDefinition, error)
}

type ListSkillsExecutor interface {
	Execute(context.Context, types.ListSkillsCommand) (types.ListSkillsResult, error)
}

type Server struct {
	skillregistryv1.UnimplementedSkillRegistryServiceServer
	upsertSkill UpsertSkillExecutor
	getSkill    GetSkillExecutor
	listSkills  ListSkillsExecutor
}

func NewServer(upsertSkill UpsertSkillExecutor, getSkill GetSkillExecutor, listSkills ListSkillsExecutor) *Server {
	return &Server{
		upsertSkill: upsertSkill,
		getSkill:    getSkill,
		listSkills:  listSkills,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	skillregistryv1.RegisterSkillRegistryServiceServer(registrar, server)
}

func (server *Server) UpsertSkill(
	ctx context.Context,
	request *skillregistryv1.UpsertSkillRequest,
) (*skillregistryv1.UpsertSkillResponse, error) {
	if request == nil || request.GetSkill() == nil {
		return nil, status.Error(codes.InvalidArgument, "skill is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	skill := skillFromProto(request.GetSkill())
	skill.TenantID = auth.TenantID
	result, err := server.upsertSkill.Execute(ctx, types.UpsertSkillCommand{
		AuthContext: auth,
		Skill:       skill,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &skillregistryv1.UpsertSkillResponse{Skill: skillToProto(result)}, nil
}

func (server *Server) GetSkill(
	ctx context.Context,
	request *skillregistryv1.GetSkillRequest,
) (*skillregistryv1.GetSkillResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.getSkill.Execute(ctx, types.GetSkillCommand{
		AuthContext: auth,
		SkillID:     request.GetSkillId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &skillregistryv1.GetSkillResponse{Skill: skillToProto(result)}, nil
}

func (server *Server) ListSkills(
	ctx context.Context,
	request *skillregistryv1.ListSkillsRequest,
) (*skillregistryv1.ListSkillsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.listSkills.Execute(ctx, types.ListSkillsCommand{
		AuthContext:  auth,
		Status:       statusFromProto(request.GetStatus()),
		OwnerService: request.GetOwnerService(),
		ToolName:     request.GetToolName(),
		Tag:          request.GetTag(),
		AfterSkillID: request.GetAfterSkillId(),
		Limit:        int(request.GetLimit()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	skills := make([]*skillregistryv1.SkillDefinition, 0, len(result.Skills))
	for _, skill := range result.Skills {
		skills = append(skills, skillToProto(skill))
	}
	return &skillregistryv1.ListSkillsResponse{
		Skills:     skills,
		NextCursor: result.NextCursor,
	}, nil
}

func authFromProto(ctx context.Context, auth *skillregistryv1.AuthContext) (types.AuthContext, bool) {
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

func skillFromProto(skill *skillregistryv1.SkillDefinition) types.SkillDefinition {
	actions := make([]int32, 0, len(skill.GetAllowedActions()))
	for _, action := range skill.GetAllowedActions() {
		actions = append(actions, int32(action))
	}
	return types.SkillDefinition{
		TenantID:         types.TenantID(skill.GetTenantId()),
		SkillID:          skill.GetSkillId(),
		DisplayName:      skill.GetDisplayName(),
		Description:      skill.GetDescription(),
		Version:          skill.GetVersion(),
		Status:           statusFromProto(skill.GetStatus()),
		ToolName:         skill.GetToolName(),
		AllowedActions:   actions,
		InputSchemaJSON:  skill.GetInputSchemaJson(),
		OutputSchemaJSON: skill.GetOutputSchemaJson(),
		PermissionScope:  skill.GetPermissionScope(),
		RiskLevel:        skill.GetRiskLevel(),
		RequiresApproval: skill.GetRequiresApproval(),
		AuditEventType:   skill.GetAuditEventType(),
		OwnerService:     skill.GetOwnerService(),
		Tags:             skill.GetTags(),
		MetadataJSON:     skill.GetMetadataJson(),
	}
}

func skillToProto(skill types.SkillDefinition) *skillregistryv1.SkillDefinition {
	actions := make([]policyv1.ToolAction, 0, len(skill.AllowedActions))
	for _, action := range skill.AllowedActions {
		actions = append(actions, policyv1.ToolAction(action))
	}
	return &skillregistryv1.SkillDefinition{
		TenantId:         string(skill.TenantID),
		SkillId:          skill.SkillID,
		DisplayName:      skill.DisplayName,
		Description:      skill.Description,
		Version:          skill.Version,
		Status:           statusToProto(skill.Status),
		ToolName:         skill.ToolName,
		AllowedActions:   actions,
		InputSchemaJson:  skill.InputSchemaJSON,
		OutputSchemaJson: skill.OutputSchemaJSON,
		PermissionScope:  skill.PermissionScope,
		RiskLevel:        skill.RiskLevel,
		RequiresApproval: skill.RequiresApproval,
		AuditEventType:   skill.AuditEventType,
		OwnerService:     skill.OwnerService,
		Tags:             skill.Tags,
		MetadataJson:     skill.MetadataJSON,
		CreatedAtUnixMs:  skill.CreatedAt.UnixMilli(),
		UpdatedAtUnixMs:  skill.UpdatedAt.UnixMilli(),
	}
}

func statusFromProto(status skillregistryv1.SkillStatus) string {
	switch status {
	case skillregistryv1.SkillStatus_SKILL_STATUS_DISABLED:
		return types.SkillStatusDisabled
	case skillregistryv1.SkillStatus_SKILL_STATUS_ACTIVE:
		return types.SkillStatusActive
	default:
		return ""
	}
}

func statusToProto(status string) skillregistryv1.SkillStatus {
	switch status {
	case types.SkillStatusDisabled:
		return skillregistryv1.SkillStatus_SKILL_STATUS_DISABLED
	case types.SkillStatusActive:
		return skillregistryv1.SkillStatus_SKILL_STATUS_ACTIVE
	default:
		return skillregistryv1.SkillStatus_SKILL_STATUS_UNSPECIFIED
	}
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrSkillNotFound):
		return status.Error(codes.NotFound, "skill not found")
	case errors.Is(err, types.ErrDBReadFailed):
		return status.Error(codes.Unavailable, "skill registry read failed")
	case errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "skill registry write failed")
	default:
		return status.Error(codes.Internal, "skill registry internal error")
	}
}
