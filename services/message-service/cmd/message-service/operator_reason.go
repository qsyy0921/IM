package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const messageOperatorReasonMaxBytes = 16 * 1024

func messageOperatorReasonFromEnv(reasonEnv string, reasonFileEnv string, fallback string) (string, error) {
	reasonPath := strings.TrimSpace(os.Getenv(reasonFileEnv))
	reasonValue, hasReasonValue := os.LookupEnv(reasonEnv)
	reasonValue = strings.TrimSpace(reasonValue)
	if reasonPath != "" {
		if hasReasonValue && reasonValue != "" {
			return "", fmt.Errorf("%s and %s cannot both be set", reasonEnv, reasonFileEnv)
		}
		return messageOperatorReasonFromFile(reasonFileEnv, reasonPath)
	}
	if hasReasonValue && reasonValue != "" {
		return reasonValue, nil
	}
	return strings.TrimSpace(fallback), nil
}

func messageOperatorReasonFromFile(envName string, path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", envName, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s must point to a file", envName)
	}
	if info.Size() > messageOperatorReasonMaxBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", envName, messageOperatorReasonMaxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", envName, err)
	}
	reason := strings.TrimSpace(string(data))
	if reason == "" {
		return "", errors.New(envName + " is empty")
	}
	return reason, nil
}
