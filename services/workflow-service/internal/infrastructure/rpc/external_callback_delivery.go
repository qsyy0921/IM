package rpc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

type ExternalCallbackEndpoint struct {
	Ref string `json:"ref"`
	URL string `json:"url"`
}

type ExternalCallbackEndpointFile struct {
	Endpoints []ExternalCallbackEndpoint `json:"endpoints"`
}

type ExternalCallbackHTTPProvider struct {
	client    *http.Client
	endpoints map[string]string
	timeout   time.Duration
}

type externalCallbackDeliveryPlan struct {
	SchemaVersion                string `json:"schema_version"`
	DeliveryPlanID               string `json:"delivery_plan_id"`
	SourceDecisionManifestSha256 string `json:"source_decision_manifest_sha256"`
	WorkflowBinding              struct {
		WorkflowID                   string `json:"workflow_id"`
		StepID                       string `json:"step_id"`
		ExpectedWorkflowType         string `json:"expected_workflow_type"`
		ExpectedStatus               string `json:"expected_status"`
		ExpectedTargetService        string `json:"expected_target_service"`
		ExpectedTargetOperation      string `json:"expected_target_operation"`
		ExpectedTargetRefHash        string `json:"expected_target_ref_hash"`
		ExpectedPayloadSchemaVersion string `json:"expected_payload_schema_version"`
		ExpectedPayloadRefHash       string `json:"expected_payload_ref_hash"`
		ExpectedApprovalPolicyRef    string `json:"expected_approval_policy_ref"`
		DecisionPolicyRef            string `json:"decision_policy_ref"`
	} `json:"workflow_binding"`
	CallbackDeliveryContract struct {
		CallbackProviderRef          string `json:"callback_provider_ref"`
		CallbackEndpointRef          string `json:"callback_endpoint_ref"`
		DeliveryQueueRef             string `json:"delivery_queue_ref"`
		CallbackPayloadSchemaVersion string `json:"callback_payload_schema_version"`
		CallbackPayloadRefHash       string `json:"callback_payload_ref_hash"`
	} `json:"callback_delivery_contract"`
	RetryContract struct {
		RetryPolicyRef           string `json:"retry_policy_ref"`
		MaxAttempts              int    `json:"max_attempts"`
		BackoffPolicyRef         string `json:"backoff_policy_ref"`
		CallbackTimeoutPolicyRef string `json:"callback_timeout_policy_ref"`
	} `json:"retry_contract"`
	NoDirectExecution    bool `json:"no_direct_execution"`
	NoDecisionRecorded   bool `json:"no_decision_recorded"`
	DoesNotCallProvider  bool `json:"does_not_call_provider"`
	DoesNotExecuteTarget bool `json:"does_not_execute_target"`
}

