package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestRequestVerificationChallengeUseCaseRequiresCurrentPassword(t *testing.T) {
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
	}
	verifier := &fakePasswordVerifier{ok: false}
	notifier := &fakeChallengeNotifier{}
	useCase := NewRequestVerificationChallengeUseCase(repository, fakeChallengeTokenCodec{}, verifier, ChallengeOptions{ReturnDevToken: true, Notifier: notifier})
	_, err := useCase.Execute(context.Background(), types.RequestVerificationChallengeCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
		Password:    "wrong",
	})
	if !errors.Is(err, types.ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	if repository.createVerificationCalled {
		t.Fatal("verification challenge should not be created after invalid password")
	}
	if notifier.called {
		t.Fatal("verification challenge should not be sent after invalid password")
	}

	verifier.ok = true
	result, err := useCase.Execute(context.Background(), types.RequestVerificationChallengeCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
		Password:    "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("request verification: %v", err)
	}
	if !repository.createVerificationCalled || result.DevChallengeToken != "challenge-token" {
		t.Fatalf("expected challenge creation with dev token, result=%+v called=%v", result, repository.createVerificationCalled)
	}
	if !notifier.called || notifier.notification.Token != "challenge-token" || notifier.notification.Type != types.ChallengeTypeEmailVerification {
		t.Fatalf("expected verification notification with challenge token, got called=%v notification=%+v", notifier.called, notifier.notification)
	}
	if !repository.deliverySuccessCalled || repository.deliverySuccessChallengeID != "challenge-1" {
		t.Fatalf("expected verification challenge delivery success to be recorded, called=%v challenge=%q", repository.deliverySuccessCalled, repository.deliverySuccessChallengeID)
	}
}

func TestRequestVerificationChallengeUseCaseReturnsDeliveryFailure(t *testing.T) {
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
	}
	notifier := &fakeChallengeNotifier{err: types.NewChallengeDeliveryFailed("webhook failed")}
	useCase := NewRequestVerificationChallengeUseCase(repository, fakeChallengeTokenCodec{}, &fakePasswordVerifier{ok: true}, ChallengeOptions{Notifier: notifier})
	_, err := useCase.Execute(context.Background(), types.RequestVerificationChallengeCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
		Password:    "correct horse battery staple",
	})
	if !errors.Is(err, types.ErrChallengeDeliveryFailed) {
		t.Fatalf("expected delivery failure, got %v", err)
	}
	if !repository.createVerificationCalled || !notifier.called {
		t.Fatalf("expected challenge to be created then sent, created=%v sent=%v", repository.createVerificationCalled, notifier.called)
	}
	if !repository.deliveryFailureCalled || repository.deliveryFailureChallengeID != "challenge-1" || repository.deliveryFailureLastError != "challenge delivery unavailable" {
		t.Fatalf("expected failed delivery challenge to be recorded, called=%v challenge=%q error=%q", repository.deliveryFailureCalled, repository.deliveryFailureChallengeID, repository.deliveryFailureLastError)
	}
	if repository.expireChallengeCalled {
		t.Fatal("delivery failure should be recorded through the delivery failure path, not bare ExpireChallenge")
	}
}

func TestRequestVerificationChallengeUseCaseEnqueuesDeliveryOutbox(t *testing.T) {
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
	}
	notifier := &fakeChallengeNotifier{}
	deliveryTokens := &fakeChallengeDeliveryTokenCodec{
		encrypted: types.EncryptedChallengeToken{
			Ciphertext: "encrypted-token",
			Nonce:      "nonce-value",
			KeyVersion: "local-v1",
		},
	}
	useCase := NewRequestVerificationChallengeUseCase(
		repository,
		fakeChallengeTokenCodec{},
		&fakePasswordVerifier{ok: true},
		ChallengeOptions{
			ReturnDevToken:     true,
			Notifier:           notifier,
			DeliveryOutbox:     true,
			DeliveryTokenCodec: deliveryTokens,
		},
	)
	result, err := useCase.Execute(context.Background(), types.RequestVerificationChallengeCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
		Password:    "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("request verification outbox: %v", err)
	}
	if !repository.createVerificationCalled || result.DevChallengeToken != "challenge-token" {
		t.Fatalf("expected challenge creation with dev token, result=%+v called=%v", result, repository.createVerificationCalled)
	}
	if notifier.called || repository.deliverySuccessCalled || repository.deliveryFailureCalled {
		t.Fatalf("outbox mode must not send synchronously or record sync delivery state, notifier=%v success=%v failure=%v", notifier.called, repository.deliverySuccessCalled, repository.deliveryFailureCalled)
	}
	if deliveryTokens.token != "challenge-token" || repository.createVerificationDelivery.EncryptedToken.Ciphertext != "encrypted-token" {
		t.Fatalf("expected encrypted delivery record from raw token, token=%q delivery=%+v", deliveryTokens.token, repository.createVerificationDelivery)
	}
}

