package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	identitygrpc "github.com/qsyy0921/IM/services/identity-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/identity-service/internal/app"
	credentialinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/credential"
	kafkainfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/kafka"
	mfainfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/mfa"
	monitoringinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/monitoring"
	notificationinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/notification"
	postgresinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/postgres"
	tokeninfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/token"
	challengedelivery "github.com/qsyy0921/IM/services/identity-service/internal/trigger/challengedelivery"
	"github.com/qsyy0921/IM/services/identity-service/internal/trigger/outbox"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type gatewayTokenSigner interface {
	SignGatewayToken(types.TokenClaims) (string, error)
	JWKSet() tokeninfra.JWKSet
}

type gatewayTokenKeyRingSigner struct {
	current gatewayTokenSigner
	jwkSet  tokeninfra.JWKSet
}

func (signer gatewayTokenKeyRingSigner) SignGatewayToken(claims types.TokenClaims) (string, error) {
	return signer.current.SignGatewayToken(claims)
}

func (signer gatewayTokenKeyRingSigner) JWKSet() tokeninfra.JWKSet {
	return signer.jwkSet
}

type gatewayTokenRS256KeyRingConfig struct {
	Issuer        string                      `json:"issuer,omitempty"`
	Current       gatewayTokenRS256CurrentKey `json:"current"`
	OldPublicKeys []tokeninfra.JWK            `json:"old_public_keys,omitempty"`
}

type gatewayTokenRS256CurrentKey struct {
	KeyID          string `json:"kid"`
	PrivateKeyPEM  string `json:"private_key_pem,omitempty"`
	PrivateKeyFile string `json:"private_key_file,omitempty"`
}

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("identity-service runtime wiring is idle; set NEXUSIM_IDENTITY_SERVICE_MODE=grpc, outbox-relay, challenge-delivery-worker, challenge-delivery-repair, challenge-delivery-repair-audit, challenge-request-limit-cleanup, session-mfa-proof-audit, or gateway-token-keyring-rotate")
		return nil
	case "grpc":
		return runGRPC()
	case "outbox-relay":
		return runOutboxRelay()
	case "challenge-delivery-worker":
		return runChallengeDeliveryWorker()
	case "challenge-delivery-repair":
		return runChallengeDeliveryRepair()
	case "challenge-delivery-repair-audit":
		return runChallengeDeliveryRepairAudit()
	case "challenge-request-limit-cleanup":
		return runChallengeRequestLimitCleanup()
	case "session-mfa-proof-audit":
		return runSessionMFAProofAudit()
	case "gateway-token-keyring-rotate":
		return runGatewayTokenKeyRingRotate()
	default:
		return errors.New("unsupported NEXUSIM_IDENTITY_SERVICE_MODE")
	}
}

