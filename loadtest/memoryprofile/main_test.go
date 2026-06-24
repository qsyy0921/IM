package main

import (
	"context"
	"strings"
	"testing"
	"time"

	memoryv1 "github.com/qsyy0921/IM/api/proto/nexusim/memory/v1"
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

func testConfig(execute bool) config {
	return config{
		memoryTarget:    "127.0.0.1:10580",
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

type fakeMemoryClient struct {
	memoryv1.MemoryServiceClient
	recomputeCalled  bool
	recomputeRequest *memoryv1.RecomputeProfileAggregateRequest
	response         *memoryv1.RecomputeProfileAggregateResponse
	err              error
}

func (client *fakeMemoryClient) RecomputeProfileAggregate(
	_ context.Context,
	request *memoryv1.RecomputeProfileAggregateRequest,
	_ ...grpc.CallOption,
) (*memoryv1.RecomputeProfileAggregateResponse, error) {
	client.recomputeCalled = true
	client.recomputeRequest = request
	if client.err != nil {
		return nil, client.err
	}
	return client.response, nil
}
