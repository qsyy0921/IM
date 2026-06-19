package pythonworker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const DefaultMaxOutputBytes int64 = 64 * 1024

var (
	ErrWorkerUnavailable = errors.New("python worker unavailable")
	ErrWorkerFailed      = errors.New("python worker returned failed candidate")
)

type RunnerOptions struct {
	Python         string
	ScriptPath     string
	WorkDir        string
	Timeout        time.Duration
	MaxOutputBytes int64
}

type Runner struct {
	python         string
	scriptPath     string
	workDir        string
	timeout        time.Duration
	maxOutputBytes int64
}

func NewRunner(options RunnerOptions) (Runner, error) {
	python := required(options.Python)
	if python == "" {
		python = "python"
	}
	scriptPath := required(options.ScriptPath)
	if scriptPath == "" {
		return Runner{}, errors.New("python worker script path is required")
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	maxOutputBytes := options.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = DefaultMaxOutputBytes
	}
	return Runner{
		python:         python,
		scriptPath:     scriptPath,
		workDir:        options.WorkDir,
		timeout:        timeout,
		maxOutputBytes: maxOutputBytes,
	}, nil
}

func (runner Runner) Run(ctx context.Context, request Request) (Candidate, error) {
	requestPayload, err := MarshalRequest(request)
	if err != nil {
		return Candidate{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, runner.timeout)
	defer cancel()

	tempDir, err := os.MkdirTemp("", "nexusim-pythonworker-*")
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: create temp dir", ErrWorkerUnavailable)
	}
	defer os.RemoveAll(tempDir)

	requestPath := filepath.Join(tempDir, "request.json")
	if err := os.WriteFile(requestPath, requestPayload, 0o600); err != nil {
		return Candidate{}, fmt.Errorf("%w: write request", ErrWorkerUnavailable)
	}

	command := exec.CommandContext(ctx, runner.python, runner.scriptPath, requestPath)
	if runner.workDir != "" {
		command.Dir = runner.workDir
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if ctx.Err() != nil {
		return Candidate{}, fmt.Errorf("%w: timeout", ErrWorkerUnavailable)
	}
	if int64(len(output)) > runner.maxOutputBytes {
		return Candidate{}, fmt.Errorf("%w: output too large", ErrMalformedCandidate)
	}
	if err != nil && len(output) == 0 {
		return Candidate{}, fmt.Errorf("%w: %s", ErrWorkerUnavailable, stderr.String())
	}
	candidate, decodeErr := DecodeCandidate(limitBytes(output, runner.maxOutputBytes))
	if decodeErr != nil {
		return Candidate{}, decodeErr
	}
	if err != nil || candidate.Status == StatusFailed {
		return candidate, ErrWorkerFailed
	}
	return candidate, nil
}

func limitBytes(payload []byte, maxBytes int64) []byte {
	if maxBytes <= 0 || int64(len(payload)) <= maxBytes {
		return payload
	}
	reader := io.LimitReader(bytes.NewReader(payload), maxBytes)
	limited, _ := io.ReadAll(reader)
	return limited
}