func TestConfirmPasswordResetUseCaseHashesNewPassword(t *testing.T) {
	repository := &fakeIdentityRepository{}
	useCase := NewConfirmPasswordResetUseCase(repository, fakeChallengeTokenCodec{}, fakePasswordHasher{hash: "new-password-hash"})
	_, err := useCase.Execute(context.Background(), types.ConfirmPasswordResetCommand{
		TenantID:       "tenant-1",
		UserID:         "user-1",
		ChallengeID:    "challenge-1",
		ChallengeToken: "challenge-token",
		NewPassword:    "new correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("confirm password reset: %v", err)
	}
	if !repository.confirmPasswordResetCalled || repository.resetPasswordHash != "new-password-hash" || repository.resetTokenHash == "" {
		t.Fatalf("expected hashed password reset, called=%v password=%q token=%q", repository.confirmPasswordResetCalled, repository.resetPasswordHash, repository.resetTokenHash)
	}
}

func TestRequestPasswordResetUseCaseNeverReturnsDevToken(t *testing.T) {
	repository := &fakeIdentityRepository{}
	notifier := &fakeChallengeNotifier{}
	useCase := NewRequestPasswordResetUseCase(repository, fakeChallengeTokenCodec{}, ChallengeOptions{ReturnDevToken: true, Notifier: notifier})
	result, err := useCase.Execute(context.Background(), types.RequestPasswordResetCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	})
	if err != nil {
		t.Fatalf("request password reset: %v", err)
	}
	if !repository.createPasswordResetCalled {
		t.Fatal("expected repository to create reset challenge")
	}
	if result.ChallengeID != "challenge-1" || result.DevChallengeToken != "" {
		t.Fatalf("password reset response must not expose dev token: %+v", result)
	}
	if !notifier.called || notifier.notification.Token != "challenge-token" || notifier.notification.Type != types.ChallengeTypePasswordReset {
		t.Fatalf("expected password reset notification with challenge token, got called=%v notification=%+v", notifier.called, notifier.notification)
	}
	if !repository.deliverySuccessCalled || repository.deliverySuccessChallengeID != "challenge-1" {
		t.Fatalf("expected password reset delivery success to be recorded, called=%v challenge=%q", repository.deliverySuccessCalled, repository.deliverySuccessChallengeID)
	}
}

func TestRequestPasswordResetUseCaseRecordsChallengeDeliveryFailure(t *testing.T) {
	repository := &fakeIdentityRepository{}
	notifier := &fakeChallengeNotifier{err: types.NewChallengeDeliveryFailed("webhook failed")}
	useCase := NewRequestPasswordResetUseCase(repository, fakeChallengeTokenCodec{}, ChallengeOptions{Notifier: notifier})
	_, err := useCase.Execute(context.Background(), types.RequestPasswordResetCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	})
	if !errors.Is(err, types.ErrChallengeDeliveryFailed) {
		t.Fatalf("expected delivery failure, got %v", err)
	}
	if !repository.createPasswordResetCalled || !notifier.called {
		t.Fatalf("expected reset challenge to be created then sent, created=%v sent=%v", repository.createPasswordResetCalled, notifier.called)
	}
	if !repository.deliveryFailureCalled || repository.deliveryFailureChallengeID != "challenge-1" || repository.deliveryFailureLastError != "challenge delivery unavailable" {
		t.Fatalf("expected failed reset delivery to be recorded, called=%v challenge=%q error=%q", repository.deliveryFailureCalled, repository.deliveryFailureChallengeID, repository.deliveryFailureLastError)
	}
	if repository.expireChallengeCalled {
		t.Fatal("delivery failure should be recorded through the delivery failure path, not bare ExpireChallenge")
	}
}

func TestRequestPasswordResetUseCaseEnqueuesDeliveryOutboxWithoutDevToken(t *testing.T) {
	repository := &fakeIdentityRepository{}
	notifier := &fakeChallengeNotifier{}
	deliveryTokens := &fakeChallengeDeliveryTokenCodec{
		encrypted: types.EncryptedChallengeToken{
			Ciphertext: "encrypted-reset-token",
			Nonce:      "reset-nonce",
			KeyVersion: "local-v1",
		},
	}
	useCase := NewRequestPasswordResetUseCase(repository, fakeChallengeTokenCodec{}, ChallengeOptions{
		ReturnDevToken:     true,
		Notifier:           notifier,
		DeliveryOutbox:     true,
		DeliveryTokenCodec: deliveryTokens,
	})
	result, err := useCase.Execute(context.Background(), types.RequestPasswordResetCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	})
	if err != nil {
		t.Fatalf("request password reset outbox: %v", err)
	}
	if !repository.createPasswordResetCalled || result.DevChallengeToken != "" {
		t.Fatalf("expected password reset outbox without dev token, result=%+v called=%v", result, repository.createPasswordResetCalled)
	}
	if notifier.called || repository.deliverySuccessCalled || repository.deliveryFailureCalled {
		t.Fatalf("outbox mode must not send synchronously or record sync delivery state, notifier=%v success=%v failure=%v", notifier.called, repository.deliverySuccessCalled, repository.deliveryFailureCalled)
	}
	if deliveryTokens.token != "challenge-token" || repository.createPasswordResetDelivery.EncryptedToken.Ciphertext != "encrypted-reset-token" {
		t.Fatalf("expected encrypted reset delivery record from raw token, token=%q delivery=%+v", deliveryTokens.token, repository.createPasswordResetDelivery)
	}
}

