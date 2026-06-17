package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runWebhook(cfg config) error {
	if strings.TrimSpace(cfg.webhookFile) == "" {
		return errors.New("--webhook-file is required in webhook mode")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.webhookFile), 0o755); err != nil {
		return fmt.Errorf("create webhook file directory: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/challenge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if cfg.webhookBearerToken != "" && r.Header.Get("Authorization") != "Bearer "+cfg.webhookBearerToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		defer r.Body.Close()
		var notification challengeNotification
		if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		notification.Authorization = r.Header.Get("Authorization")
		bytes, err := json.MarshalIndent(notification, "", "  ")
		if err != nil {
			http.Error(w, "encode failed", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(cfg.webhookFile, append(bytes, '\n'), 0o644); err != nil {
			http.Error(w, "write failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	listener, err := net.Listen("tcp", cfg.webhookListen)
	if err != nil {
		return fmt.Errorf("listen webhook: %w", err)
	}
	fmt.Printf("webhook_listen=%s\n", listener.Addr().String())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve webhook: %w", err)
	}
	return nil
}
func waitWebhookNotification(cfg config, challengeID string) (challengeNotification, error) {
	if strings.TrimSpace(cfg.webhookFile) == "" {
		return challengeNotification{}, errors.New("--webhook-file is required in client mode")
	}
	deadline := time.Now().Add(cfg.waitTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		bytes, err := os.ReadFile(cfg.webhookFile)
		if err == nil && len(bytes) > 0 {
			var notification challengeNotification
			if err := json.Unmarshal(bytes, &notification); err != nil {
				lastErr = err
			} else if notification.ChallengeID == challengeID {
				return notification, nil
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			lastErr = err
		}
		time.Sleep(cfg.pollInterval)
	}
	if lastErr != nil {
		return challengeNotification{}, fmt.Errorf("wait webhook notification: %w", lastErr)
	}
	return challengeNotification{}, fmt.Errorf("timed out waiting for webhook notification for challenge %s", challengeID)
}