type externalCallbackRedrivePlan struct {
	SchemaVersion              string `json:"schema_version"`
	RedrivePlanID              string `json:"redrive_plan_id"`
	SourceDeliveryStatusSha256 string `json:"source_delivery_status_sha256"`
	SourceDeliveryPlanSha256   string `json:"source_delivery_plan_sha256"`
	WorkflowBinding            struct {
		WorkflowID                   string `json:"workflow_id"`
		StepID                       string `json:"step_id"`
		ExpectedWorkflowType         string `json:"expected_workflow_type"`
		ExpectedStatus               string `json:"expected_status"`
		ExpectedTargetService        string `json:"expected_target_service"`
		ExpectedTargetOperation      string `json:"expected_target_operation"`
		ExpectedTargetRefHash        string `json:"expected_target_ref_hash"`
		ExpectedPayloadSchemaVersion string `json:"expected_payload_schema_version"`
		ExpectedPayloadRefHash       string `json:"expected_payload_ref_hash"`
		ExpectedApprovalPolicyRef    string `json:"expected_approval_policy_ref"`
		DecisionPolicyRef            string `json:"decision_policy_ref"`
	} `json:"workflow_binding"`
	RedriveSource struct {
		DeliveryStatus           string `json:"delivery_status"`
		AttemptNumber            int    `json:"attempt_number"`
		MaxAttempts              int    `json:"max_attempts"`
		SourceDeliveryPlanSha256 string `json:"source_delivery_plan_sha256"`
		DeliveryAttemptRef       string `json:"delivery_attempt_ref"`
		FailureClassRef          string `json:"failure_class_ref"`
		RedrivePolicyRef         string `json:"redrive_policy_ref"`
	} `json:"redrive_source"`
	RedriveContract struct {
		Owner                      string `json:"owner"`
		RedriveQueueRef            string `json:"redrive_queue_ref"`
		RedriveReasonRef           string `json:"redrive_reason_ref"`
		OperatorReviewRef          string `json:"operator_review_ref"`
		RedrivePlanCallsProvider   bool   `json:"redrive_plan_calls_provider"`
		RedrivePlanRecordsDecision bool   `json:"redrive_plan_records_decision"`
		RedrivePlanExecutesTarget  bool   `json:"redrive_plan_executes_target"`
	} `json:"redrive_contract"`
	NoDirectExecution    bool `json:"no_direct_execution"`
	NoDecisionRecorded   bool `json:"no_decision_recorded"`
	DoesNotCallProvider  bool `json:"does_not_call_provider"`
	DoesNotExecuteTarget bool `json:"does_not_execute_target"`
}

func LoadExternalCallbackDeliveryPlan(path string, tenantID types.TenantID) (types.WorkflowExternalCallbackDelivery, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return types.WorkflowExternalCallbackDelivery{}, errors.New("workflow external callback delivery plan file is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return types.WorkflowExternalCallbackDelivery{}, err
	}
	var plan externalCallbackDeliveryPlan
	if err := json.Unmarshal(content, &plan); err != nil {
		return types.WorkflowExternalCallbackDelivery{}, err
	}
	if strings.TrimSpace(plan.SchemaVersion) != "nexusim.workflow.external_callback_delivery_plan.v1" {
		return types.WorkflowExternalCallbackDelivery{}, types.NewInvalidArgument("workflow external callback delivery plan schema is unsupported")
	}
	if !plan.NoDirectExecution || !plan.NoDecisionRecorded || !plan.DoesNotCallProvider || !plan.DoesNotExecuteTarget {
		return types.WorkflowExternalCallbackDelivery{}, types.NewInvalidArgument("workflow external callback delivery plan boundary flags are invalid")
	}
	if strings.ToUpper(strings.TrimSpace(plan.WorkflowBinding.ExpectedStatus)) != types.WorkflowStatusWaitingDecision {
		return types.WorkflowExternalCallbackDelivery{}, types.NewInvalidArgument("workflow external callback delivery plan must bind WAITING_DECISION")
	}
	sum := sha256.Sum256(content)
	delivery := types.WorkflowExternalCallbackDelivery{
		TenantID:                     tenantID,
		WorkflowID:                   plan.WorkflowBinding.WorkflowID,
		DeliveryID:                   plan.DeliveryPlanID,
		DeliveryPlanSha256:           "sha256:" + fmt.Sprintf("%x", sum[:]),
		SourceDecisionManifestSha256: plan.SourceDecisionManifestSha256,
		StepID:                       plan.WorkflowBinding.StepID,
		WorkflowType:                 plan.WorkflowBinding.ExpectedWorkflowType,
		TargetService:                plan.WorkflowBinding.ExpectedTargetService,
		TargetOperation:              plan.WorkflowBinding.ExpectedTargetOperation,
		TargetRefHash:                plan.WorkflowBinding.ExpectedTargetRefHash,
		PayloadSchemaVersion:         plan.WorkflowBinding.ExpectedPayloadSchemaVersion,
		PayloadRefHash:               plan.WorkflowBinding.ExpectedPayloadRefHash,
		ApprovalPolicyRef:            plan.WorkflowBinding.ExpectedApprovalPolicyRef,
		DecisionPolicyRef:            plan.WorkflowBinding.DecisionPolicyRef,
		CallbackProviderRef:          plan.CallbackDeliveryContract.CallbackProviderRef,
		CallbackEndpointRef:          plan.CallbackDeliveryContract.CallbackEndpointRef,
		DeliveryQueueRef:             plan.CallbackDeliveryContract.DeliveryQueueRef,
		RetryPolicyRef:               plan.RetryContract.RetryPolicyRef,
		BackoffPolicyRef:             plan.RetryContract.BackoffPolicyRef,
		CallbackTimeoutPolicyRef:     plan.RetryContract.CallbackTimeoutPolicyRef,
		CallbackPayloadSchemaVersion: plan.CallbackDeliveryContract.CallbackPayloadSchemaVersion,
		CallbackPayloadRefHash:       plan.CallbackDeliveryContract.CallbackPayloadRefHash,
		Status:                       types.WorkflowExternalCallbackDeliveryStatusPending,
		MaxAttempts:                  plan.RetryContract.MaxAttempts,
	}
	delivery = delivery.Normalized()
	if err := delivery.Validate(); err != nil {
		return types.WorkflowExternalCallbackDelivery{}, err
	}
	return delivery, nil
}

