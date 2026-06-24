package memorycandidate

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestRunnerConsumesMemoryExtractionResult(t *testing.T) {
	runner := newFakeRunner(t, "success")

	result, err := runner.Run(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != StatusCompleted {
		t.Fatalf("result.Status = %q", result.Status)
	}
	if result.CandidateCount != 2 {
		t.Fatalf("result.CandidateCount = %d", result.CandidateCount)
	}
	if result.Report.RawTextReturned {
		t.Fatal("result returned raw text")
	}
	if result.Report.FinalMemoryPersisted {
		t.Fatal("result claimed final memory persistence")
	}
	if !result.Report.RequiresGoValidation {
		t.Fatal("result did not require Go validation")
	}
	var profile Candidate
	for _, candidate := range result.Candidates {
		if candidate.MemoryEventType == "PROFILE_SIGNAL" {
			profile = candidate
		}
	}
	if profile.CandidateID == "" {
		t.Fatal("PROFILE_SIGNAL candidate not found")
	}
	if profile.ReviewState != "NEEDS_REVIEW" || !hasFlag(profile.SafetyFlags, "GROUP_SCOPE_PROFILE_SIGNAL") {
		t.Fatalf("profile candidate does not require review: %+v", profile)
	}
}

func TestRunnerKeepsOrdinaryChatAsZeroCandidates(t *testing.T) {
	runner := newFakeRunner(t, "ordinary")

	result, err := runner.Run(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != StatusCompleted {
		t.Fatalf("result.Status = %q", result.Status)
	}
	if result.CandidateCount != 0 || len(result.Candidates) != 0 {
		t.Fatalf("ordinary result produced candidates: %+v", result)
	}
	if result.OrdinaryMessageCount != 1 {
		t.Fatalf("result.OrdinaryMessageCount = %d", result.OrdinaryMessageCount)
	}
}

func TestRunnerReturnsFailedResult(t *testing.T) {
	runner := newFakeRunner(t, "failed")

	result, err := runner.Run(context.Background(), validRequest())
	if !errors.Is(err, ErrWorkerFailed) {
		t.Fatalf("Run() error = %v, want ErrWorkerFailed", err)
	}
	if result.Status != StatusFailed || result.ErrorClass != "UNSAFE_INPUT" {
		t.Fatalf("unexpected failed result: %+v", result)
	}
	if result.CandidateCount != 0 || len(result.Candidates) != 0 {
		t.Fatalf("failed result contains candidates: %+v", result)
	}
}

func TestRunnerRejectsUnsafeRequestBeforeProcess(t *testing.T) {
	runner := newFakeRunner(t, "success")
	request := validRequest()
	request.Messages[0].Text = "Bearer secret-token-value"

	_, err := runner.Run(context.Background(), request)
	if !errors.Is(err, ErrUnsafeRequest) {
		t.Fatalf("Run() error = %v, want ErrUnsafeRequest", err)
	}
}

func TestDecodeResultRejectsRawTextOutput(t *testing.T) {
	_, err := DecodeResult([]byte(`{
		"schema_version":1,
		"task_id":"memory_task",
		"extractor_version":"memory-extraction-candidate-v1",
		"status":"COMPLETED",
		"message_count":1,
		"candidate_count":0,
		"ordinary_message_count":1,
		"candidates":[],
		"raw_text":"decision: must not cross into Go",
		"report":{
			"schema_version":1,
			"scope":"low-sensitive memory extraction candidate report",
			"message_count":1,
			"candidate_count":0,
			"ordinary_message_count":1,
			"raw_text_returned":false,
			"final_memory_persisted":false,
			"requires_go_validation":true
		}
	}`))
	if !errors.Is(err, ErrUnsafeResult) {
		t.Fatalf("DecodeResult() error = %v, want ErrUnsafeResult", err)
	}
}

func TestDecodeResultRejectsProfileSignalWithoutReview(t *testing.T) {
	payload := successPayload(`"review_state":"CANDIDATE_ONLY","safety_flags":["LOW_SENSITIVE","GO_VALIDATION_REQUIRED"]`)
	_, err := DecodeResult([]byte(payload))
	if !errors.Is(err, ErrMalformedResult) {
		t.Fatalf("DecodeResult() error = %v, want ErrMalformedResult", err)
	}
}

func newFakeRunner(t *testing.T, mode string) Runner {
	t.Helper()
	runner, err := NewRunner(RunnerOptions{
		Python:     os.Args[0],
		ScriptPath: "__fake_memory_candidate_worker__",
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	t.Setenv("NEXUSIM_FAKE_MEMORY_CANDIDATE_MODE", mode)
	return runner
}

func validRequest() Request {
	return Request{
		SchemaVersion:  1,
		TaskID:         "memory_task",
		TenantID:       "tenant-a",
		ConversationID: "conv-alpha",
		Messages: []Message{
			{
				MessageID:       "msg-1",
				ConversationSeq: 7,
				SpeakerID:       "user-a",
				Text:            "decision: keep memory candidates source-backed",
			},
		},
	}
}

func TestMain(m *testing.M) {
	if len(os.Args) >= 2 && os.Args[1] == "__fake_memory_candidate_worker__" {
		runFakeWorker()
		return
	}
	os.Exit(m.Run())
}

func runFakeWorker() {
	switch os.Getenv("NEXUSIM_FAKE_MEMORY_CANDIDATE_MODE") {
	case "success":
		_, _ = os.Stdout.WriteString(successPayload(`"review_state":"NEEDS_REVIEW","safety_flags":["LOW_SENSITIVE","GO_VALIDATION_REQUIRED","NEEDS_REVIEW","GROUP_SCOPE_PROFILE_SIGNAL"]`))
		os.Exit(0)
	case "ordinary":
		_, _ = os.Stdout.WriteString(`{
			"schema_version":1,
			"task_id":"memory_task",
			"extractor_version":"memory-extraction-candidate-v1",
			"status":"COMPLETED",
			"tenant_id_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"conversation_id_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"message_count":1,
			"candidate_count":0,
			"ordinary_message_count":1,
			"candidates":[],
			"report":{
				"schema_version":1,
				"scope":"low-sensitive memory extraction candidate report",
				"message_count":1,
				"candidate_count":0,
				"ordinary_message_count":1,
				"event_type_counts":{},
				"candidate_hashes":[],
				"raw_text_returned":false,
				"final_memory_persisted":false,
				"requires_go_validation":true,
				"requires_review_for_profile_signal":true
			}
		}`)
		os.Exit(0)
	case "failed":
		_, _ = os.Stdout.WriteString(`{
			"schema_version":1,
			"task_id":"memory_task",
			"extractor_version":"memory-extraction-candidate-v1",
			"status":"FAILED",
			"error_class":"UNSAFE_INPUT",
			"candidate_count":0,
			"candidates":[],
			"report":{
				"schema_version":1,
				"scope":"low-sensitive memory extraction candidate report",
				"candidate_count":0,
				"raw_text_returned":false,
				"final_memory_persisted":false,
				"requires_go_validation":true
			}
		}`)
		os.Exit(2)
	default:
		os.Exit(1)
	}
}

func successPayload(profileFields string) string {
	return `{
		"schema_version":1,
		"task_id":"memory_task",
		"extractor_version":"memory-extraction-candidate-v1",
		"status":"COMPLETED",
		"tenant_id_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"conversation_id_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"message_count":2,
		"candidate_count":2,
		"ordinary_message_count":0,
		"candidates":[
			{
				"schema_version":1,
				"task_id":"memory_task",
				"candidate_id":"memcand_decision",
				"worker_kind":"MEMORY_EXTRACTION",
				"status":"CANDIDATE",
				"output_type":"MEMORY_EVENT_CANDIDATE",
				"output_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"source_refs":["message:tenant:conversation:seq7"],
				"citations":["message:tenant:conversation:seq7"],
				"safety_flags":["LOW_SENSITIVE","GO_VALIDATION_REQUIRED","CANDIDATE_ONLY"],
				"confidence":0.78,
				"memory_event_type":"DECISION",
				"review_state":"CANDIDATE_ONLY",
				"fact_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				"speaker_id_hash":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				"message_id_hash":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
				"conversation_seq":7,
				"source_ref_count":1
			},
			{
				"schema_version":1,
				"task_id":"memory_task",
				"candidate_id":"memcand_profile",
				"worker_kind":"MEMORY_EXTRACTION",
				"status":"CANDIDATE",
				"output_type":"MEMORY_EVENT_CANDIDATE",
				"output_sha256":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
				"source_refs":["message:tenant:conversation:seq8"],
				"citations":["message:tenant:conversation:seq8"],
				"confidence":0.55,
				"memory_event_type":"PROFILE_SIGNAL",
				` + profileFields + `,
				"fact_sha256":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
				"speaker_id_hash":"1111111111111111111111111111111111111111111111111111111111111111",
				"message_id_hash":"2222222222222222222222222222222222222222222222222222222222222222",
				"conversation_seq":8,
				"source_ref_count":1
			}
		],
		"report":{
			"schema_version":1,
			"scope":"low-sensitive memory extraction candidate report",
			"message_count":2,
			"candidate_count":2,
			"ordinary_message_count":0,
			"event_type_counts":{"DECISION":1,"PROFILE_SIGNAL":1},
			"candidate_hashes":[
				"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
			],
			"raw_text_returned":false,
			"final_memory_persisted":false,
			"requires_go_validation":true,
			"requires_review_for_profile_signal":true
		}
	}`
}
