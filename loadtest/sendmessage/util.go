package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func writeSummary(resultDir string, result *summary) error {
	if err := os.MkdirAll(resultDir, 0755); err != nil {
		return err
	}
	result.ResultFile = filepath.Join(resultDir, "sendmessage-summary.json")
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(result.ResultFile, encoded, 0644)
}

type commitInfo struct {
	Short       string
	Full        string
	Dirty       bool
	StatusShort string
}

func currentCommit() commitInfo {
	if override := commitInfoFromEnv(); override.Short != "" {
		return override
	}
	shortOutput, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return commitInfo{Short: "unknown", Full: "unknown"}
	}
	fullOutput, err := exec.Command("git", "rev-parse", "HEAD").Output()
	full := "unknown"
	if err == nil {
		full = strings.TrimSpace(string(fullOutput))
	}
	statusOutput, err := exec.Command("git", "status", "--short").Output()
	statusShort := ""
	if err == nil {
		statusShort = strings.TrimSpace(string(statusOutput))
	}
	short := strings.TrimSpace(string(shortOutput))
	dirty := statusShort != ""
	if dirty {
		short += "-dirty"
	}
	return commitInfo{
		Short:       short,
		Full:        full,
		Dirty:       dirty,
		StatusShort: statusShort,
	}
}

func commitInfoFromEnv() commitInfo {
	short := strings.TrimSpace(os.Getenv("NEXUSIM_COMMIT"))
	if short == "" {
		return commitInfo{}
	}
	full := strings.TrimSpace(os.Getenv("NEXUSIM_COMMIT_FULL"))
	if full == "" {
		full = short
	}
	statusShort := strings.TrimSpace(os.Getenv("NEXUSIM_GIT_STATUS_SHORT"))
	dirty, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("NEXUSIM_GIT_DIRTY")))
	if statusShort != "" {
		dirty = true
	}
	if dirty && !strings.HasSuffix(short, "-dirty") {
		short += "-dirty"
	}
	return commitInfo{
		Short:       short,
		Full:        full,
		Dirty:       dirty,
		StatusShort: statusShort,
	}
}

func durationMS(value time.Duration) float64 {
	return float64(value) / float64(time.Millisecond)
}