func LoadExternalCallbackRedrivePlan(path string, tenantID types.TenantID) (types.WorkflowExternalCallbackRedrivePlan, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return types.WorkflowExternalCallbackRedrivePlan{}, errors.New("workflow external callback redrive plan file is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return types.WorkflowExternalCallbackRedrivePlan{}, err
	}
	var plan externalCallbackRedrivePlan
	if err := json.Unmarshal(content, &plan); err != nil {
		return types.WorkflowExternalCallbackRedrivePlan{}, err
	}
	if strings.TrimSpace(plan.SchemaVersion) != "nexusim.workflow.external_callback_redrive_plan.v1" {
		return types.WorkflowExternalCallbackRedrivePlan{}, types.NewInvalidArgument("workflow external callback redrive plan schema is unsupported")
	}
	if !plan.NoDirectExecution || !plan.NoDecisionRecorded || !plan.DoesNotCallProvider || !plan.DoesNotExecuteTarget {
		return types.WorkflowExternalCallbackRedrivePlan{}, types.NewInvalidArgument("workflow external callback redrive plan boundary flags are invalid")
	}
	if plan.RedriveContract.RedrivePlanCallsProvider ||
		plan.RedriveContract.RedrivePlanRecordsDecision ||
		plan.RedriveContract.RedrivePlanExecutesTarget {
		return types.WorkflowExternalCallbackRedrivePlan{}, types.NewInvalidArgument("workflow external callback redrive contract is unsafe")
	}
	if strings.TrimSpace(plan.RedriveContract.Owner) != "workflow-service.external-callback-delivery" {
		return types.WorkflowExternalCallbackRedrivePlan{}, types.NewInvalidArgument("workflow external callback redrive owner is unsupported")
	}
	if strings.ToUpper(strings.TrimSpace(plan.WorkflowBinding.ExpectedStatus)) != types.WorkflowStatusWaitingDecision {
		return types.WorkflowExternalCallbackRedrivePlan{}, types.NewInvalidArgument("workflow external callback redrive plan must bind WAITING_DECISION")
	}
	sourceDeliveryPlanSha256 := strings.TrimSpace(plan.SourceDeliveryPlanSha256)
	if sourceDeliveryPlanSha256 == "" {
		sourceDeliveryPlanSha256 = strings.TrimSpace(plan.RedriveSource.SourceDeliveryPlanSha256)
	}
	if sourceDeliveryPlanSha256 == "" ||
		(strings.TrimSpace(plan.RedriveSource.SourceDeliveryPlanSha256) != "" &&
			strings.TrimSpace(plan.RedriveSource.SourceDeliveryPlanSha256) != sourceDeliveryPlanSha256) {
		return types.WorkflowExternalCallbackRedrivePlan{}, types.NewInvalidArgument("workflow external callback redrive source delivery plan hash mismatch")
	}
	sum := sha256.Sum256(content)
	redrive := types.WorkflowExternalCallbackRedrivePlan{
		TenantID:                   tenantID,
		RedrivePlanID:              plan.RedrivePlanID,
		RedrivePlanSha256:          "sha256:" + fmt.Sprintf("%x", sum[:]),
		SourceDeliveryStatusSha256: plan.SourceDeliveryStatusSha256,
		SourceDeliveryPlanSha256:   sourceDeliveryPlanSha256,
		WorkflowID:                 plan.WorkflowBinding.WorkflowID,
		StepID:                     plan.WorkflowBinding.StepID,
		WorkflowType:               plan.WorkflowBinding.ExpectedWorkflowType,
		TargetService:              plan.WorkflowBinding.ExpectedTargetService,
		TargetOperation:            plan.WorkflowBinding.ExpectedTargetOperation,
		TargetRefHash:              plan.WorkflowBinding.ExpectedTargetRefHash,
		PayloadSchemaVersion:       plan.WorkflowBinding.ExpectedPayloadSchemaVersion,
		PayloadRefHash:             plan.WorkflowBinding.ExpectedPayloadRefHash,
		ApprovalPolicyRef:          plan.WorkflowBinding.ExpectedApprovalPolicyRef,
		DecisionPolicyRef:          plan.WorkflowBinding.DecisionPolicyRef,
		DeliveryStatus:             plan.RedriveSource.DeliveryStatus,
		AttemptNumber:              plan.RedriveSource.AttemptNumber,
		MaxAttempts:                plan.RedriveSource.MaxAttempts,
		DeliveryAttemptRef:         plan.RedriveSource.DeliveryAttemptRef,
		FailureClassRef:            plan.RedriveSource.FailureClassRef,
		RedrivePolicyRef:           plan.RedriveSource.RedrivePolicyRef,
		RedriveQueueRef:            plan.RedriveContract.RedriveQueueRef,
		RedriveReasonRef:           plan.RedriveContract.RedriveReasonRef,
		OperatorReviewRef:          plan.RedriveContract.OperatorReviewRef,
	}
	redrive = redrive.Normalized()
	if err := redrive.Validate(); err != nil {
		return types.WorkflowExternalCallbackRedrivePlan{}, err
	}
	return redrive, nil
}

