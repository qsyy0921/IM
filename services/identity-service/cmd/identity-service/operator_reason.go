package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const identityOperatorReasonMaxBytes = 16 * 1024

func identityOperatorReasonFromEnv(reasonEnv string, reasonFileEnv string, defaultValue string) (string, error) {
	reasonPath := strings.TrimSpace(os.Getenv(reasonFileEnv))
	reasonValue, hasReasonValue := os.LookupEnv(reasonEnv)
	reasonValue = strings.TrimSpace(reasonValue)
	if reasonPath != "" {
		if hasReasonValue && reasonValue != "" {
			return "", fmt.Errorf("%s and %s cannot both be set", reasonEnv, reasonFileEnv)
		}
		return identityOperatorReasonFromFile(reasonFileEnv, reasonPath)
	}
	if hasReasonValue && reasonValue != "" {
		return reasonValue, nil
	}
	return strings.TrimSpace(defaultValue), nil
}

func identityOperatorReasonFromFile(envName string, path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", envName, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s must point to a file", envName)
	}
	if info.Size() > identityOperatorReasonMaxBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", envName, identityOperatorReasonMaxBytes)
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
