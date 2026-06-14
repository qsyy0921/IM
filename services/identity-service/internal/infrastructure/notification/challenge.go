package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type NoopChallengeNotifier struct{}

func NewNoopChallengeNotifier() NoopChallengeNotifier {
	return NoopChallengeNotifier{}
}

func (NoopChallengeNotifier) SendChallenge(context.Context, types.ChallengeNotification) error {
	return nil
}

type WebhookChallengeNotifier struct {
	url         string
	bearerToken string
	client      *http.Client
}

func NewWebhookChallengeNotifier(url string, bearerToken string, timeout time.Duration) (*WebhookChallengeNotifier, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, errors.New("identity challenge webhook url is required")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &WebhookChallengeNotifier{
		url:         url,
		bearerToken: strings.TrimSpace(bearerToken),
		client:      &http.Client{Timeout: timeout},
	}, nil
}

func (notifier *WebhookChallengeNotifier) SendChallenge(ctx context.Context, notification types.ChallengeNotification) error {
	if notifier == nil || notifier.client == nil || notifier.url == "" {
		return types.NewChallengeDeliveryFailed("identity challenge notifier is not configured")
	}
	payload, err := json.Marshal(notification)
	if err != nil {
		return types.NewChallengeDeliveryFailed(err.Error())
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, notifier.url, bytes.NewReader(payload))
	if err != nil {
		return types.NewChallengeDeliveryFailed(err.Error())
	}
	request.Header.Set("Content-Type", "application/json")
	if notification.RequestID != "" {
		request.Header.Set("X-NexusIM-Request-ID", notification.RequestID)
	}
	if notifier.bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+notifier.bearerToken)
	}
	response, err := notifier.client.Do(request)
	if err != nil {
		return types.NewChallengeDeliveryFailed(err.Error())
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return types.NewChallengeDeliveryFailed("identity challenge webhook returned non-success status")
	}
	return nil
}

type SMTPChallengeNotifierConfig struct {
	Addr          string
	From          string
	Username      string
	Password      string
	ServerName    string
	TLSMode       string
	SubjectPrefix string
	Timeout       time.Duration
}

type SMTPChallengeNotifier struct {
	addr          string
	from          mail.Address
	username      string
	password      string
	serverName    string
	tlsMode       string
	subjectPrefix string
	timeout       time.Duration
}

const (
	smtpTLSModeNone     = "none"
	smtpTLSModeStartTLS = "starttls"
	smtpTLSModeTLS      = "tls"
)

func NewSMTPChallengeNotifier(config SMTPChallengeNotifierConfig) (*SMTPChallengeNotifier, error) {
	addr := strings.TrimSpace(config.Addr)
	if addr == "" {
		return nil, errors.New("identity challenge smtp addr is required")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil || strings.TrimSpace(host) == "" {
		return nil, errors.New("identity challenge smtp addr must be host:port")
	}
	from, err := mail.ParseAddress(strings.TrimSpace(config.From))
	if err != nil || from.Address == "" {
		return nil, errors.New("identity challenge smtp from address is required")
	}
	tlsMode := strings.ToLower(strings.TrimSpace(config.TLSMode))
	if tlsMode == "" {
		tlsMode = smtpTLSModeStartTLS
	}
	switch tlsMode {
	case smtpTLSModeNone, smtpTLSModeStartTLS, smtpTLSModeTLS:
	default:
		return nil, errors.New("identity challenge smtp tls mode must be none, starttls, or tls")
	}
	serverName := strings.TrimSpace(config.ServerName)
	if serverName == "" {
		serverName = host
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	subjectPrefix := strings.TrimSpace(config.SubjectPrefix)
	if subjectPrefix == "" {
		subjectPrefix = "NexusIM"
	}
	return &SMTPChallengeNotifier{
		addr:          addr,
		from:          *from,
		username:      strings.TrimSpace(config.Username),
		password:      config.Password,
		serverName:    serverName,
		tlsMode:       tlsMode,
		subjectPrefix: subjectPrefix,
		timeout:       timeout,
	}, nil
}

func (notifier *SMTPChallengeNotifier) SendChallenge(ctx context.Context, notification types.ChallengeNotification) error {
	if notifier == nil || notifier.addr == "" || notifier.from.Address == "" {
		return types.NewChallengeDeliveryFailed("identity challenge smtp notifier is not configured")
	}
	if notification.Channel != types.VerificationChannelEmail {
		return types.NewChallengeDeliveryFailed("identity challenge smtp notifier only supports email channel")
	}
	to, err := mail.ParseAddress(strings.TrimSpace(notification.Destination))
	if err != nil || to.Address == "" {
		return types.NewChallengeDeliveryFailed("identity challenge smtp destination is invalid")
	}
	if strings.TrimSpace(notification.Token) == "" {
		return types.NewChallengeDeliveryFailed("identity challenge smtp token is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, notifier.timeout)
	defer cancel()

	client, closer, err := notifier.connect(ctx)
	if err != nil {
		return err
	}
	defer closer()

	if err := notifier.deliver(client, to, notification); err != nil {
		return err
	}
	_ = client.Quit()
	return nil
}

func (notifier *SMTPChallengeNotifier) connect(ctx context.Context) (*smtp.Client, func(), error) {
	var conn net.Conn
	var err error
	dialer := &net.Dialer{Timeout: notifier.timeout}
	tlsConfig := &tls.Config{ServerName: notifier.serverName, MinVersion: tls.VersionTLS12}
	if notifier.tlsMode == smtpTLSModeTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", notifier.addr, tlsConfig)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", notifier.addr)
	}
	if err != nil {
		return nil, func() {}, types.NewChallengeDeliveryFailed("identity challenge smtp network error")
	}
	client, err := smtp.NewClient(conn, notifier.serverName)
	if err != nil {
		_ = conn.Close()
		return nil, func() {}, types.NewChallengeDeliveryFailed("identity challenge smtp provider returned non-success status")
	}
	closer := func() {
		_ = client.Close()
	}
	if notifier.tlsMode == smtpTLSModeStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			closer()
			return nil, func() {}, types.NewChallengeDeliveryFailed("identity challenge smtp provider does not support starttls")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			closer()
			return nil, func() {}, types.NewChallengeDeliveryFailed("identity challenge smtp starttls failed")
		}
	}
	if notifier.username != "" || notifier.password != "" {
		auth := smtp.PlainAuth("", notifier.username, notifier.password, notifier.serverName)
		if err := client.Auth(auth); err != nil {
			closer()
			return nil, func() {}, types.NewChallengeDeliveryFailed("identity challenge smtp authentication failed")
		}
	}
	return client, closer, nil
}