func LoadExternalCallbackEndpoints(path string) (map[string]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("workflow external callback endpoints file is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file ExternalCallbackEndpointFile
	if err := json.Unmarshal(content, &file); err != nil {
		return nil, err
	}
	endpoints := make(map[string]string, len(file.Endpoints))
	for _, endpoint := range file.Endpoints {
		ref := strings.TrimSpace(endpoint.Ref)
		url := strings.TrimSpace(endpoint.URL)
		if ref == "" || url == "" {
			return nil, types.NewInvalidArgument("workflow external callback endpoint is incomplete")
		}
		if _, exists := endpoints[ref]; exists {
			return nil, types.NewAlreadyExists("workflow external callback endpoint ref is duplicated")
		}
		endpoints[ref] = url
	}
	if len(endpoints) == 0 {
		return nil, types.NewInvalidArgument("workflow external callback endpoints file is empty")
	}
	return endpoints, nil
}

func NewExternalCallbackHTTPProvider(
	client *http.Client,
	endpoints map[string]string,
	timeout time.Duration,
) (ExternalCallbackHTTPProvider, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	if len(endpoints) == 0 {
		return ExternalCallbackHTTPProvider{}, errors.New("workflow external callback endpoints are required")
	}
	copied := make(map[string]string, len(endpoints))
	for ref, url := range endpoints {
		ref = strings.TrimSpace(ref)
		url = strings.TrimSpace(url)
		if ref == "" || url == "" {
			return ExternalCallbackHTTPProvider{}, errors.New("workflow external callback endpoint is incomplete")
		}
		copied[ref] = url
	}
	return ExternalCallbackHTTPProvider{client: client, endpoints: copied, timeout: timeout}, nil
}

func (provider ExternalCallbackHTTPProvider) DeliverExternalCallback(
	ctx context.Context,
	delivery types.WorkflowExternalCallbackDelivery,
) (types.WorkflowExternalCallbackDeliveryResult, error) {
	if provider.client == nil {
		return types.WorkflowExternalCallbackDeliveryResult{FailureClass: "provider_not_configured"}, types.NewUnavailable("workflow external callback provider is not configured")
	}
	endpointURL, ok := provider.endpoints[delivery.CallbackEndpointRef]
	if !ok {
		return types.WorkflowExternalCallbackDeliveryResult{FailureClass: "endpoint_ref_not_found"}, types.NewNotFound("workflow external callback endpoint ref not found")
	}
	body := map[string]any{
		"schema_version":                  "nexusim.workflow.external_callback.request.v1",
		"tenant_id":                       string(delivery.TenantID),
		"workflow_id":                     delivery.WorkflowID,
		"delivery_id":                     delivery.DeliveryID,
		"step_id":                         delivery.StepID,
		"workflow_type":                   delivery.WorkflowType,
		"target_service":                  delivery.TargetService,
		"target_operation":                delivery.TargetOperation,
		"target_ref_hash":                 delivery.TargetRefHash,
		"payload_schema_version":          delivery.PayloadSchemaVersion,
		"payload_ref_hash":                delivery.PayloadRefHash,
		"approval_policy_ref":             delivery.ApprovalPolicyRef,
		"decision_policy_ref":             delivery.DecisionPolicyRef,
		"callback_provider_ref":           delivery.CallbackProviderRef,
		"delivery_queue_ref":              delivery.DeliveryQueueRef,
		"callback_payload_schema_version": delivery.CallbackPayloadSchemaVersion,
		"callback_payload_ref_hash":       delivery.CallbackPayloadRefHash,
		"attempt_count":                   delivery.AttemptCount,
		"max_attempts":                    delivery.MaxAttempts,
		"boundary": map[string]bool{
			"records_decision":  false,
			"executes_target":   false,
			"contains_raw_url":  false,
			"contains_raw_body": false,
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return types.WorkflowExternalCallbackDeliveryResult{FailureClass: "request_encode_failed"}, types.NewInvalidArgument("workflow external callback request invalid")
	}
	callCtx, cancel := context.WithTimeout(ctx, provider.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpointURL, bytes.NewReader(encoded))
	if err != nil {
		return types.WorkflowExternalCallbackDeliveryResult{FailureClass: "request_build_failed"}, types.NewInvalidArgument("workflow external callback endpoint invalid")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-NexusIM-Workflow-ID", delivery.WorkflowID)
	request.Header.Set("X-NexusIM-Delivery-ID", delivery.DeliveryID)
	response, err := provider.client.Do(request)
	if err != nil {
		return types.WorkflowExternalCallbackDeliveryResult{FailureClass: "provider_unavailable"}, types.NewUnavailable("workflow external callback provider unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return types.WorkflowExternalCallbackDeliveryResult{FailureClass: fmt.Sprintf("provider_status_%d", response.StatusCode)}, types.NewUnavailable("workflow external callback provider rejected request")
	}
	return types.WorkflowExternalCallbackDeliveryResult{
		DeliveryResultRef: fmt.Sprintf("provider-status:%d", response.StatusCode),
	}, nil
}
