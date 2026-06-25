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
