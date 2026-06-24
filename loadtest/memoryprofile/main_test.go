package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	memoryv1 "github.com/qsyy0921/IM/api/proto/nexusim/memory/v1"
	workflowv1 "github.com/qsyy0921/IM/api/proto/nexusim/workflow/v1"
	"google.golang.org/grpc"
)

func TestParseArgsDefaultsSubjectToUser(t *testing.T) {
	cfg, err := parseArgs([]string{
		"--aggregate-key", "phoenix-launch",
		"--user-id", "user-1",
	})
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if cfg.subjectUserID != "user-1" {
		t.Fatalf("subjectUserID = %q, want user-1", cfg.subjectUserID)
	}
	if cfg.execute {
		t.Fatal("operator must default to plan-only")
	}
}

func TestParseArgsRejectsCrossUserUntilPolicyOperatorExists(t *testing.T) {
	_, err := parseArgs([]string{
		"--aggregate-key", "phoenix-launch",
		"--user-id", "user-1",
		"--subject-user-id", "user-2",
	})
	if err == nil || !strings.Contains(err.Error(), "must match") {
		t.Fatalf("expected cross-user validation error, got %v", err)
	}
}

func TestExecutePlanOnlyDoesNotCallMemoryService(t *testing.T) {
	client := &fakeMemoryClient{}
	result, err := execute(context.Background(), testConfig(false), client)
	if err != nil {
		t.Fatalf("execute plan: %v", err)
	}
	if !result.Success || result.Executed {
		t.Fatalf("unexpected plan result: %+v", result)
	}
	if client.recomputeCalled {
		t.Fatal("plan-only operator must not call memory-service")
	}
}

func TestExecuteRecomputeRedactsProfileDetails(t *testing.T) {
	cfg := testConfig(true)
	client := &fakeMemoryClient{
		response: &memoryv1.RecomputeProfileAggregateResponse{
			Active:       true,
			SupportCount: 2,
			Item: &memoryv1.ProfileAggregate{
				ProfileId:                "profile-1",
				AggregateType:            cfg.aggregateType,
				AggregateKey:             cfg.aggregateKey,
				Status:                   memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE,
				ReviewState:              memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_APPROVED,
				SummaryText:              "raw profile summary must not be written",
				SupportingMemoryEventIds: []string{"mem-1", "mem-2"},
				UpdatedAtUnixMs:          1234,
			},
		},
	}
	result, err := execute(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("execute recompute: %v", err)
	}
	if !client.recomputeCalled {
		t.Fatal("expected recompute RPC to be called")
	}
	if !result.Success || !result.Executed || !result.Active || result.SupportCount != 2 {
		t.Fatalf("unexpected execute result: %+v", result)
	}
	if result.SummaryTextSHA256 == "" || result.SummaryTextLength == 0 {
		t.Fatalf("summary hash/length should be recorded: %+v", result)
	}
	if len(result.SupportingMemoryIDHashes) != 2 {
		t.Fatalf("support ids should be hashed: %+v", result.SupportingMemoryIDHashes)
	}
	if result.SupportingMemoryIDHashes[0] == "mem-1" || result.SummaryTextSHA256 == "raw profile summary must not be written" {
		t.Fatalf("raw profile details leaked into summary: %+v", result)
	}
	request := client.recomputeRequest
	if request.GetAuthContext().GetUserId() != cfg.userID ||
		request.GetSubjectUserId() != cfg.subjectUserID ||
		request.GetAggregateType() != cfg.aggregateType ||
		request.GetAggregateKey() != cfg.aggregateKey ||
		request.GetMinSupportCount() != int32(cfg.minSupportCount) {
		t.Fatalf("unexpected recompute request: %+v", request)
	}
}

func TestParseArgsRequiresApprovalForBatchExecute(t *testing.T) {
	_, err := parseArgs([]string{
		"--batch-file", "batch.json",
		"--user-id", "user-1",
		"--execute",
	})
	if err == nil || !strings.Contains(err.Error(), "requires --approval-workflow-id") {
		t.Fatalf("expected approval workflow validation error, got %v", err)
	}
}

