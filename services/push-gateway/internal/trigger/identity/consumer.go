package identity

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	identityeventsv1 "github.com/qsyy0921/IM/schemas/kafka/identity/v1"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"google.golang.org/protobuf/proto"
)

const (
	TopicIdentityEvents         = "im.identity.events"
	EventIdentityDeviceRevoked  = "identity.device.revoked.v1"
	EventIdentitySessionRevoked = "identity.session.revoked.v1"
)

type Consumer interface {
	Fetch(context.Context) (types.DeliveryEventMessage, error)
	Commit(context.Context, types.DeliveryEventMessage) error
}

type Recorder interface {
	RevokeDevice(ctx context.Context, tenantID string, userID string, deviceID string) error
	RevokeSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string) error
}

type Worker struct {
	consumer Consumer
	recorder Recorder
	config   Config
	metrics  workerMetrics
}

type Config struct {
	ErrorBackoff time.Duration
	Logf         func(format string, args ...any)
}

type workerMetrics struct {
	totalErrors        atomic.Uint64
	consecutiveErrors  atomic.Uint64
	lastErrorAtMS      atomic.Int64
	lastSuccessAtMS    atomic.Int64
	lastCommitAtMS     atomic.Int64
	lastErrorBackoffMS atomic.Int64
}

func NewWorker(consumer Consumer, recorder Recorder, configs ...Config) *Worker {
	var config Config
	if len(configs) > 0 {
		config = configs[0]
	}
	return &Worker{consumer: consumer, recorder: recorder, config: normalizeConfig(config)}
}

func (worker *Worker) Run(ctx context.Context) error {
	if worker.consumer == nil {
		return errors.New("push identity consumer is not configured")
	}
	if worker.recorder == nil {
		return errors.New("push identity recorder is not configured")
	}
	for {
		err := worker.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if isPermanentError(err) {
				return err
			}
			if worker.config.Logf != nil {
				worker.config.Logf("push-gateway identity consumer retrying after error: %v", err)
			}
			worker.recordError()
			worker.metrics.lastErrorBackoffMS.Store(worker.config.ErrorBackoff.Milliseconds())
			if err := waitForInterval(ctx, worker.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		worker.recordSuccess()
	}
}

func (worker *Worker) RunOnce(ctx context.Context) error {
	if worker.consumer == nil {
		return errors.New("push identity consumer is not configured")
	}
	if worker.recorder == nil {
		return errors.New("push identity recorder is not configured")
	}
	message, err := worker.consumer.Fetch(ctx)
	if err != nil {
		return err
	}
	if err := worker.apply(ctx, message); err != nil {
		return err
	}
	if err := worker.consumer.Commit(ctx, message); err != nil {
		return err
	}
	return nil
}

func (worker *Worker) Snapshot() types.ConsumerWorkerSnapshot {
	return types.ConsumerWorkerSnapshot{
		TotalErrors:        worker.metrics.totalErrors.Load(),
		ConsecutiveErrors:  worker.metrics.consecutiveErrors.Load(),
		LastErrorAtMS:      worker.metrics.lastErrorAtMS.Load(),
		LastSuccessAtMS:    worker.metrics.lastSuccessAtMS.Load(),
		LastCommitAtMS:     worker.metrics.lastCommitAtMS.Load(),
		LastErrorBackoffMS: worker.metrics.lastErrorBackoffMS.Load(),
	}
}

func normalizeConfig(config Config) Config {
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	return config
}

func isPermanentError(err error) bool {
	return errors.Is(err, types.ErrInvalidFrame) ||
		errors.Is(err, types.ErrUnsupportedIdentityEvent)
}

func (worker *Worker) recordError() {
	worker.metrics.totalErrors.Add(1)
	worker.metrics.consecutiveErrors.Add(1)
	worker.metrics.lastErrorAtMS.Store(time.Now().UnixMilli())
}

func (worker *Worker) recordSuccess() {
	worker.metrics.consecutiveErrors.Store(0)
	now := time.Now().UnixMilli()
	worker.metrics.lastSuccessAtMS.Store(now)
	worker.metrics.lastCommitAtMS.Store(now)
}

func waitForInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (worker *Worker) apply(ctx context.Context, message types.DeliveryEventMessage) error {
	var event identityeventsv1.IdentityEvent
	if err := proto.Unmarshal(message.Value, &event); err != nil {
		return err
	}
	if event.GetEventId() == "" ||
		event.GetEventType() == "" ||
		event.GetEventVersion() == "" ||
		event.GetTenantId() == "" ||
		event.GetAggregateId() == "" ||
		event.GetPartitionKey() == "" {
		return types.NewInvalidFrame("identity event envelope is incomplete")
	}
	switch payload := event.GetPayload().(type) {
	case *identityeventsv1.IdentityEvent_DeviceRevoked:
		if event.GetEventType() != EventIdentityDeviceRevoked {
			return types.NewInvalidFrame("identity event type mismatch")
		}
		revoked := payload.DeviceRevoked
		if revoked == nil ||
			revoked.GetTenantId() == "" ||
			revoked.GetUserId() == "" ||
			revoked.GetDeviceId() == "" {
			return types.NewInvalidFrame("identity device revoked payload is incomplete")
		}
		if event.GetTenantId() != revoked.GetTenantId() {
			return types.NewInvalidFrame("identity event envelope mismatch")
		}
		return worker.recorder.RevokeDevice(ctx, revoked.GetTenantId(), revoked.GetUserId(), revoked.GetDeviceId())
	case *identityeventsv1.IdentityEvent_SessionRevoked:
		if event.GetEventType() != EventIdentitySessionRevoked {
			return types.NewInvalidFrame("identity event type mismatch")
		}
		revoked := payload.SessionRevoked
		if revoked == nil ||
			revoked.GetTenantId() == "" ||
			revoked.GetUserId() == "" ||
			revoked.GetDeviceId() == "" ||
			revoked.GetSessionId() == "" {
			return types.NewInvalidFrame("identity session revoked payload is incomplete")
		}
		if event.GetTenantId() != revoked.GetTenantId() {
			return types.NewInvalidFrame("identity event envelope mismatch")
		}
		return worker.recorder.RevokeSession(ctx, revoked.GetTenantId(), revoked.GetUserId(), revoked.GetDeviceId(), revoked.GetSessionId())
	default:
		return types.ErrUnsupportedIdentityEvent
	}
}