func TestRequestPasswordResetUseCaseHidesInvalidOrRateLimitedTarget(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "invalid credentials", err: types.NewInvalidCredentials("invalid credentials")},
		{name: "rate limited", err: types.NewChallengeRateLimited("too many active challenges")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repository := &fakeIdentityRepository{createPasswordResetErr: tc.err}
			notifier := &fakeChallengeNotifier{}
			useCase := NewRequestPasswordResetUseCase(repository, fakeChallengeTokenCodec{}, ChallengeOptions{ReturnDevToken: true, Notifier: notifier})
			useCase.now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }
			result, err := useCase.Execute(context.Background(), types.RequestPasswordResetCommand{
				TenantID:    "tenant-1",
				UserID:      "user-1",
				Channel:     types.VerificationChannelEmail,
				Destination: "user1@example.com",
				TTLSeconds:  600,
			})
			if err != nil {
				t.Fatalf("request password reset should return neutral success: %v", err)
			}
			if !repository.createPasswordResetCalled {
				t.Fatal("expected repository to be asked to create reset challenge")
			}
			if result.ChallengeID != "challenge-1" || result.DevChallengeToken != "" || result.ExpiresAtUnixMS != time.Unix(1_800_000_600, 0).UnixMilli() {
				t.Fatalf("unexpected neutral result: %+v", result)
			}
			if notifier.called {
				t.Fatalf("neutral password reset response must not send challenge notification: %+v", notifier.notification)
			}
		})
	}
}

func TestRequestPasswordResetUseCaseHidesRequestLimiterAndSkipsDelivery(t *testing.T) {
	repository := &fakeIdentityRepository{recordPasswordResetRequestErr: types.NewChallengeRateLimited("challenge request temporarily limited")}
	notifier := &fakeChallengeNotifier{}
	useCase := NewRequestPasswordResetUseCase(repository, fakeChallengeTokenCodec{}, ChallengeOptions{
		ReturnDevToken:            true,
		Notifier:                  notifier,
		RequestLimitTargetKeySeed: "request-limit-secret",
	})
	useCase.now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }
	result, err := useCase.Execute(context.Background(), types.RequestPasswordResetCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "User1@Example.COM",
		TTLSeconds:  600,
	})
	if err != nil {
		t.Fatalf("request password reset should return neutral success: %v", err)
	}
	if result.ChallengeID != "challenge-1" ||
		result.DevChallengeToken != "" ||
		result.ExpiresAtUnixMS != time.Unix(1_800_000_600, 0).UnixMilli() {
		t.Fatalf("unexpected neutral result: %+v", result)
	}
	if !repository.recordPasswordResetRequestCalled || repository.recordPasswordResetRequestTargetKey == "" {
		t.Fatalf("expected request limiter target key, called=%v key=%q", repository.recordPasswordResetRequestCalled, repository.recordPasswordResetRequestTargetKey)
	}
	if strings.Contains(repository.recordPasswordResetRequestTargetKey, "User1") ||
		strings.Contains(repository.recordPasswordResetRequestTargetKey, "example.com") {
		t.Fatalf("target key should not contain raw destination: %q", repository.recordPasswordResetRequestTargetKey)
	}
	if repository.createPasswordResetCalled || notifier.called {
		t.Fatalf("limited neutral response must not create challenge or send notification, created=%v sent=%v", repository.createPasswordResetCalled, notifier.called)
	}
}

func TestPasswordResetRequestTargetKeyNormalizesEmailAndHidesDestination(t *testing.T) {
	options := ChallengeOptions{RequestLimitTargetKeySeed: "request-limit-secret"}
	base := types.RequestPasswordResetCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: " User1@Example.COM ",
	}
	normalized := base
	normalized.Destination = "user1@example.com"

	key := passwordResetRequestTargetKey(options, base)
	normalizedKey := passwordResetRequestTargetKey(options, normalized)
	if key == "" || key != normalizedKey {
		t.Fatalf("expected stable normalized target key, got %q and %q", key, normalizedKey)
	}
	if strings.Contains(key, "User1") || strings.Contains(key, "example.com") {
		t.Fatalf("target key should not contain raw destination: %q", key)
	}
	if disabled := passwordResetRequestTargetKey(ChallengeOptions{}, base); disabled != "" {
		t.Fatalf("expected empty key when limiter secret is unset, got %q", disabled)
	}
}