func TestBatchPlanOnlyRedactsTargetsAndDoesNotCallServices(t *testing.T) {
	cfg := testConfig(false)
	cfg.batchFile = writeBatchManifest(t, []repairTarget{{
		SubjectUserID:   cfg.userID,
		AggregateType:   "SKILL",
		AggregateKey:    "phoenix-launch",
		MinSupportCount: 2,
	}, {
		SubjectUserID:   cfg.userID,
		AggregateType:   "ROLE",
		AggregateKey:    "incident-commander",
		MinSupportCount: 3,
	}})
	client := &fakeMemoryClient{}
	result, err := executeWithClients(context.Background(), cfg, client, nil)
	if err != nil {
		t.Fatalf("execute batch plan: %v", err)
	}
	if !result.Success || result.Executed || !result.BatchMode || result.BatchTargetCount != 2 {
		t.Fatalf("unexpected plan result: %+v", result)
	}
	if client.recomputeCalled {
		t.Fatal("batch plan-only operator must not call memory-service")
	}
	encoded, _ := json.Marshal(result)
	output := string(encoded)
	for _, forbidden := range []string{"phoenix-launch", "incident-commander"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("plan leaked raw aggregate key %q: %s", forbidden, output)
		}
	}
}

func TestBatchExecuteRequiresApprovedWorkflowBeforeMemoryCall(t *testing.T) {
	cfg := testConfig(true)
	cfg.batchFile = writeBatchManifest(t, []repairTarget{{
		SubjectUserID: cfg.userID,
		AggregateType: "SKILL",
		AggregateKey:  "phoenix-launch",
	}})
	cfg.approvalWorkflowID = "wf_repair_1"
	plan, err := buildRepairPlan(cfg)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	memoryClient := &fakeMemoryClient{}
	workflowClient := &fakeWorkflowClient{getResponse: workflowResponse(cfg, plan, "WAITING_DECISION")}
	result, err := executeWithClients(context.Background(), cfg, memoryClient, workflowClient)
	if err == nil || !strings.Contains(err.Error(), "must be APPROVED") {
		t.Fatalf("expected approval failure, got result=%+v err=%v", result, err)
	}
	if memoryClient.recomputeCalled {
		t.Fatal("memory-service must not be called before approved workflow is verified")
	}
}

func TestBatchExecuteRejectsApprovalHashMismatch(t *testing.T) {
	cfg := testConfig(true)
	cfg.batchFile = writeBatchManifest(t, []repairTarget{{
		SubjectUserID: cfg.userID,
		AggregateType: "SKILL",
		AggregateKey:  "phoenix-launch",
	}})
	cfg.approvalWorkflowID = "wf_repair_1"
	plan, err := buildRepairPlan(cfg)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	response := workflowResponse(cfg, plan, "APPROVED")
	response.Workflow.PayloadRefHash = "sha256:mismatch"
	memoryClient := &fakeMemoryClient{}
	workflowClient := &fakeWorkflowClient{getResponse: response}
	result, err := executeWithClients(context.Background(), cfg, memoryClient, workflowClient)
	if err == nil || !strings.Contains(err.Error(), "payload hash mismatch") {
		t.Fatalf("expected hash mismatch failure, got result=%+v err=%v", result, err)
	}
	if memoryClient.recomputeCalled {
		t.Fatal("memory-service must not be called when approval hash mismatches")
	}
}

