package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
)

type capacityIdentityUser struct {
	userID   string
	deviceID string
	password string
}

type capacityRecorder struct {
	mu             sync.Mutex
	operations     int
	tokenIssues    int
	expectedErrors int
	latenciesMS    []float64
}

func (rec *capacityRecorder) record(start time.Time, tokenIssues int) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.operations++
	rec.tokenIssues += tokenIssues
	rec.latenciesMS = append(rec.latenciesMS, elapsedMS(start))
}

func (rec *capacityRecorder) apply(result *summary) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	result.capacityOperationCount = rec.operations
	result.capacityTokenIssueCount = rec.tokenIssues
	result.capacityExpectedErrorCount = rec.expectedErrors
	result.capacityLatencySamples = append([]float64(nil), rec.latenciesMS...)
}

func runCapacityScenario(ctx context.Context, cfg config, client identityChallengeClient, pool *pgxpool.Pool, result *summary) error {
	if cfg.duration <= 0 {
		return errors.New("identity capacity duration must be positive")
	}
	if cfg.vus <= 0 {
		return errors.New("identity capacity vus must be positive")
	}

	recorder := &capacityRecorder{}
	users := make([]capacityIdentityUser, 0, cfg.vus)
	for index := 0; index < cfg.vus; index++ {
		user, err := setupCapacityIdentityUser(ctx, cfg, client, index, recorder)
		if err != nil {
			recorder.apply(result)
			return err
		}
		users = append(users, user)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	deadline := time.Now().Add(cfg.duration)
	errCh := make(chan error, cfg.vus)
	var wg sync.WaitGroup
	for _, user := range users {
		user := user
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if time.Now().After(deadline) {
					return
				}
				if err := runCapacityIdentityIteration(runCtx, cfg, client, user, recorder); err != nil {
					select {
					case errCh <- err:
					default:
					}
					cancel()
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	recorder.apply(result)
	if err := firstCapacityError(errCh); err != nil {
		return err
	}

	if pool != nil {
		if err := fillCapacityPostgresStats(ctx, pool, cfg, result); err != nil {
			return err
		}
		if result.ChallengeDeliveryOutbox.Pending != 0 || result.ChallengeDeliveryOutbox.DLQ != 0 {
			return fmt.Errorf("challenge delivery outbox did not drain during capacity run: %+v", result.ChallengeDeliveryOutbox)
		}
	}
	return nil
}

func setupCapacityIdentityUser(ctx context.Context, cfg config, client identityChallengeClient, index int, recorder *capacityRecorder) (capacityIdentityUser, error) {
	user := capacityIdentityUser{
		userID:   fmt.Sprintf("%s-vu-%02d", strings.TrimSpace(cfg.userID), index+1),
		deviceID: fmt.Sprintf("%s-vu-%02d", strings.TrimSpace(cfg.deviceID), index+1),
		password: cfg.password,
	}
	if user.userID == "-vu-01" {
		user.userID = fmt.Sprintf("identity-user-vu-%02d", index+1)
	}
	if user.deviceID == "-vu-01" {
		user.deviceID = fmt.Sprintf("identity-device-vu-%02d", index+1)
	}

	registerStarted := time.Now()
	registerCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	_, err := client.RegisterUser(registerCtx, &identityv1.RegisterUserRequest{
		TenantId:  cfg.tenantID,
		UserId:    user.userID,
		Password:  user.password,
		TraceId:   "identity-capacity",
		RequestId: fmt.Sprintf("identity-capacity-register-%02d", index+1),
	})
	cancel()
	recorder.record(registerStarted, 0)
	if err != nil {
		return capacityIdentityUser{}, fmt.Errorf("capacity register user %s: %w", user.userID, err)
	}
	return user, nil
}

func runCapacityIdentityIteration(ctx context.Context, cfg config, client identityChallengeClient, user capacityIdentityUser, recorder *capacityRecorder) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	loginStarted := time.Now()
	loginCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	login, err := client.Login(loginCtx, &identityv1.LoginRequest{
		TenantId:          cfg.tenantID,
		UserId:            user.userID,
		Password:          user.password,
		DeviceId:          user.deviceID,
		Audience:          cfg.audience,
		GatewayTtlSeconds: 900,
		RefreshTtlSeconds: 3600,
		TraceId:           "identity-capacity",
		RequestId:         fmt.Sprintf("identity-capacity-login-%s-%d", user.userID, time.Now().UnixNano()),
	})
	cancel()
	recorder.record(loginStarted, 1)
	if err != nil {
		return fmt.Errorf("capacity login %s: %w", user.userID, err)
	}
	if login.GetGatewayToken() == "" || login.GetRefreshToken() == "" || login.GetSessionId() == "" {
		return fmt.Errorf("capacity login %s did not return token/session fields", user.userID)
	}

	refreshStarted := time.Now()
	refreshCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	refresh, err := client.RefreshGatewayToken(refreshCtx, &identityv1.RefreshGatewayTokenRequest{
		TenantId:          cfg.tenantID,
		UserId:            user.userID,
		DeviceId:          user.deviceID,
		RefreshToken:      login.GetRefreshToken(),
		Audience:          cfg.audience,
		GatewayTtlSeconds: 900,
		RefreshTtlSeconds: 3600,
		TraceId:           "identity-capacity",
		RequestId:         fmt.Sprintf("identity-capacity-refresh-%s-%d", user.userID, time.Now().UnixNano()),
	})
	cancel()
	recorder.record(refreshStarted, 1)
	if err != nil {
		return fmt.Errorf("capacity refresh %s: %w", user.userID, err)
	}
	if refresh.GetGatewayToken() == "" || refresh.GetRefreshToken() == "" || refresh.GetSessionId() == "" {
		return fmt.Errorf("capacity refresh %s did not return token/session fields", user.userID)
	}
	if refresh.GetRefreshToken() == login.GetRefreshToken() {
		return fmt.Errorf("capacity refresh %s did not rotate refresh token", user.userID)
	}
	return nil
}

func firstCapacityError(errCh <-chan error) error {
	for err := range errCh {
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	return nil
}

func fillCapacityPostgresStats(ctx context.Context, pool *pgxpool.Pool, cfg config, result *summary) error {
	rows, err := pool.Query(ctx, `
SELECT status, COUNT(*)
FROM identity_challenge_delivery_outbox
WHERE tenant_id = $1
GROUP BY status
`, cfg.tenantID)
	if err != nil {
		return fmt.Errorf("query capacity challenge delivery outbox stats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return fmt.Errorf("scan capacity challenge delivery outbox stats: %w", err)
		}
		result.ChallengeDeliveryOutbox.Total += count
		switch status {
		case "PENDING":
			result.ChallengeDeliveryOutbox.Pending = count
		case "DELIVERED":
			result.ChallengeDeliveryOutbox.Delivered = count
		case "DLQ":
			result.ChallengeDeliveryOutbox.DLQ = count
		case "CANCELED":
			result.ChallengeDeliveryOutbox.Canceled = count
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate capacity challenge delivery outbox stats: %w", err)
	}

	if err := pool.QueryRow(ctx, `
SELECT COALESCE(SUM(delivery_attempt_count), 0)
FROM identity_challenges
WHERE tenant_id = $1
`, cfg.tenantID).Scan(&result.ChallengeRow.DeliveryAttemptCount); err != nil {
		return fmt.Errorf("query capacity challenge delivery attempt count: %w", err)
	}
	return nil
}