func runGRPC() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	signer, err := newGatewayTokenSigner()
	if err != nil {
		return err
	}
	refreshTokens := tokeninfra.NewRefreshTokenCodec()
	challengeTokens := tokeninfra.NewChallengeTokenCodec()
	passwords := credentialinfra.NewPBKDF2Hasher(envInt("NEXUSIM_IDENTITY_PASSWORD_PBKDF2_ITERATIONS", 0))
	dummyPasswordHash, err := passwords.HashPassword("nexusim dummy login password")
	if err != nil {
		return err
	}
	mfaManager, err := newMFASecretManager()
	if err != nil {
		return err
	}
	mfaRecoveryCodes, err := newMFARecoveryCodeManager()
	if err != nil {
		return err
	}
	challengeNotifier, challengeDeliveryMode, err := newChallengeNotifier()
	if err != nil {
		return err
	}
	var challengeDeliveryTokens app.ChallengeDeliveryTokenCodec
	if challengeDeliveryMode == "outbox" {
		challengeDeliveryTokens, err = newChallengeDeliveryTokenManager()
		if err != nil {
			return err
		}
	}
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	challengeDeliveryMetrics := monitoringinfra.NewChallengeDeliveryMetrics(challengeDeliveryMode)
	challengeNotifier = monitoringinfra.NewInstrumentedChallengeNotifier(challengeNotifier, challengeDeliveryMetrics)
	challengeOptions := app.ChallengeOptions{
		ReturnDevToken:            envBool("NEXUSIM_IDENTITY_DEV_RETURN_CHALLENGE_TOKEN", false),
		Notifier:                  challengeNotifier,
		DeliveryOutbox:            challengeDeliveryMode == "outbox",
		DeliveryTokenCodec:        challengeDeliveryTokens,
		RequestLimitTargetKeySeed: envString("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_SECRET", ""),
	}
	jwkSet, err := gatewayTokenJWKSetWithAdditionalKeys(signer.JWKSet())
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, identityDebugAddr(), monitoringinfra.NewHandler(pool, grpcMetrics).
		WithJWKSet(jwkSet).
		WithChallengeDeliveryMetrics(challengeDeliveryMetrics))
	if err != nil {
		return err
	}
	defer stopDebug()

	addr := envString("NEXUSIM_IDENTITY_GRPC_ADDR", "0.0.0.0:10600")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(
		pool,
		postgresinfra.WithChallengeRequestLimit(
			envInt("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_MAX_PER_WINDOW", postgresinfra.DefaultChallengeRequestMaxPerWindow),
			envDuration("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_WINDOW", postgresinfra.DefaultChallengeRequestWindow),
		),
		postgresinfra.WithChallengeRequestLockDuration(
			envDuration("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LOCK_DURATION", postgresinfra.DefaultChallengeRequestLockDuration),
		),
	)
	server, err := newGRPCServer(grpcMetrics)
	if err != nil {
		return err
	}
	mfaRiskPolicy := identityMFARiskPolicyFromEnv()
	mfaRecoveryRiskPolicy := identityMFARecoveryRiskPolicyFromEnv(mfaRiskPolicy)
	identitygrpc.Register(server, identitygrpc.NewServer(
		app.NewRegisterUserUseCase(repository, passwords),
		app.NewLoginUseCase(
			repository,
			signer,
			passwords,
			refreshTokens,
			app.WithLoginRiskPolicy(app.LoginRiskPolicy{
				MaxFailedAttempts: envInt("NEXUSIM_IDENTITY_LOGIN_MAX_FAILED_ATTEMPTS", app.DefaultLoginMaxFailedAttempts),
				FailureWindow:     envDuration("NEXUSIM_IDENTITY_LOGIN_FAILURE_WINDOW", app.DefaultLoginFailureWindow),
				LockDuration:      envDuration("NEXUSIM_IDENTITY_LOGIN_LOCK_DURATION", app.DefaultLoginLockDuration),
			}),
			app.WithLoginMFARiskPolicy(mfaRiskPolicy),
			app.WithLoginMFARecoveryRiskPolicy(mfaRecoveryRiskPolicy),
			app.WithLoginDummyPasswordHash(dummyPasswordHash),
			app.WithLoginMFASecretManager(mfaManager),
			app.WithLoginMFARecoveryCodeManager(mfaRecoveryCodes),
		),
		app.NewRefreshGatewayTokenUseCase(
			repository,
			signer,
			refreshTokens,
			app.WithRefreshMFARiskPolicy(mfaRiskPolicy),
			app.WithRefreshMFARecoveryRiskPolicy(mfaRecoveryRiskPolicy),
			app.WithRefreshMFASecretManager(mfaManager),
			app.WithRefreshMFARecoveryCodeManager(mfaRecoveryCodes),
		),
		app.NewRequestVerificationChallengeUseCase(repository, challengeTokens, passwords, app.ChallengeOptions{
			ReturnDevToken:     challengeOptions.ReturnDevToken,
			Notifier:           challengeOptions.Notifier,
			DeliveryOutbox:     challengeOptions.DeliveryOutbox,
			DeliveryTokenCodec: challengeOptions.DeliveryTokenCodec,
		}),
		app.NewConfirmVerificationChallengeUseCase(repository, challengeTokens),
		app.NewRequestPasswordResetUseCase(repository, challengeTokens, challengeOptions),
		app.NewConfirmPasswordResetUseCase(repository, challengeTokens, passwords),
		app.NewBeginMFAEnrollmentUseCase(repository, passwords, mfaManager),
		app.NewConfirmMFAEnrollmentUseCase(repository, mfaManager, mfaRecoveryCodes),
		app.NewDisableMFAFactorUseCase(repository, passwords),
		app.NewRegenerateMFARecoveryCodesUseCase(repository, passwords, mfaManager, mfaRecoveryCodes),
		app.NewRevokeMFARecoveryCodesUseCase(repository, passwords),
		app.NewIssueGatewayTokenUseCase(repository, signer),
		app.NewRevokeDeviceUseCase(repository),
		app.NewRevokeSessionUseCase(repository),
		app.NewGetDeviceStateUseCase(repository),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("identity-service grpc listening on %s", addr)

	select {
	case err := <-serveErr:
		if errors.Is(err, grpc.ErrServerStopped) {
			return context.Canceled
		}
		return err
	case <-ctx.Done():
		server.GracefulStop()
		err := <-serveErr
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return err
		}
		return context.Canceled
	}
}

func newChallengeNotifier() (app.ChallengeNotifier, string, error) {
	mode := challengeDeliveryMode()
	switch mode {
	case "", "noop", "disabled":
		return notificationinfra.NewNoopChallengeNotifier(), mode, nil
	case "outbox":
		return notificationinfra.NewNoopChallengeNotifier(), mode, nil
	case "webhook":
		notifier, err := newChallengeWebhookNotifier()
		return notifier, mode, err
	default:
		return nil, mode, errors.New("unsupported NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_MODE")
	}
}

func newChallengeWebhookNotifier() (app.ChallengeNotifier, error) {
	return notificationinfra.NewWebhookChallengeNotifier(
		envString("NEXUSIM_IDENTITY_CHALLENGE_WEBHOOK_URL", ""),
		envString("NEXUSIM_IDENTITY_CHALLENGE_WEBHOOK_BEARER_TOKEN", ""),
		envDuration("NEXUSIM_IDENTITY_CHALLENGE_WEBHOOK_TIMEOUT", 5*time.Second),
	)
}

