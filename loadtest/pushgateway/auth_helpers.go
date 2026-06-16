package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	nhooyr "nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func ownerAuth(cfg config, traceID string, requestID string) verifiedAuthIdentity {
	return verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.ownerUserID,
		deviceID:  "push-smoke-owner-device",
		sessionID: "push-smoke-owner-session",
		traceID:   traceID,
		requestID: requestID,
	}
}

func receiverAuth(cfg config, deviceID string, traceID string, requestID string) verifiedAuthIdentity {
	if deviceID == "" {
		deviceID = cfg.receiverDeviceID
	}
	return verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.receiverUserID,
		deviceID:  deviceID,
		sessionID: "push-smoke",
		traceID:   traceID,
		requestID: requestID,
	}
}

func withVerifiedAuthMetadata(ctx context.Context, cfg config, auth verifiedAuthIdentity) context.Context {
	if !cfg.verifiedAuthMetadata {
		return ctx
	}
	pairs := []string{
		metadataTenantID, auth.tenantID,
		metadataUserID, auth.userID,
		metadataDeviceID, auth.deviceID,
	}
	if auth.sessionID != "" {
		pairs = append(pairs, metadataSessionID, auth.sessionID)
	}
	if auth.traceID != "" {
		pairs = append(pairs, metadataTraceID, auth.traceID)
	}
	if auth.requestID != "" {
		pairs = append(pairs, metadataRequestID, auth.requestID)
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
}

func conversationAuth(auth verifiedAuthIdentity) *conversationv1.AuthContext {
	return &conversationv1.AuthContext{
		TenantId:  auth.tenantID,
		UserId:    auth.userID,
		DeviceId:  auth.deviceID,
		SessionId: auth.sessionID,
		TraceId:   auth.traceID,
		RequestId: auth.requestID,
	}
}

func messageAuth(auth verifiedAuthIdentity) *messagev1.AuthContext {
	return &messagev1.AuthContext{
		TenantId:  auth.tenantID,
		UserId:    auth.userID,
		DeviceId:  auth.deviceID,
		SessionId: auth.sessionID,
		TraceId:   auth.traceID,
		RequestId: auth.requestID,
	}
}

func deliveryAuth(auth verifiedAuthIdentity) *deliveryv1.AuthContext {
	return &deliveryv1.AuthContext{
		TenantId:  auth.tenantID,
		UserId:    auth.userID,
		DeviceId:  auth.deviceID,
		SessionId: auth.sessionID,
		TraceId:   auth.traceID,
		RequestId: auth.requestID,
	}
}

func registerTLSFlags(prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" gRPC mTLS")
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" gRPC mTLS")
}

func dialGRPCService(target string, tlsConfig grpctls.Config, tlsFlagPrefix string, serviceName string) (*grpc.ClientConn, error) {
	dialOption, err := grpctls.DialOption(tlsConfig, tlsFlagPrefix)
	if err != nil {
		return nil, fmt.Errorf("configure %s TLS: %w", serviceName, err)
	}
	conn, err := grpc.NewClient(target, dialOption)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", serviceName, err)
	}
	return conn, nil
}

func webSocketDialOptions(cfg config, header http.Header) (*nhooyr.DialOptions, error) {
	options := &nhooyr.DialOptions{}
	if header != nil {
		options.HTTPHeader = header
	}
	tlsConfig, err := webSocketTLSConfig(cfg.pushTLS, "push-tls")
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		options.HTTPClient = &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsConfig},
		}
	}
	if options.HTTPHeader == nil && options.HTTPClient == nil {
		return nil, nil
	}
	return options, nil
}

func webSocketTLSConfig(config grpctls.Config, flagPrefix string) (*tls.Config, error) {
	if !config.Enabled() {
		return nil, nil
	}
	caFile := strings.TrimSpace(config.CAFile)
	if caFile == "" {
		return nil, errors.New("--" + flagPrefix + "-ca-file is required when push WebSocket TLS is configured")
	}
	clientCertFile := strings.TrimSpace(config.ClientCertFile)
	clientKeyFile := strings.TrimSpace(config.ClientKeyFile)
	if (clientCertFile == "") != (clientKeyFile == "") {
		return nil, errors.New("--" + flagPrefix + "-client-cert-file and --" + flagPrefix + "-client-key-file must be configured together")
	}
	pemBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New("--" + flagPrefix + "-ca-file does not contain a valid PEM certificate")
	}
	tlsConfig := &tls.Config{
		RootCAs:    roots,
		ServerName: strings.TrimSpace(config.ServerName),
		MinVersion: tls.VersionTLS12,
	}
	if clientCertFile != "" {
		cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
}