func TestBatchExecuteWithApprovedWorkflowRecomputesEveryTarget(t *testing.T) {
	cfg := testConfig(true)
	cfg.batchFile = writeBatchManifest(t, []repairTarget{{
		SubjectUserID:   cfg.userID,
		AggregateType:   "SKILL",
		AggregateKey:    "phoenix-launch",
		MinSupportCount: 2,
	}, {
		SubjectUserID:   cfg.userID,
		AggregateType:   "ROLE",
		AggregateKey:    "incident-commander",
		MinSupportCount: 3,
	}})
	cfg.approvalWorkflowID = "wf_repair_1"
	plan, err := buildRepairPlan(cfg)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	memoryClient := &fakeMemoryClient{
		responses: []*memoryv1.RecomputeProfileAggregateResponse{{
			Active:       true,
			SupportCount: 2,
			Item: &memoryv1.ProfileAggregate{
				ProfileId:                "profile-skill",
				Status:                   memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE,
				ReviewState:              memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_APPROVED,
				SummaryText:              "raw skill profile must not leak",
				SupportingMemoryEventIds: []string{"mem-a", "mem-b"},
				UpdatedAtUnixMs:          1000,
			},
		}, {
			Active:       true,
			SupportCount: 3,
			Item: &memoryv1.ProfileAggregate{
				ProfileId:                "profile-role",
				Status:                   memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE,
				ReviewState:              memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_APPROVED,
				SummaryText:              "raw role profile must not leak",
				SupportingMemoryEventIds: []string{"mem-c", "mem-d", "mem-e"},
				UpdatedAtUnixMs:          2000,
			},
		}},
	}
	workflowClient := &fakeWorkflowClient{getResponse: workflowResponse(cfg, plan, "APPROVED")}
	result, err := executeWithClients(context.Background(), cfg, memoryClient, workflowClient)
	if err != nil {
		t.Fatalf("execute approved batch: %v", err)
	}
	if !result.Success || !result.Executed || !result.ApprovalVerified || len(result.Targets) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(memoryClient.recomputeRequests) != 2 {
		t.Fatalf("expected two recompute requests, got %d", len(memoryClient.recomputeRequests))
	}
	if memoryClient.recomputeRequests[0].GetAggregateKey() != "phoenix-launch" ||
		memoryClient.recomputeRequests[1].GetAggregateKey() != "incident-commander" {
		t.Fatalf("unexpected recompute requests: %+v", memoryClient.recomputeRequests)
	}
	encoded, _ := json.Marshal(result)
	output := string(encoded)
	for _, forbidden := range []string{"phoenix-launch", "incident-commander", "raw skill profile", "raw role profile", "mem-a"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("batch summary leaked raw value %q: %s", forbidden, output)
		}
	}
}

func TestRequestApprovalCreatesRepairWorkflowWithoutMemoryCall(t *testing.T) {
	cfg := testConfig(false)
	cfg.batchFile = writeBatchManifest(t, []repairTarget{{
		SubjectUserID: cfg.userID,
		AggregateType: "SKILL",
		AggregateKey:  "phoenix-launch",
	}})
	cfg.requestApproval = true
	plan, err := buildRepairPlan(cfg)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	workflow := workflowResponse(cfg, plan, "WAITING_DECISION").GetWorkflow()
	workflow.WorkflowId = "wf_created_1"
	memoryClient := &fakeMemoryClient{}
	workflowClient := &fakeWorkflowClient{createResponse: &workflowv1.CreateWorkflowResponse{
		Workflow: workflow,
	}}
	result, err := executeWithClients(context.Background(), cfg, memoryClient, workflowClient)
	if err != nil {
		t.Fatalf("request approval: %v", err)
	}
	if !result.Success || result.Executed || !result.ApprovalRequested ||
		result.ApprovalWorkflowID != "wf_created_1" ||
		result.ApprovalWorkflowStatus != "WAITING_DECISION" {
		t.Fatalf("unexpected approval request result: %+v", result)
	}
	if memoryClient.recomputeCalled {
		t.Fatal("request-approval must not call memory-service")
	}
	request := workflowClient.createRequest
	if request.GetWorkflowType() != approvalWorkflowType ||
		request.GetTargetService() != approvalTargetService ||
		request.GetTargetOperation() != approvalTargetOperationBatch ||
		request.GetPayloadRefHash() != plan.PayloadRefHash ||
		request.GetTargetRefHash() != plan.TargetRefHash {
		t.Fatalf("unexpected create workflow request: %+v", request)
	}
}