func challengeDeliveryMode() string {
	mode := strings.ToLower(strings.TrimSpace(envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_MODE", "noop")))
	if mode == "" {
		return "noop"
	}
	return mode
}

func newChallengeDeliveryTokenManager() (*tokeninfra.ChallengeDeliveryTokenManager, error) {
	if keyRing, ok, err := loadSecretKeyRingConfig(
		"NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEYRING_JSON",
		"NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEYRING_FILE",
	); err != nil {
		return nil, err
	} else if ok {
		return tokeninfra.NewChallengeDeliveryTokenManagerWithKeyRing(keyRing.Current, keyRing.Keys)
	}
	return tokeninfra.NewChallengeDeliveryTokenManager(envString(
		"NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY",
		envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_SECRET", ""),
	))
}

func newMFASecretManager() (app.MFASecretManager, error) {
	if keyRing, ok, err := loadSecretKeyRingConfig(
		"NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_JSON",
		"NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_FILE",
	); err != nil {
		return nil, err
	} else if ok {
		return mfainfra.NewTOTPManagerWithKeyRing(keyRing.Current, keyRing.Keys)
	}
	secret := envString(
		"NEXUSIM_IDENTITY_MFA_SECRET_KEY",
		envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_SECRET", envString("NEXUSIM_PUSH_AUTH_HMAC_SECRET", "")),
	)
	if secret == "" {
		return disabledMFASecretManager{}, nil
	}
	return mfainfra.NewTOTPManager(secret)
}

type secretKeyRingConfig struct {
	Current string            `json:"current"`
	Keys    map[string]string `json:"keys"`
}

func loadSecretKeyRingConfig(jsonEnv string, fileEnv string) (secretKeyRingConfig, bool, error) {
	raw := strings.TrimSpace(os.Getenv(jsonEnv))
	if raw == "" {
		path := strings.TrimSpace(os.Getenv(fileEnv))
		if path == "" {
			return secretKeyRingConfig{}, false, nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return secretKeyRingConfig{}, true, err
		}
		raw = strings.TrimSpace(string(content))
		if raw == "" {
			return secretKeyRingConfig{}, true, errors.New("identity secret keyring file is empty")
		}
	}
	if raw == "" {
		return secretKeyRingConfig{}, false, nil
	}
	var config secretKeyRingConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return secretKeyRingConfig{}, true, err
	}
	config.Current = strings.TrimSpace(config.Current)
	if config.Current == "" {
		return secretKeyRingConfig{}, true, errors.New("identity secret keyring current key version is required")
	}
	if len(config.Keys) == 0 {
		return secretKeyRingConfig{}, true, errors.New("identity secret keyring keys are required")
	}
	normalized := make(map[string]string, len(config.Keys))
	for keyVersion, secret := range config.Keys {
		keyVersion = strings.TrimSpace(keyVersion)
		if keyVersion == "" {
			return secretKeyRingConfig{}, true, errors.New("identity secret keyring key version is required")
		}
		if _, ok := normalized[keyVersion]; ok {
			return secretKeyRingConfig{}, true, errors.New("identity secret keyring duplicate key version")
		}
		if strings.TrimSpace(secret) == "" {
			return secretKeyRingConfig{}, true, errors.New("identity secret keyring key value is required")
		}
		normalized[keyVersion] = secret
	}
	if _, ok := normalized[config.Current]; !ok {
		return secretKeyRingConfig{}, true, errors.New("identity secret keyring current key version is not configured")
	}
	config.Keys = normalized
	return config, true, nil
}

func newMFARecoveryCodeManager() (app.MFARecoveryCodeManager, error) {
	secret := envString(
		"NEXUSIM_IDENTITY_MFA_RECOVERY_CODE_SECRET",
		envString("NEXUSIM_IDENTITY_MFA_SECRET_KEY", ""),
	)
	if secret == "" {
		return disabledMFARecoveryCodeManager{}, nil
	}
	return mfainfra.NewRecoveryCodeManager(secret)
}

func identityMFARiskPolicyFromEnv() app.LoginRiskPolicy {
	return app.LoginRiskPolicy{
		MaxFailedAttempts: envInt("NEXUSIM_IDENTITY_MFA_MAX_FAILED_ATTEMPTS", app.DefaultMFAMaxFailedAttempts),
		FailureWindow:     envDuration("NEXUSIM_IDENTITY_MFA_FAILURE_WINDOW", app.DefaultMFAFailureWindow),
		LockDuration:      envDuration("NEXUSIM_IDENTITY_MFA_LOCK_DURATION", app.DefaultMFALockDuration),
	}
}

func identityMFARecoveryRiskPolicyFromEnv(fallback app.LoginRiskPolicy) app.LoginRiskPolicy {
	return app.LoginRiskPolicy{
		MaxFailedAttempts: envInt("NEXUSIM_IDENTITY_MFA_RECOVERY_MAX_FAILED_ATTEMPTS", fallback.MaxFailedAttempts),
		FailureWindow:     envDuration("NEXUSIM_IDENTITY_MFA_RECOVERY_FAILURE_WINDOW", fallback.FailureWindow),
		LockDuration:      envDuration("NEXUSIM_IDENTITY_MFA_RECOVERY_LOCK_DURATION", fallback.LockDuration),
	}
}

type disabledMFASecretManager struct{}

func (disabledMFASecretManager) NewTOTPSecret() (string, types.EncryptedMFASecret, error) {
	return "", types.EncryptedMFASecret{}, types.NewMFAUnavailable("mfa secret encryption key is required")
}

func (disabledMFASecretManager) VerifyTOTP(types.EncryptedMFASecret, string, time.Time) (bool, error) {
	return false, types.NewMFAUnavailable("mfa secret encryption key is required")
}

func (disabledMFASecretManager) OTPAuthURI(string, string, string) string {
	return ""
}

type disabledMFARecoveryCodeManager struct{}

func (disabledMFARecoveryCodeManager) NewRecoveryCodes(int) ([]types.MFARecoveryCode, error) {
	return nil, types.NewMFAUnavailable("mfa recovery code key is required")
}

func (disabledMFARecoveryCodeManager) HashRecoveryCode(string) (string, error) {
	return "", types.NewMFAUnavailable("mfa recovery code key is required")
}

func newGatewayTokenSigner() (gatewayTokenSigner, error) {
	secret := envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_SECRET", envString("NEXUSIM_PUSH_AUTH_HMAC_SECRET", ""))
	switch strings.ToLower(envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT", "legacy")) {
	case "legacy", "hmac", "custom":
		return tokeninfra.NewHMACSigner(secret)
	case "jwt", "jwt-hs256", "hs256":
		return tokeninfra.NewJWTSigner(
			secret,
			envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEY_ID", ""),
			envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ISSUER", ""),
		)
	case "jwt-rs256", "rs256":
		if signer, ok, err := loadRS256KeyRingSigner(); err != nil {
			return nil, err
		} else if ok {
			return signer, nil
		}
		privateKeyPEM, err := loadRSAPrivateKeyPEM()
		if err != nil {
			return nil, err
		}
		return tokeninfra.NewRS256SignerFromPEM(
			privateKeyPEM,
			envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEY_ID", ""),
			envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ISSUER", ""),
		)
	default:
		return nil, errors.New("unsupported NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT")
	}
}

func loadRS256KeyRingSigner() (gatewayTokenSigner, bool, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_JSON"))
	if raw == "" {
		path := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_FILE"))
		if path == "" {
			return nil, false, nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, true, err
		}
		raw = strings.TrimSpace(string(content))
	}
	if raw == "" {
		return nil, false, nil
	}
	var config gatewayTokenRS256KeyRingConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, true, err
	}
	keyID := strings.TrimSpace(config.Current.KeyID)
	if keyID == "" {
		return nil, true, errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING current.kid is required")
	}
	privateKeyPEM, err := loadRS256KeyRingPrivateKeyPEM(config.Current)
	if err != nil {
		return nil, true, err
	}
	issuer := strings.TrimSpace(config.Issuer)
	if issuer == "" {
		issuer = envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ISSUER", "")
	}
	current, err := tokeninfra.NewRS256SignerFromPEM(privateKeyPEM, keyID, issuer)
	if err != nil {
		return nil, true, err
	}
	publicKeys, err := mergeGatewayTokenPublicJWKSets(
		current.JWKSet(),
		tokeninfra.JWKSet{Keys: config.OldPublicKeys},
	)
	if err != nil {
		return nil, true, err
	}
	return gatewayTokenKeyRingSigner{current: current, jwkSet: publicKeys}, true, nil
}

