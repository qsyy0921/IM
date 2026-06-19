package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	skillregistryv1 "github.com/qsyy0921/IM/api/proto/nexusim/skillregistry/v1"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type SkillRegistryClient struct {
	client  skillregistryv1.SkillRegistryServiceClient
	timeout time.Duration
}

func NewSkillRegistryClient(client skillregistryv1.SkillRegistryServiceClient, timeout time.Duration) SkillRegistryClient {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return SkillRegistryClient{client: client, timeout: timeout}
}

func DialSkillRegistryClient(_ context.Context, addr string, timeout time.Duration) (SkillRegistryClient, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return SkillRegistryClient{}, nil, errors.New("skill-registry address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return SkillRegistryClient{}, nil, err
	}
	return NewSkillRegistryClient(skillregistryv1.NewSkillRegistryServiceClient(conn), timeout), conn.Close, nil
}

func (client SkillRegistryClient) GetSkill(
	ctx context.Context,
	auth types.AuthContext,
	skillID string,
) (types.SkillDefinition, error) {
	callCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	callCtx = outgoingMetadataContext(callCtx, auth)
	response, err := client.client.GetSkill(callCtx, &skillregistryv1.GetSkillRequest{
		AuthContext: &skillregistryv1.AuthContext{
			TenantId:  string(auth.TenantID),
			UserId:    string(auth.UserID),
			DeviceId:  auth.DeviceID,
			SessionId: auth.SessionID,
			TraceId:   auth.TraceID,
			RequestId: auth.RequestID,
		},
		SkillId: skillID,
	})
	if err != nil {
		return types.SkillDefinition{}, mapSkillError(err)
	}
	return skillFromProto(response.GetSkill()), nil
}

func skillFromProto(skill *skillregistryv1.SkillDefinition) types.SkillDefinition {
	if skill == nil {
		return types.SkillDefinition{}
	}
	actions := make([]string, 0, len(skill.GetAllowedActions()))
	for _, action := range skill.GetAllowedActions() {
		actions = append(actions, toolActionFromProto(action))
	}
	return types.SkillDefinition{
		TenantID:         types.TenantID(skill.GetTenantId()),
		SkillID:          skill.GetSkillId(),
		Status:           skillStatusFromProto(skill.GetStatus()),
		ToolName:         skill.GetToolName(),
		AllowedActions:   actions,
		RiskLevel:        skill.GetRiskLevel(),
		RequiresApproval: skill.GetRequiresApproval(),
		AuditEventType:   skill.GetAuditEventType(),
		OwnerService:     skill.GetOwnerService(),
	}
}

func skillStatusFromProto(status skillregistryv1.SkillStatus) string {
	switch status {
	case skillregistryv1.SkillStatus_SKILL_STATUS_ACTIVE:
		return types.SkillStatusActive
	case skillregistryv1.SkillStatus_SKILL_STATUS_DISABLED:
		return types.SkillStatusDisabled
	default:
		return ""
	}
}

func toolActionFromProto(action policyv1.ToolAction) string {
	switch action {
	case policyv1.ToolAction_TOOL_ACTION_CALL:
		return types.ToolActionCall
	case policyv1.ToolAction_TOOL_ACTION_APPROVE:
		return types.ToolActionApprove
	case policyv1.ToolAction_TOOL_ACTION_EXECUTE:
		return types.ToolActionExecute
	default:
		return ""
	}
}