func dialIdentityService(cfg config) (*grpc.ClientConn, error) {
	return dialGRPCService(cfg.identityTarget, cfg.identityTLS, "identity-tls", "identity-service")
}

func connectWebSocket(ctx context.Context, cfg config, deviceID string) (*nhooyr.Conn, serverFrame, error) {
	return connectWebSocketWithResume(ctx, cfg, deviceID, "", nil)
}

func connectWebSocketWithToken(ctx context.Context, cfg config, deviceID string, token string) (*nhooyr.Conn, serverFrame, error) {
	return connectWebSocketWithTokenAndResume(ctx, cfg, deviceID, token, "", nil)
}

func connectWebSocketWithResume(
	ctx context.Context,
	cfg config,
	deviceID string,
	resumeToken string,
	lastReceived []cursor,
) (*nhooyr.Conn, serverFrame, error) {
	return connectWebSocketWithTokenAndResume(ctx, cfg, deviceID, "", resumeToken, lastReceived)
}

func connectWebSocketWithTokenAndResume(
	ctx context.Context,
	cfg config,
	deviceID string,
	token string,
	resumeToken string,
	lastReceived []cursor,
) (*nhooyr.Conn, serverFrame, error) {
	u, err := url.Parse(cfg.pushURL)
	if err != nil {
		return nil, serverFrame{}, err
	}
	query := u.Query()
	query.Set("device_id", deviceID)
	var header http.Header
	switch cfg.pushAuthMode {
	case "", "mock":
		query.Set("tenant_id", cfg.tenantID)
		query.Set("user_id", cfg.receiverUserID)
	case "hmac", "jwt":
		if token == "" {
			token, err = gatewayToken(ctx, cfg, deviceID)
			if err != nil {
				return nil, serverFrame{}, err
			}
		}
		header = http.Header{"Authorization": []string{"Bearer " + token}}
	default:
		return nil, serverFrame{}, fmt.Errorf("unsupported push auth mode: %s", cfg.pushAuthMode)
	}
	u.RawQuery = query.Encode()
	dialOptions, err := webSocketDialOptions(cfg, header)
	if err != nil {
		return nil, serverFrame{}, fmt.Errorf("configure push WebSocket TLS: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	conn, _, err := nhooyr.Dial(requestCtx, u.String(), dialOptions)
	if err != nil {
		return nil, serverFrame{}, err
	}
	if err := wsjson.Write(requestCtx, conn, clientFrame{
		Op:           opClientHello,
		RequestID:    "push-smoke-hello-" + deviceID,
		DeviceID:     deviceID,
		ResumeToken:  resumeToken,
		LastReceived: lastReceived,
	}); err != nil {
		conn.CloseNow()
		return nil, serverFrame{}, err
	}
	var hello serverFrame
	if err := wsjson.Read(requestCtx, conn, &hello); err != nil {
		conn.CloseNow()
		return nil, serverFrame{}, err
	}
	if hello.Op != opServerHello || hello.SessionID == "" {
		conn.CloseNow()
		return nil, serverFrame{}, fmt.Errorf("unexpected hello: %+v", hello)
	}
	return conn, hello, nil
}

func connectWebSocketWithResumeExpectError(
	ctx context.Context,
	cfg config,
	deviceID string,
	resumeToken string,
	lastReceived []cursor,
) (serverFrame, error) {
	u, err := url.Parse(cfg.pushURL)
	if err != nil {
		return serverFrame{}, err
	}
	query := u.Query()
	query.Set("device_id", deviceID)
	var header http.Header
	switch cfg.pushAuthMode {
	case "", "mock":
		query.Set("tenant_id", cfg.tenantID)
		query.Set("user_id", cfg.receiverUserID)
	case "hmac", "jwt":
		token, err := gatewayToken(ctx, cfg, deviceID)
		if err != nil {
			return serverFrame{}, err
		}
		header = http.Header{"Authorization": []string{"Bearer " + token}}
	default:
		return serverFrame{}, fmt.Errorf("unsupported push auth mode: %s", cfg.pushAuthMode)
	}
	u.RawQuery = query.Encode()
	dialOptions, err := webSocketDialOptions(cfg, header)
	if err != nil {
		return serverFrame{}, fmt.Errorf("configure push WebSocket TLS: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	conn, _, err := nhooyr.Dial(requestCtx, u.String(), dialOptions)
	if err != nil {
		return serverFrame{}, err
	}
	defer conn.CloseNow()
	if err := wsjson.Write(requestCtx, conn, clientFrame{
		Op:           opClientHello,
		RequestID:    "push-smoke-resume-error-" + deviceID,
		DeviceID:     deviceID,
		ResumeToken:  resumeToken,
		LastReceived: lastReceived,
	}); err != nil {
		return serverFrame{}, err
	}
	frame, err := readServerFrame(requestCtx, cfg, conn)
	if err != nil {
		return serverFrame{}, err
	}
	if frame.Op != opError {
		return frame, fmt.Errorf("expected error frame, got %+v", frame)
	}
	return frame, nil
}

func readServerFrame(ctx context.Context, cfg config, conn *nhooyr.Conn) (serverFrame, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	var frame serverFrame
	if err := wsjson.Read(requestCtx, conn, &frame); err != nil {
		return serverFrame{}, err
	}
	return frame, nil
}

func waitWebSocketPermissionDenied(ctx context.Context, cfg config, deviceID string, token string) (serverFrame, int, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	attempts := 0
	var lastErr error
	for {
		attempts++
		frame, err := readAuthErrorFrame(ctx, cfg, deviceID, token)
		if err == nil && frame.Op == opError && frame.Code == "PERMISSION_DENIED" {
			return frame, attempts, nil
		}
		if err == nil {
			lastErr = fmt.Errorf("unexpected auth frame: %+v", frame)
		} else {
			lastErr = err
		}
		if time.Now().Add(cfg.pollInterval).After(deadline) {
			return serverFrame{}, attempts, fmt.Errorf("wait for revoked token rejection: %w", lastErr)
		}
		select {
		case <-ctx.Done():
			return serverFrame{}, attempts, ctx.Err()
		case <-time.After(cfg.pollInterval):
		}
	}
}

func readAuthErrorFrame(ctx context.Context, cfg config, deviceID string, token string) (serverFrame, error) {
	u, err := url.Parse(cfg.pushURL)
	if err != nil {
		return serverFrame{}, err
	}
	query := u.Query()
	query.Set("device_id", deviceID)
	u.RawQuery = query.Encode()
	dialOptions, err := webSocketDialOptions(cfg, http.Header{"Authorization": []string{"Bearer " + token}})
	if err != nil {
		return serverFrame{}, fmt.Errorf("configure push WebSocket TLS: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	conn, _, err := nhooyr.Dial(requestCtx, u.String(), dialOptions)
	if err != nil {
		return serverFrame{}, err
	}
	defer conn.CloseNow()
	var frame serverFrame
	if err := wsjson.Read(requestCtx, conn, &frame); err != nil {
		return serverFrame{}, err
	}
	return frame, nil
}

func pingWebSocket(ctx context.Context, cfg config, conn *nhooyr.Conn, requestID string) (serverFrame, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	if err := wsjson.Write(requestCtx, conn, clientFrame{
		Op:        opClientPing,
		RequestID: requestID,
	}); err != nil {
		return serverFrame{}, err
	}
	for {
		var frame serverFrame
		if err := wsjson.Read(requestCtx, conn, &frame); err != nil {
			return serverFrame{}, err
		}
		switch frame.Op {
		case opServerPong:
			return frame, nil
		case opDeliveryNotify, opResumeHint:
			continue
		case opError:
			return frame, fmt.Errorf("unexpected ping error frame: %+v", frame)
		default:
			return frame, fmt.Errorf("unexpected ping response frame: %+v", frame)
		}
	}
}

type pushGatewayTokenClaims struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
	Audience  string `json:"aud"`
	Expires   int64  `json:"exp"`
}

type gatewayTokenResult struct {
	Token     string
	SessionID string
}

func signPushGatewayToken(cfg config, deviceID string) (string, error) {
	claims := pushGatewayTokenClaims{
		TenantID: cfg.tenantID,
		UserID:   cfg.receiverUserID,
		DeviceID: deviceID,
		TraceID:  "push-smoke-auth",
		Audience: "push-gateway",
		Expires:  time.Now().Add(cfg.pushAuthTokenTTL).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(cfg.pushAuthTokenSigningSecret))
	_, _ = mac.Write([]byte(payloadPart))
	return payloadPart + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func gatewayToken(ctx context.Context, cfg config, deviceID string) (string, error) {
	result, err := gatewayTokenDetails(ctx, cfg, deviceID)
	if err != nil {
		return "", err
	}
	return result.Token, nil
}

func gatewayTokenDetails(ctx context.Context, cfg config, deviceID string) (gatewayTokenResult, error) {
	if strings.TrimSpace(cfg.identityTarget) != "" {
		switch cfg.identityTokenMethod {
		case "login", "register_login":
			return loginGatewayToken(ctx, cfg, deviceID)
		default:
			return issueGatewayToken(ctx, cfg, deviceID)
		}
	}
	token, err := signPushGatewayToken(cfg, deviceID)
	if err != nil {
		return gatewayTokenResult{}, err
	}
	return gatewayTokenResult{Token: token}, nil
}

func issueGatewayToken(ctx context.Context, cfg config, deviceID string) (gatewayTokenResult, error) {
	conn, err := dialIdentityService(cfg)
	if err != nil {
		return gatewayTokenResult{}, err
	}
	defer conn.Close()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := identityv1.NewIdentityServiceClient(conn).IssueGatewayToken(requestCtx, &identityv1.IssueGatewayTokenRequest{
		TenantId:   cfg.tenantID,
		UserId:     cfg.receiverUserID,
		DeviceId:   deviceID,
		Audience:   "push-gateway",
		TtlSeconds: int64(cfg.pushAuthTokenTTL.Seconds()),
		TraceId:    "push-smoke-auth",
		RequestId:  "push-smoke-identity-token-" + deviceID,
	})
	if err != nil {
		return gatewayTokenResult{}, fmt.Errorf("issue gateway token: %w", err)
	}
	if response.GetGatewayToken() == "" {
		return gatewayTokenResult{}, errors.New("identity-service returned empty gateway token")
	}
	return gatewayTokenResult{Token: response.GetGatewayToken(), SessionID: response.GetSessionId()}, nil
}

func loginGatewayToken(ctx context.Context, cfg config, deviceID string) (gatewayTokenResult, error) {
	conn, err := dialIdentityService(cfg)
	if err != nil {
		return gatewayTokenResult{}, err
	}
	defer conn.Close()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := identityv1.NewIdentityServiceClient(conn).Login(requestCtx, &identityv1.LoginRequest{
		TenantId:          cfg.tenantID,
		UserId:            cfg.receiverUserID,
		Password:          cfg.identityLoginPassword,
		DeviceId:          deviceID,
		Audience:          "push-gateway",
		GatewayTtlSeconds: int64(cfg.pushAuthTokenTTL.Seconds()),
		TraceId:           "push-smoke-auth-login",
		RequestId:         "push-smoke-identity-login-" + deviceID,
	})
	if err != nil {
		return gatewayTokenResult{}, fmt.Errorf("identity login: %w", err)
	}
	if response.GetGatewayToken() == "" {
		return gatewayTokenResult{}, errors.New("identity-service login returned empty gateway token")
	}
	if response.GetRefreshToken() == "" {
		return gatewayTokenResult{}, errors.New("identity-service login returned empty refresh token")
	}
	return gatewayTokenResult{Token: response.GetGatewayToken(), SessionID: response.GetSessionId()}, nil
}

func registerIdentityCredential(ctx context.Context, cfg config) error {
	conn, err := dialIdentityService(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	_, err = identityv1.NewIdentityServiceClient(conn).RegisterUser(requestCtx, &identityv1.RegisterUserRequest{
		TenantId:  cfg.tenantID,
		UserId:    cfg.receiverUserID,
		Password:  cfg.identityLoginPassword,
		TraceId:   "push-smoke-auth-register",
		RequestId: "push-smoke-identity-register",
	})
	if err != nil {
		return fmt.Errorf("identity register user: %w", err)
	}
	return nil
}

func revokeIdentityDevice(ctx context.Context, cfg config) error {
	conn, err := dialIdentityService(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	requestCtx = metadata.AppendToOutgoingContext(
		requestCtx,
		"x-nexusim-tenant-id", cfg.tenantID,
		"x-nexusim-user-id", cfg.ownerUserID,
		"x-nexusim-trace-id", "push-smoke-identity-revoke",
		"x-nexusim-request-id", "push-smoke-identity-revoke",
	)
	_, err = identityv1.NewIdentityServiceClient(conn).RevokeDevice(requestCtx, &identityv1.RevokeDeviceRequest{
		AdminContext: &identityv1.AdminContext{
			TenantId:       cfg.tenantID,
			OperatorUserId: cfg.ownerUserID,
			TraceId:        "push-smoke-identity-revoke",
			RequestId:      "push-smoke-identity-revoke",
		},
		UserId:   cfg.receiverUserID,
		DeviceId: cfg.receiverDeviceID,
		Reason:   "push gateway identity revoke smoke",
	})
	if err != nil {
		return fmt.Errorf("revoke identity device: %w", err)
	}
	return nil
}

func revokeIdentitySession(ctx context.Context, cfg config, sessionID string) error {
	conn, err := dialIdentityService(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	requestCtx = metadata.AppendToOutgoingContext(
		requestCtx,
		"x-nexusim-tenant-id", cfg.tenantID,
		"x-nexusim-user-id", cfg.ownerUserID,
		"x-nexusim-trace-id", "push-smoke-identity-session-revoke",
		"x-nexusim-request-id", "push-smoke-identity-session-revoke",
	)
	_, err = identityv1.NewIdentityServiceClient(conn).RevokeSession(requestCtx, &identityv1.RevokeSessionRequest{
		AdminContext: &identityv1.AdminContext{
			TenantId:       cfg.tenantID,
			OperatorUserId: cfg.ownerUserID,
			TraceId:        "push-smoke-identity-session-revoke",
			RequestId:      "push-smoke-identity-session-revoke",
		},
		UserId:    cfg.receiverUserID,
		DeviceId:  cfg.receiverDeviceID,
		SessionId: sessionID,
		Reason:    "push gateway identity session revoke smoke",
	})
	if err != nil {
		return fmt.Errorf("revoke identity session: %w", err)
	}
	return nil
}

func pushAuthTokenTransport(cfg config) string {
	if isSignedPushAuthMode(cfg.pushAuthMode) {
		return "authorization_header"
	}
	return "query"
}

func pushAuthQueryIdentitySent(cfg config) bool {
	return !isSignedPushAuthMode(cfg.pushAuthMode)
}

func pushAuthTokenSource(cfg config) string {
	if !isSignedPushAuthMode(cfg.pushAuthMode) {
		return "query_identity"
	}
	if strings.TrimSpace(cfg.identityTarget) != "" {
		switch cfg.identityTokenMethod {
		case "login":
			return "identity_service_login"
		case "register_login":
			return "identity_service_register_login"
		default:
			return "identity_service"
		}
	}
	return "local_hmac"
}

func isSignedPushAuthMode(mode string) bool {
	return mode == "hmac" || mode == "jwt"
}

func identityGatewayTokenFormat(cfg config) string {
	if strings.TrimSpace(cfg.identityTarget) == "" {
		return ""
	}
	value := strings.TrimSpace(cfg.identityGatewayTokenFormat)
	if value == "" {
		return "legacy"
	}
	return value
}

func identityTokenMethod(cfg config) string {
	if strings.TrimSpace(cfg.identityTarget) == "" {
		return ""
	}
	return normalizeIdentityTokenMethod(cfg.identityTokenMethod)
}

func normalizeIdentityTokenMethod(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "issue", "issue_gateway", "issue_gateway_token":
		return "issue_gateway_token"
	case "login":
		return "login"
	case "register", "register_login", "register_then_login":
		return "register_login"
	default:
		return value
	}
}

func normalizePushAuthConfig(cfg *config) {
	if cfg == nil {
		return
	}
	cfg.pushAuthHMACSecret = strings.TrimSpace(cfg.pushAuthHMACSecret)
	cfg.pushAuthHMACPreviousSecrets = strings.TrimSpace(cfg.pushAuthHMACPreviousSecrets)
	cfg.pushAuthTokenSigningSecret = strings.TrimSpace(cfg.pushAuthTokenSigningSecret)
	cfg.pushAuthTokenSigningSecretExplicit = cfg.pushAuthTokenSigningSecret != ""
	if cfg.pushAuthMode == "hmac" && cfg.pushAuthTokenSigningSecret == "" {
		cfg.pushAuthTokenSigningSecret = cfg.pushAuthHMACSecret
	}
}

func pushAuthTokenSignedWithNonCurrentSecret(cfg config) bool {
	if cfg.pushAuthMode != "hmac" {
		return false
	}
	return strings.TrimSpace(cfg.pushAuthTokenSigningSecret) != "" &&
		strings.TrimSpace(cfg.pushAuthHMACSecret) != "" &&
		strings.TrimSpace(cfg.pushAuthTokenSigningSecret) != strings.TrimSpace(cfg.pushAuthHMACSecret)
}