func loadRS256KeyRingPrivateKeyPEM(current gatewayTokenRS256CurrentKey) (string, error) {
	if pemValue := strings.TrimSpace(current.PrivateKeyPEM); pemValue != "" {
		return pemValue, nil
	}
	path := strings.TrimSpace(current.PrivateKeyFile)
	if path == "" {
		return "", errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING current.private_key_pem or current.private_key_file is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func runGatewayTokenKeyRingRotate() error {
	path := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_FILE"))
	if path == "" {
		return errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_FILE is required")
	}
	rotated, err := rotateRS256KeyRingFile(path, gatewayTokenKeyRingRotateOptions{
		NewKeyID:    envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ROTATE_NEW_KID", ""),
		RSABits:     envInt("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ROTATE_RSA_BITS", 2048),
		OldKeyLimit: envInt("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ROTATE_OLD_KEY_LIMIT", 3),
		Now:         time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	log.Printf("rotated identity gateway RS256 keyring file=%s current_kid=%s old_public_keys=%d", path, rotated.Current.KeyID, len(rotated.OldPublicKeys))
	return nil
}

type gatewayTokenKeyRingRotateOptions struct {
	NewKeyID    string
	RSABits     int
	OldKeyLimit int
	Now         time.Time
}

func rotateRS256KeyRingFile(path string, options gatewayTokenKeyRingRotateOptions) (gatewayTokenRS256KeyRingConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	var config gatewayTokenRS256KeyRingConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	rotated, err := rotateRS256KeyRing(config, options)
	if err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	raw, err := json.MarshalIndent(rotated, "", "  ")
	if err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	raw = append(raw, '\n')
	perm := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	if err := writeFileReplace(path, raw, perm); err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	return rotated, nil
}

func rotateRS256KeyRing(config gatewayTokenRS256KeyRingConfig, options gatewayTokenKeyRingRotateOptions) (gatewayTokenRS256KeyRingConfig, error) {
	oldCurrentKey, err := currentRS256PublicJWK(config.Current)
	if err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	newKeyID := strings.TrimSpace(options.NewKeyID)
	if newKeyID == "" {
		newKeyID = "nexusim-gateway-rs256-" + now.UTC().Format("20060102T150405Z")
	}
	if newKeyID == oldCurrentKey.KeyID {
		return gatewayTokenRS256KeyRingConfig{}, errors.New("new gateway token kid must differ from current kid")
	}
	for _, key := range config.OldPublicKeys {
		publicKey, ok := publicGatewayTokenJWK(key)
		if !ok {
			return gatewayTokenRS256KeyRingConfig{}, errors.New("gateway token keyring old_public_keys may only contain RS256 public keys")
		}
		if publicKey.KeyID == newKeyID {
			return gatewayTokenRS256KeyRingConfig{}, errors.New("new gateway token kid must not already exist in old_public_keys")
		}
	}
	bits := options.RSABits
	if bits == 0 {
		bits = 2048
	}
	if bits < 2048 {
		return gatewayTokenRS256KeyRingConfig{}, errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ROTATE_RSA_BITS must be at least 2048")
	}
	oldLimit := options.OldKeyLimit
	if oldLimit == 0 {
		oldLimit = 3
	}
	if oldLimit < 0 {
		return gatewayTokenRS256KeyRingConfig{}, errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ROTATE_OLD_KEY_LIMIT must be non-negative")
	}
	newPrivateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	oldKeys, err := mergeGatewayTokenPublicJWKSets(
		tokeninfra.JWKSet{Keys: []tokeninfra.JWK{oldCurrentKey}},
		tokeninfra.JWKSet{Keys: config.OldPublicKeys},
	)
	if err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	if oldLimit < len(oldKeys.Keys) {
		oldKeys.Keys = oldKeys.Keys[:oldLimit]
	}
	return gatewayTokenRS256KeyRingConfig{
		Issuer: strings.TrimSpace(config.Issuer),
		Current: gatewayTokenRS256CurrentKey{
			KeyID:         newKeyID,
			PrivateKeyPEM: marshalRSAPrivateKeyPEM(newPrivateKey),
		},
		OldPublicKeys: oldKeys.Keys,
	}, nil
}

func currentRS256PublicJWK(current gatewayTokenRS256CurrentKey) (tokeninfra.JWK, error) {
	keyID := strings.TrimSpace(current.KeyID)
	if keyID == "" {
		return tokeninfra.JWK{}, errors.New("gateway token keyring current.kid is required")
	}
	privateKeyPEM, err := loadRS256KeyRingPrivateKeyPEM(current)
	if err != nil {
		return tokeninfra.JWK{}, err
	}
	signer, err := tokeninfra.NewRS256SignerFromPEM(privateKeyPEM, keyID, "")
	if err != nil {
		return tokeninfra.JWK{}, err
	}
	jwks := signer.JWKSet()
	if len(jwks.Keys) != 1 {
		return tokeninfra.JWK{}, errors.New("gateway token keyring current key did not produce one public jwk")
	}
	return jwks.Keys[0], nil
}

func marshalRSAPrivateKeyPEM(privateKey *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
}

func writeFileReplace(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmpPath, path)
}

func loadRSAPrivateKeyPEM() (string, error) {
	if pemValue := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_PEM")); pemValue != "" {
		return pemValue, nil
	}
	path := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_FILE"))
	if path == "" {
		return "", errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_PEM or NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_FILE is required for RS256")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func gatewayTokenJWKSetWithAdditionalKeys(base tokeninfra.JWKSet) (tokeninfra.JWKSet, error) {
	additional, err := loadAdditionalGatewayTokenJWKSet()
	if err != nil {
		return tokeninfra.JWKSet{}, err
	}
	return mergeGatewayTokenPublicJWKSets(base, additional)
}

func mergeGatewayTokenPublicJWKSets(sets ...tokeninfra.JWKSet) (tokeninfra.JWKSet, error) {
	totalKeys := 0
	for _, set := range sets {
		totalKeys += len(set.Keys)
	}
	result := tokeninfra.JWKSet{Keys: make([]tokeninfra.JWK, 0, totalKeys)}
	seen := make(map[string]struct{}, totalKeys)
	appendKey := func(key tokeninfra.JWK) error {
		publicKey, ok := publicGatewayTokenJWK(key)
		if !ok {
			return errors.New("gateway token JWKS may only expose RS256 public keys")
		}
		if _, ok := seen[publicKey.KeyID]; ok {
			return nil
		}
		seen[publicKey.KeyID] = struct{}{}
		result.Keys = append(result.Keys, publicKey)
		return nil
	}
	for _, set := range sets {
		for _, key := range set.Keys {
			if err := appendKey(key); err != nil {
				return tokeninfra.JWKSet{}, err
			}
		}
	}
	return result, nil
}

func publicGatewayTokenJWK(key tokeninfra.JWK) (tokeninfra.JWK, bool) {
	publicKey := tokeninfra.JWK{
		KeyType:   strings.TrimSpace(key.KeyType),
		KeyUse:    strings.TrimSpace(key.KeyUse),
		KeyID:     strings.TrimSpace(key.KeyID),
		Algorithm: strings.TrimSpace(key.Algorithm),
		Modulus:   strings.TrimSpace(key.Modulus),
		Exponent:  strings.TrimSpace(key.Exponent),
	}
	if publicKey.KeyType != "RSA" || publicKey.Algorithm != "RS256" || publicKey.KeyID == "" {
		return tokeninfra.JWK{}, false
	}
	if publicKey.KeyUse != "" && publicKey.KeyUse != "sig" {
		return tokeninfra.JWK{}, false
	}
	if publicKey.Modulus == "" || publicKey.Exponent == "" || hasGatewayTokenPrivateJWKMaterial(key) {
		return tokeninfra.JWK{}, false
	}
	modulus, err := base64.RawURLEncoding.DecodeString(publicKey.Modulus)
	if err != nil || len(modulus) == 0 || new(big.Int).SetBytes(modulus).BitLen() < 2048 {
		return tokeninfra.JWK{}, false
	}
	exponent, err := base64.RawURLEncoding.DecodeString(publicKey.Exponent)
	exponentInt := new(big.Int).SetBytes(exponent)
	if err != nil || len(exponent) == 0 || !exponentInt.IsInt64() || exponentInt.Int64() <= 1 {
		return tokeninfra.JWK{}, false
	}
	return publicKey, true
}

func hasGatewayTokenPrivateJWKMaterial(key tokeninfra.JWK) bool {
	return strings.TrimSpace(key.Key) != "" ||
		strings.TrimSpace(key.PrivateExponent) != "" ||
		strings.TrimSpace(key.Prime1) != "" ||
		strings.TrimSpace(key.Prime2) != "" ||
		strings.TrimSpace(key.Exponent1) != "" ||
		strings.TrimSpace(key.Exponent2) != "" ||
		strings.TrimSpace(key.Coefficient) != "" ||
		strings.TrimSpace(string(key.OtherPrimes)) != ""
}

func loadAdditionalGatewayTokenJWKSet() (tokeninfra.JWKSet, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_JSON"))
	if raw == "" {
		path := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_FILE"))
		if path == "" {
			return tokeninfra.JWKSet{}, nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return tokeninfra.JWKSet{}, err
		}
		raw = strings.TrimSpace(string(content))
	}
	if raw == "" {
		return tokeninfra.JWKSet{}, nil
	}
	var set tokeninfra.JWKSet
	if err := json.Unmarshal([]byte(raw), &set); err != nil {
		return tokeninfra.JWKSet{}, err
	}
	return set, nil
}

func runOutboxRelay() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	stopDebug, err := startDebugServer(ctx, identityDebugAddr(), monitoringinfra.NewHandler(pool))
	if err != nil {
		return err
	}
	defer stopDebug()

	brokers := splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS"))
	producer, err := kafkainfra.NewWriterProducer(brokers)
	if err != nil {
		return err
	}
	defer producer.Close()

	topic := envString("NEXUSIM_IDENTITY_EVENTS_TOPIC", outbox.TopicIdentityEvents)
	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          topic,
			BatchSize:      envInt("NEXUSIM_IDENTITY_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_IDENTITY_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_IDENTITY_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_IDENTITY_OUTBOX_RETRY_BASE_DELAY", time.Second),
		},
	)
	log.Printf("identity-service outbox relay started topic=%s", topic)
	return relay.Run(ctx)
}