func testConfig(execute bool) config {
	return config{
		memoryTarget:    "127.0.0.1:10580",
		workflowTarget:  "127.0.0.1:10750",
		tenantID:        "tenant-1",
		userID:          "user-1",
		deviceID:        "device-1",
		subjectUserID:   "user-1",
		aggregateType:   "SKILL",
		aggregateKey:    "phoenix-launch",
		minSupportCount: 2,
		requestTimeout:  time.Second,
		resultRoot:      defaultResultRoot,
		runName:         "test-run",
		execute:         execute,
	}
}

func writeBatchManifest(t *testing.T, targets []repairTarget) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	raw, err := json.Marshal(batchManifest{
		SchemaVersion: batchManifestSchemaVersion,
		Targets:       targets,
	})
	if err != nil {
		t.Fatalf("marshal batch: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write batch: %v", err)
	}
	return path
}

func workflowResponse(cfg config, plan repairPlan, status string) *workflowv1.GetWorkflowResponse {
	return &workflowv1.GetWorkflowResponse{Workflow: &workflowv1.Workflow{
		TenantId:             cfg.tenantID,
		WorkflowId:           cfg.approvalWorkflowID,
		WorkflowType:         approvalWorkflowType,
		Status:               status,
		TargetService:        approvalTargetService,
		TargetOperation:      approvalTargetOperationBatch,
		PayloadSchemaVersion: batchManifestSchemaVersion,
		PayloadRefHash:       plan.PayloadRefHash,
		TargetRefHash:        plan.TargetRefHash,
	}}
}

type fakeMemoryClient struct {
	memoryv1.MemoryServiceClient
	recomputeCalled   bool
	recomputeRequest  *memoryv1.RecomputeProfileAggregateRequest
	recomputeRequests []*memoryv1.RecomputeProfileAggregateRequest
	response          *memoryv1.RecomputeProfileAggregateResponse
	responses         []*memoryv1.RecomputeProfileAggregateResponse
	err               error
}

func (client *fakeMemoryClient) RecomputeProfileAggregate(
	_ context.Context,
	request *memoryv1.RecomputeProfileAggregateRequest,
	_ ...grpc.CallOption,
) (*memoryv1.RecomputeProfileAggregateResponse, error) {
	client.recomputeCalled = true
	client.recomputeRequest = request
	client.recomputeRequests = append(client.recomputeRequests, request)
	if client.err != nil {
		return nil, client.err
	}
	if len(client.responses) > 0 {
		response := client.responses[0]
		client.responses = client.responses[1:]
		return response, nil
	}
	return client.response, nil
}

type fakeWorkflowClient struct {
	workflowv1.WorkflowServiceClient
	createRequest  *workflowv1.CreateWorkflowRequest
	createResponse *workflowv1.CreateWorkflowResponse
	getRequest     *workflowv1.GetWorkflowRequest
	getResponse    *workflowv1.GetWorkflowResponse
	err            error
}

func (client *fakeWorkflowClient) CreateWorkflow(
	_ context.Context,
	request *workflowv1.CreateWorkflowRequest,
	_ ...grpc.CallOption,
) (*workflowv1.CreateWorkflowResponse, error) {
	client.createRequest = request
	if client.err != nil {
		return nil, client.err
	}
	return client.createResponse, nil
}

func (client *fakeWorkflowClient) GetWorkflow(
	_ context.Context,
	request *workflowv1.GetWorkflowRequest,
	_ ...grpc.CallOption,
) (*workflowv1.GetWorkflowResponse, error) {
	client.getRequest = request
	if client.err != nil {
		return nil, client.err
	}
	return client.getResponse, nil
}
