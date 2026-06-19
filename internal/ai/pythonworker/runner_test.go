package pythonworker

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestRunnerConsumesCandidateFromWorker(t *testing.T) {
	runner := newFakeRunner(t, "success")

	candidate, err := runner.Run(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if candidate.Status != StatusCandidate {
		t.Fatalf("candidate.Status = %q", candidate.Status)
	}
	if candidate.OutputSHA256 == "" {
		t.Fatal("candidate.OutputSHA256 is empty")
	}
}

func TestRunnerReturnsFailedCandidate(t *testing.T) {
	runner := newFakeRunner(t, "failed")

	candidate, err := runner.Run(context.Background(), validRequest())
	if !errors.Is(err, ErrWorkerFailed) {
		t.Fatalf("Run() error = %v, want ErrWorkerFailed", err)
	}
	if candidate.Status != StatusFailed || candidate.ErrorClass != "UNSAFE_INPUT" {
		t.Fatalf("unexpected candidate: %+v", candidate)
	}
}

func TestRunnerRejectsUnsafeRequestBeforeProcess(t *testing.T) {
	runner := newFakeRunner(t, "success")
	request := validRequest()
	request.CandidateText = "sk-secret-token-value"

	_, err := runner.Run(context.Background(), request)
	if !errors.Is(err, ErrUnsafeRequest) {
		t.Fatalf("Run() error = %v, want ErrUnsafeRequest", err)
	}
}

func TestRunnerRejectsMalformedCandidateOutput(t *testing.T) {
	runner := newFakeRunner(t, "malformed")

	_, err := runner.Run(context.Background(), validRequest())
	if !errors.Is(err, ErrMalformedCandidate) {
		t.Fatalf("Run() error = %v, want ErrMalformedCandidate", err)
	}
}

func TestRunnerRejectsForbiddenRawOutputCandidate(t *testing.T) {
	runner := newFakeRunner(t, "raw_output")

	_, err := runner.Run(context.Background(), validRequest())
	if !errors.Is(err, ErrUnsafeCandidate) {
		t.Fatalf("Run() error = %v, want ErrUnsafeCandidate", err)
	}
}

func TestRunnerRejectsSensitiveCitationCandidate(t *testing.T) {
	runner := newFakeRunner(t, "sensitive_citation")

	_, err := runner.Run(context.Background(), validRequest())
	if !errors.Is(err, ErrUnsafeCandidate) {
		t.Fatalf("Run() error = %v, want ErrUnsafeCandidate", err)
	}
}

func TestRunnerRejectsMalformedHashCandidate(t *testing.T) {
	runner := newFakeRunner(t, "bad_hash")

	_, err := runner.Run(context.Background(), validRequest())
	if !errors.Is(err, ErrMalformedCandidate) {
		t.Fatalf("Run() error = %v, want ErrMalformedCandidate", err)
	}
}

func newFakeRunner(t *testing.T, mode string) Runner {
	t.Helper()
	runner, err := NewRunner(RunnerOptions{
		Python:     os.Args[0],
		ScriptPath: "__fake_python_worker__",
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	t.Setenv("NEXUSIM_FAKE_PYTHON_WORKER_MODE", mode)
	return runner
}

func TestMain(m *testing.M) {
	if len(os.Args) >= 2 && os.Args[1] == "__fake_python_worker__" {
		runFakeWorker()
		return
	}
	os.Exit(m.Run())
}

func runFakeWorker() {
	switch os.Getenv("NEXUSIM_FAKE_PYTHON_WORKER_MODE") {
	case "success":
		_, _ = os.Stdout.WriteString(`{
			"schema_version":1,
			"task_id":"task_01",
			"candidate_id":"cand_01",
			"worker_kind":"MEMORY_EXTRACTION",
			"status":"CANDIDATE",
			"output_type":"MEMORY_EVENT_CANDIDATE",
			"output_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"source_refs":["message:tenant:conversation:seq1"],
			"citations":["message:tenant:conversation:seq1"],
			"safety_flags":["LOW_SENSITIVE"],
			"confidence":0.9
		}`)
		os.Exit(0)
	case "failed":
		_, _ = os.Stdout.WriteString(`{
			"schema_version":1,
			"task_id":"task_01",
			"candidate_id":"cand_01",
			"worker_kind":"EVAL",
			"status":"FAILED",
			"output_type":"EVAL_RESULT",
			"output_sha256":"",
			"source_refs":[],
			"citations":[],
			"safety_flags":["UNSAFE_INPUT"],
			"error_class":"UNSAFE_INPUT"
		}`)
		os.Exit(2)
	case "malformed":
		_, _ = os.Stdout.WriteString(`{"schema_version":1,"status":"CANDIDATE"}`)
		os.Exit(0)
	case "raw_output":
		_, _ = os.Stdout.WriteString(`{
			"schema_version":1,
			"task_id":"task_01",
			"candidate_id":"cand_01",
			"worker_kind":"MEMORY_EXTRACTION",
			"status":"CANDIDATE",
			"output_type":"MEMORY_EVENT_CANDIDATE",
			"output_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"source_refs":["message:tenant:conversation:seq1"],
			"citations":["message:tenant:conversation:seq1"],
			"safety_flags":["LOW_SENSITIVE"],
			"raw_output":"candidate body must not cross into Go"
		}`)
		os.Exit(0)
	case "sensitive_citation":
		_, _ = os.Stdout.WriteString(`{
			"schema_version":1,
			"task_id":"task_01",
			"candidate_id":"cand_01",
			"worker_kind":"MEMORY_EXTRACTION",
			"status":"CANDIDATE",
			"output_type":"MEMORY_EVENT_CANDIDATE",
			"output_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"source_refs":["message:tenant:conversation:seq1"],
			"citations":["Bearer secret-token-value"],
			"safety_flags":["LOW_SENSITIVE"]
		}`)
		os.Exit(0)
	case "bad_hash":
		_, _ = os.Stdout.WriteString(`{
			"schema_version":1,
			"task_id":"task_01",
			"candidate_id":"cand_01",
			"worker_kind":"MEMORY_EXTRACTION",
			"status":"CANDIDATE",
			"output_type":"MEMORY_EVENT_CANDIDATE",
			"output_sha256":"ABC",
			"source_refs":["message:tenant:conversation:seq1"],
			"citations":["message:tenant:conversation:seq1"],
			"safety_flags":["LOW_SENSITIVE"]
		}`)
		os.Exit(0)
	default:
		os.Exit(1)
	}
}