func runChallengeDeliveryWorker() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	challengeDeliveryMetrics := monitoringinfra.NewChallengeDeliveryMetrics("outbox-webhook")
	stopDebug, err := startDebugServer(ctx, identityDebugAddr(), monitoringinfra.NewHandler(pool).
		WithChallengeDeliveryMetrics(challengeDeliveryMetrics))
	if err != nil {
		return err
	}
	defer stopDebug()

	tokenManager, err := newChallengeDeliveryTokenManager()
	if err != nil {
		return err
	}
	notifier, err := newChallengeWebhookNotifier()
	if err != nil {
		return err
	}
	instrumentedNotifier := monitoringinfra.NewInstrumentedChallengeNotifier(notifier, challengeDeliveryMetrics)
	worker := challengedelivery.NewWorker(
		postgresinfra.NewChallengeDeliveryStore(pool),
		instrumentedNotifier,
		tokenManager,
		challengedelivery.Config{
			BatchSize:      envInt("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_BATCH_SIZE", 100),
			PollInterval:   envDuration("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_RETRY_BASE_DELAY", time.Second),
		},
	)
	log.Println("identity-service challenge delivery worker started")
	return worker.Run(ctx)
}

func runChallengeDeliveryRepair() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	ids, err := parseInt64CSV(os.Getenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_IDS"))
	if err != nil {
		return err
	}
	stats, err := postgresinfra.NewChallengeDeliveryStore(pool).RepairDeliveries(
		ctx,
		types.ChallengeDeliveryRepairOptions{
			DeliveryIDs: ids,
			Mode:        envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_MODE", types.ChallengeDeliveryRepairModeAudit),
			Operator:    envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_OPERATOR", "manual"),
			Reason:      envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_REASON", "manual identity challenge delivery repair"),
			DryRun:      envBool("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_DRY_RUN", false),
		})
	if err != nil {
		return err
	}
	log.Printf(
		"identity-service challenge delivery repair completed requested=%d audited=%d mutated=%d skipped=%d mode=%s dry_run=%t",
		stats.Requested,
		stats.Audited,
		stats.Mutated,
		stats.Skipped,
		envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_MODE", types.ChallengeDeliveryRepairModeAudit),
		envBool("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_DRY_RUN", false),
	)
	return nil
}

func runChallengeDeliveryRepairAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	deliveryID, err := optionalPositiveInt64Env("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_DELIVERY_ID")
	if err != nil {
		return err
	}
	rows, err := postgresinfra.NewChallengeDeliveryStore(pool).AuditDeliveryRepairs(ctx, postgresinfra.ChallengeDeliveryRepairAuditOptions{
		DeliveryID:           deliveryID,
		TenantID:             envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_TENANT_ID", ""),
		UserID:               envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_USER_ID", ""),
		ChallengeID:          envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_CHALLENGE_ID", ""),
		Mode:                 envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_MODE", ""),
		Outcome:              envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_OUTCOME", ""),
		PreviousFailureClass: envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_PREVIOUS_FAILURE_CLASS", ""),
		NewFailureClass:      envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_NEW_FAILURE_CLASS", ""),
		Limit:                envInt("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("identity-service challenge delivery repair audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"identity_challenge_delivery_repair delivery_id=%d tenant_id=%s user_id=%s challenge_id=%s mode=%s outcome=%s skip_reason=%s dry_run=%t previous_delivery_status=%s previous_challenge_status=%s previous_challenge_delivery_status=%s previous_retry_count=%d previous_failure_class=%s new_delivery_status=%s new_challenge_status=%s new_challenge_delivery_status=%s new_failure_class=%s operator=%s repaired_at=%s reason=%q previous_dead_lettered_at=%s",
			row.DeliveryID,
			row.TenantID,
			row.UserID,
			row.ChallengeID,
			row.Mode,
			row.Outcome,
			row.SkipReason,
			row.DryRun,
			row.PreviousDeliveryStatus,
			row.PreviousChallengeStatus,
			row.PreviousChallengeDeliveryStatus,
			row.PreviousRetryCount,
			row.PreviousFailureClass,
			row.NewDeliveryStatus,
			row.NewChallengeStatus,
			row.NewChallengeDeliveryStatus,
			row.NewFailureClass,
			row.Operator,
			row.RepairedAt.Format(time.RFC3339),
			row.Reason,
			formatOptionalTime(row.PreviousDeadLetteredAt),
		)
	}
	return nil
}

