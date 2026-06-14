package notification

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestWebhookChallengeNotifierPostsChallenge(t *testing.T) {
	var got types.ChallengeNotification
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected content type %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("X-NexusIM-Request-ID") != "request-1" {
			t.Fatalf("unexpected request id header %q", r.Header.Get("X-NexusIM-Request-ID"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	notifier, err := NewWebhookChallengeNotifier(server.URL, "provider-token", time.Second)
	if err != nil {
		t.Fatalf("new notifier: %v", err)
	}
	err = notifier.SendChallenge(context.Background(), types.ChallengeNotification{
		TenantID:        "tenant-1",
		UserID:          "user-1",
		ChallengeID:     "challenge-1",
		Type:            types.ChallengeTypePasswordReset,
		Channel:         types.VerificationChannelEmail,
		Destination:     "user1@example.com",
		Token:           "challenge-token",
		ExpiresAtUnixMS: 1_800_000_900_000,
		RequestID:       "request-1",
	})
	if err != nil {
		t.Fatalf("send challenge: %v", err)
	}
	if gotAuthorization != "Bearer provider-token" {
		t.Fatalf("unexpected authorization header %q", gotAuthorization)
	}
	if got.Token != "challenge-token" || got.Type != types.ChallengeTypePasswordReset || got.Destination != "user1@example.com" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestWebhookChallengeNotifierReturnsStableErrorOnNonSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	notifier, err := NewWebhookChallengeNotifier(server.URL, "", time.Second)
	if err != nil {
		t.Fatalf("new notifier: %v", err)
	}
	err = notifier.SendChallenge(context.Background(), types.ChallengeNotification{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		ChallengeID: "challenge-1",
		Type:        types.ChallengeTypeEmailVerification,
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
		Token:       "challenge-token",
	})
	if !errors.Is(err, types.ErrChallengeDeliveryFailed) {
		t.Fatalf("expected challenge delivery failure, got %v", err)
	}
}

func TestSMTPChallengeNotifierSendsEmail(t *testing.T) {
	addr, messages := startFakeSMTPServer(t)
	notifier, err := NewSMTPChallengeNotifier(SMTPChallengeNotifierConfig{
		Addr:    addr,
		From:    "NexusIM <no-reply@nexusim.local>",
		TLSMode: "none",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new smtp notifier: %v", err)
	}
	err = notifier.SendChallenge(context.Background(), types.ChallengeNotification{
		TenantID:        "tenant-1",
		UserID:          "user-1",
		ChallengeID:     "challenge-1",
		Type:            types.ChallengeTypeEmailVerification,
		Channel:         types.VerificationChannelEmail,
		Destination:     "User One <user1@example.com>",
		Token:           "challenge-token",
		ExpiresAtUnixMS: 1_800_000_900_000,
	})
	if err != nil {
		t.Fatalf("send smtp challenge: %v", err)
	}
	select {
	case message := <-messages:
		if !strings.Contains(message, "Subject: NexusIM email verification code") ||
			!strings.Contains(message, "challenge-token") ||
			!strings.Contains(message, "To: \"User One\" <user1@example.com>") {
			t.Fatalf("unexpected smtp message:\n%s", message)
		}
	case <-time.After(time.Second):
		t.Fatal("smtp server did not receive message")
	}
}

func TestSMTPChallengeNotifierRejectsPhoneChannel(t *testing.T) {
	notifier, err := NewSMTPChallengeNotifier(SMTPChallengeNotifierConfig{
		Addr:    "127.0.0.1:2525",
		From:    "no-reply@nexusim.local",
		TLSMode: "none",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new smtp notifier: %v", err)
	}
	err = notifier.SendChallenge(context.Background(), types.ChallengeNotification{
		Channel:     types.VerificationChannelPhone,
		Destination: "+15555550123",
		Token:       "challenge-token",
	})
	if !errors.Is(err, types.ErrChallengeDeliveryFailed) {
		t.Fatalf("expected challenge delivery failure, got %v", err)
	}
}

func TestSMTPChallengeNotifierRequiresConfig(t *testing.T) {
	if _, err := NewSMTPChallengeNotifier(SMTPChallengeNotifierConfig{}); err == nil {
		t.Fatal("expected missing smtp addr to fail")
	}
	if _, err := NewSMTPChallengeNotifier(SMTPChallengeNotifierConfig{
		Addr: "127.0.0.1:2525",
		From: "not-an-address",
	}); err == nil {
		t.Fatal("expected invalid from address to fail")
	}
	if _, err := NewSMTPChallengeNotifier(SMTPChallengeNotifierConfig{
		Addr:    "127.0.0.1:2525",
		From:    "no-reply@nexusim.local",
		TLSMode: "invalid",
	}); err == nil {
		t.Fatal("expected invalid tls mode to fail")
	}
}

func startFakeSMTPServer(t *testing.T) (string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen smtp: %v", err)
	}
	messages := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(messages)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		writeSMTPLine(t, conn, "220 smtp.test ESMTP")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			command := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(command, "EHLO") || strings.HasPrefix(command, "HELO"):
				writeSMTPLine(t, conn, "250 smtp.test")
			case strings.HasPrefix(command, "MAIL FROM:"):
				writeSMTPLine(t, conn, "250 OK")
			case strings.HasPrefix(command, "RCPT TO:"):
				writeSMTPLine(t, conn, "250 OK")
			case command == "DATA":
				writeSMTPLine(t, conn, "354 End data with <CR><LF>.<CR><LF>")
				var builder strings.Builder
				for {
					dataLine, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if strings.TrimSpace(dataLine) == "." {
						break
					}
					builder.WriteString(dataLine)
				}
				messages <- builder.String()
				writeSMTPLine(t, conn, "250 OK")
			case command == "QUIT":
				writeSMTPLine(t, conn, "221 Bye")
				return
			default:
				writeSMTPLine(t, conn, "250 OK")
			}
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})
	return listener.Addr().String(), messages
}

func writeSMTPLine(t *testing.T, conn net.Conn, line string) {
	t.Helper()
	if _, err := conn.Write([]byte(line + "\r\n")); err != nil {
		t.Fatalf("write smtp line: %v", err)
	}
}