func (notifier *SMTPChallengeNotifier) deliver(client *smtp.Client, to *mail.Address, notification types.ChallengeNotification) error {
	if err := client.Mail(notifier.from.Address); err != nil {
		return types.NewChallengeDeliveryFailed("identity challenge smtp provider returned non-success status")
	}
	if err := client.Rcpt(to.Address); err != nil {
		return types.NewChallengeDeliveryFailed("identity challenge smtp provider returned non-success status")
	}
	writer, err := client.Data()
	if err != nil {
		return types.NewChallengeDeliveryFailed("identity challenge smtp provider returned non-success status")
	}
	if _, err := writer.Write([]byte(notifier.message(to, notification))); err != nil {
		_ = writer.Close()
		return types.NewChallengeDeliveryFailed("identity challenge smtp network error")
	}
	if err := writer.Close(); err != nil {
		return types.NewChallengeDeliveryFailed("identity challenge smtp provider returned non-success status")
	}
	return nil
}

func (notifier *SMTPChallengeNotifier) message(to *mail.Address, notification types.ChallengeNotification) string {
	subject := notifier.subject(notification.Type)
	body := notifier.body(notification)
	headers := []string{
		"From: " + notifier.from.String(),
		"To: " + to.String(),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + body
}

func (notifier *SMTPChallengeNotifier) subject(challengeType types.ChallengeType) string {
	switch challengeType {
	case types.ChallengeTypePasswordReset:
		return notifier.subjectPrefix + " password reset code"
	case types.ChallengeTypeEmailVerification:
		return notifier.subjectPrefix + " email verification code"
	default:
		return notifier.subjectPrefix + " verification code"
	}
}

func (notifier *SMTPChallengeNotifier) body(notification types.ChallengeNotification) string {
	expiresAt := time.UnixMilli(notification.ExpiresAtUnixMS).UTC().Format(time.RFC3339)
	if notification.ExpiresAtUnixMS <= 0 {
		expiresAt = "the configured challenge expiry time"
	}
	return fmt.Sprintf("Your %s code is:\r\n\r\n%s\r\n\r\nThis code expires at %s.\r\nIf you did not request this code, ignore this message.\r\n",
		challengePurpose(notification.Type),
		notification.Token,
		expiresAt,
	)
}

func challengePurpose(challengeType types.ChallengeType) string {
	switch challengeType {
	case types.ChallengeTypePasswordReset:
		return "password reset"
	case types.ChallengeTypeEmailVerification:
		return "email verification"
	case types.ChallengeTypePhoneVerification:
		return "phone verification"
	default:
		return "verification"
	}
}