func runChallengeRequestLimitCleanup() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	config, err := challengeRequestLimitCleanupConfigFromEnv()
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-config.Retention)
	deleted, err := postgresinfra.NewRepository(pool).CleanupChallengeRequestLimits(ctx, cutoff, config.BatchSize)
	if err != nil {
		return err
	}
	log.Printf(
		"identity-service challenge request limit cleanup completed deleted=%d cutoff=%s retention=%s batch_size=%d",
		deleted,
		cutoff.Format(time.RFC3339),
		config.Retention,
		config.BatchSize,
	)
	return nil
}

func runSessionMFAProofAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	stats, err := postgresinfra.NewRepository(pool).AuditSessionMFAProofConstraints(ctx)
	if err != nil {
		return err
	}
	log.Printf(
		"identity-service session mfa proof audit completed invalid_total=%d unknown_method=%d empty_method_with_proof=%d totp_missing_proof=%d recovery_invalid_proof=%d",
		stats.InvalidTotal,
		stats.UnknownMethod,
		stats.EmptyMethodWithProof,
		stats.TOTPMissingProof,
		stats.RecoveryInvalidProof,
	)
	if stats.InvalidTotal > 0 {
		return fmt.Errorf("identity session mfa proof audit found %d invalid rows", stats.InvalidTotal)
	}
	return nil
}

type challengeRequestLimitCleanupConfig struct {
	Retention time.Duration
	BatchSize int
}

func challengeRequestLimitCleanupConfigFromEnv() (challengeRequestLimitCleanupConfig, error) {
	retention, err := envPositiveDuration("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_RETENTION", 24*time.Hour)
	if err != nil {
		return challengeRequestLimitCleanupConfig{}, err
	}
	batchSize, err := envPositiveInt("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_BATCH_SIZE", 5000)
	if err != nil {
		return challengeRequestLimitCleanupConfig{}, err
	}
	config := challengeRequestLimitCleanupConfig{
		Retention: retention,
		BatchSize: batchSize,
	}
	return config, nil
}

func startDebugServer(ctx context.Context, addr string, handler http.Handler) (func(), error) {
	if strings.TrimSpace(addr) == "" {
		return func() {}, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: handler}
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		defer close(done)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("identity-service debug server stopped with error: %v", err)
		}
	}()
	log.Printf("identity-service debug server started on %s", addr)
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-done
	}, nil
}

func openPGPool(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return nil, errors.New("NEXUSIM_PG_DSN is required")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns := envInt("NEXUSIM_IDENTITY_PG_MAX_CONNS", 0); maxConns > 0 {
		config.MaxConns = int32(maxConns)
	}
	return pgxpool.NewWithConfig(ctx, config)
}

