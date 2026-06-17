package types

import "strings"

type ContactRequestSourcePolicy struct {
	SourceType           ContactRequestSourceType
	AllowContactRequests bool
	RiskLevel            ContactRequestRiskLevel
	ReviewRequired       bool
	Version              int64
	UpdatedAtUnixMS      int64
}

type GetTenantContactRequestSourcePolicyCommand struct {
	TenantID   TenantID
	SourceType ContactRequestSourceType
}

func (c GetTenantContactRequestSourcePolicyCommand) Validate() error {
	if c.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if NormalizeContactRequestSourceType(c.SourceType) == "" {
		return NewInvalidArgument("source_type is invalid")
	}
	return nil
}

func (c GetTenantContactRequestSourcePolicyCommand) NormalizedSourceType() ContactRequestSourceType {
	return NormalizeContactRequestSourceType(c.SourceType)
}

type GetTenantContactRequestSourcePolicyResult struct {
	TenantID TenantID
	Policy   ContactRequestSourcePolicy
}

type SetTenantContactRequestSourcePolicyCommand struct {
	TenantID             TenantID
	SourceType           ContactRequestSourceType
	AllowContactRequests bool
	RiskLevel            ContactRequestRiskLevel
	ReviewRequired       bool
}

func (c SetTenantContactRequestSourcePolicyCommand) Validate() error {
	if c.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if NormalizeContactRequestSourceType(c.SourceType) == "" {
		return NewInvalidArgument("source_type is invalid")
	}
	if NormalizeContactRequestRiskLevel(c.RiskLevel) == "" {
		return NewInvalidArgument("risk_level is invalid")
	}
	return nil
}

func (c SetTenantContactRequestSourcePolicyCommand) NormalizedSourceType() ContactRequestSourceType {
	return NormalizeContactRequestSourceType(c.SourceType)
}

func (c SetTenantContactRequestSourcePolicyCommand) NormalizedRiskLevel() ContactRequestRiskLevel {
	return NormalizeContactRequestRiskLevel(c.RiskLevel)
}

type SetTenantContactRequestSourcePolicyResult struct {
	TenantID TenantID
	Policy   ContactRequestSourcePolicy
	Changed  bool
}

type ReviewContactRequestCommand struct {
	TenantID  TenantID
	RequestID string
	Decision  ContactRequestReviewDecision
	Operator  string
	Reason    string
}

func (c ReviewContactRequestCommand) Validate() error {
	if c.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if strings.TrimSpace(c.RequestID) == "" {
		return NewInvalidArgument("request_id is required")
	}
	if NormalizeContactRequestReviewDecision(c.Decision) == "" {
		return NewInvalidArgument("decision is invalid")
	}
	if strings.TrimSpace(c.Operator) == "" {
		return NewInvalidArgument("operator is required")
	}
	if len(c.Reason) > maxContactReviewReasonLength {
		return NewInvalidArgument("reason is too long")
	}
	return nil
}

func (c ReviewContactRequestCommand) NormalizedDecision() ContactRequestReviewDecision {
	return NormalizeContactRequestReviewDecision(c.Decision)
}

func (c ReviewContactRequestCommand) NormalizedOperator() string {
	return strings.TrimSpace(c.Operator)
}

func NormalizeContactRequestReviewDecision(value ContactRequestReviewDecision) ContactRequestReviewDecision {
	switch ContactRequestReviewDecision(strings.ToUpper(strings.TrimSpace(string(value)))) {
	case ContactRequestReviewDecisionApprove:
		return ContactRequestReviewDecisionApprove
	case ContactRequestReviewDecisionDecline:
		return ContactRequestReviewDecisionDecline
	default:
		return ""
	}
}

type ReviewContactRequestResult struct {
	RequestID      string
	TenantID       TenantID
	SenderUserID   UserID
	ReceiverUserID UserID
	PreviousStatus ContactRequestStatus
	Status         ContactRequestStatus
	Decision       ContactRequestReviewDecision
	RiskLevel      ContactRequestRiskLevel
	ReviewRequired bool
}