func identityDebugAddr() string {
	return envString("NEXUSIM_IDENTITY_DEBUG_ADDR", envString("NEXUSIM_DEBUG_ADDR", ""))
}

func newGRPCServer(grpcMetrics *monitoringinfra.GRPCMetrics) (*grpc.Server, error) {
	options := make([]grpc.ServerOption, 0, 2)
	interceptors := make([]grpc.UnaryServerInterceptor, 0, 2)
	if grpcMetrics != nil {
		interceptors = append(interceptors, grpcMetrics.UnaryServerInterceptor(log.Default()))
	}
	switch strings.ToLower(envString("NEXUSIM_IDENTITY_ADMIN_AUTH_MODE", "body")) {
	case "body", "request", "legacy":
	case "metadata", "verified-metadata":
		interceptors = append(interceptors, identitygrpc.VerifiedAdminUnaryInterceptor(true))
	default:
		return nil, errors.New("unsupported NEXUSIM_IDENTITY_ADMIN_AUTH_MODE")
	}
	if len(interceptors) > 0 {
		options = append(options, grpc.ChainUnaryInterceptor(interceptors...))
	}
	if creds, ok, err := loadIdentityGRPCCredentialsFromEnv(); err != nil {
		return nil, err
	} else if ok {
		options = append(options, grpc.Creds(creds))
	}
	return grpc.NewServer(options...), nil
}

func loadIdentityGRPCCredentialsFromEnv() (credentials.TransportCredentials, bool, error) {
	tlsConfig, ok, err := identityGRPCTLSConfigFromEnv()
	if err != nil || !ok {
		return nil, ok, err
	}
	return credentials.NewTLS(tlsConfig), true, nil
}

func identityGRPCTLSConfigFromEnv() (*tls.Config, bool, error) {
	certFile := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE"))
	allowedClientDNSNames := envStringSet("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", strings.ToLower)
	allowedClientURIs, err := envURIStringSet("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_ALLOWED_URIS")
	if err != nil {
		return nil, true, err
	}
	requireClientCert, requireClientCertConfigured, err := envOptionalBool("NEXUSIM_IDENTITY_GRPC_TLS_REQUIRE_CLIENT_CERT")
	if err != nil {
		return nil, true, err
	}
	hasClientAllowlist := len(allowedClientDNSNames) > 0 || len(allowedClientURIs) > 0
	requireClientCert = clientCAFile != "" || hasClientAllowlist || (requireClientCertConfigured && requireClientCert)
	if certFile == "" && keyFile == "" && clientCAFile == "" && !requireClientCert && !hasClientAllowlist {
		return nil, false, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, true, errors.New("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE and NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE must be configured together")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, true, err
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if requireClientCert {
		if clientCAFile == "" {
			return nil, true, errors.New("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE is required when client certificates are required")
		}
		pemBytes, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, true, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, true, errors.New("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE does not contain a valid PEM certificate")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if hasClientAllowlist {
			tlsConfig.VerifyConnection = verifyAllowedIdentityGRPCClient(allowedClientDNSNames, allowedClientURIs)
		}
	}
	return tlsConfig, true, nil
}

func verifyAllowedIdentityGRPCClient(allowedDNSNames map[string]struct{}, allowedURIs map[string]struct{}) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("identity grpc client certificate is required")
		}
		cert := state.PeerCertificates[0]
		for _, dnsName := range cert.DNSNames {
			if _, ok := allowedDNSNames[strings.ToLower(strings.TrimSpace(dnsName))]; ok {
				return nil
			}
		}
		for _, uri := range cert.URIs {
			if uri == nil {
				continue
			}
			if _, ok := allowedURIs[uri.String()]; ok {
				return nil
			}
		}
		return errors.New("identity grpc client certificate identity is not allowed")
	}
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envStringSet(name string, normalize func(string) string) map[string]struct{} {
	values := make(map[string]struct{})
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return values
	}
	for _, item := range strings.Split(raw, ",") {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if normalize != nil {
			value = normalize(value)
		}
		values[value] = struct{}{}
	}
	return values
}

func envURIStringSet(name string) (map[string]struct{}, error) {
	values := make(map[string]struct{})
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return values, nil
	}
	for _, item := range strings.Split(raw, ",") {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" && parsed.Opaque == "" {
			return nil, fmt.Errorf("%s contains an invalid URI", name)
		}
		values[parsed.String()] = struct{}{}
	}
	return values, nil
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envPositiveInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func envOptionalBool(name string) (bool, bool, error) {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if value == "" {
		return false, false, nil
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true, true, nil
	case "0", "false", "no", "n", "off":
		return false, true, nil
	default:
		return false, true, fmt.Errorf("%s must be a boolean", name)
	}
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envPositiveDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsed, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseInt64CSV(value string) ([]int64, error) {
	parts := strings.Split(value, ",")
	result := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parsed, err := strconv.ParseInt(part, 10, 64)
		if err != nil || parsed <= 0 {
			return nil, errors.New("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_IDS must contain positive integer ids")
		}
		result = append(result, parsed)
	}
	return result, nil
}

func optionalPositiveInt64Env(name string) (*int64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return nil, fmt.Errorf("%s must be a positive integer", name)
	}
	return &parsed, nil
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}
