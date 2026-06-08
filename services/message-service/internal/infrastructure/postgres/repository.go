package postgres

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type MessageRepository struct {
	pool      *pgxpool.Pool
	now       func() time.Time
	messageID func() (types.MessageID, error)
	eventID   func() (types.EventID, error)
}

type MessageRepositoryOption func(*MessageRepository)

func NewMessageRepository(pool *pgxpool.Pool, opts ...MessageRepositoryOption) *MessageRepository {
	repo := &MessageRepository{
		pool: pool,
		now:  func() time.Time { return time.Now().UTC() },
		messageID: func() (types.MessageID, error) {
			id, err := newUUIDString()
			if err != nil {
				return "", err
			}
			return types.MessageID("msg_" + id), nil
		},
		eventID: func() (types.EventID, error) {
			id, err := newUUIDString()
			if err != nil {
				return "", err
			}
			return types.EventID(id), nil
		},
	}
	for _, opt := range opts {
		opt(repo)
	}
	return repo
}

func WithClock(clock func() time.Time) MessageRepositoryOption {
	return func(repo *MessageRepository) {
		if clock != nil {
			repo.now = clock
		}
	}
}

func WithIDGenerators(
	messageID func() (types.MessageID, error),
	eventID func() (types.EventID, error),
) MessageRepositoryOption {
	return func(repo *MessageRepository) {
		if messageID != nil {
			repo.messageID = messageID
		}
		if eventID != nil {
			repo.eventID = eventID
		}
	}
}

func (r *MessageRepository) AppendMessage(ctx context.Context, input domain.AppendMessageInput) (domain.AppendMessageResult, error) {
	if r.pool == nil {
		return domain.AppendMessageResult{}, ErrRepositoryNotConfigured
	}
	commandHash, err := domain.ComputeSendMessageCommandHash(input.Command)
	if err != nil {
		return domain.AppendMessageResult{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AppendMessageResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := r.lockCommandIdempotency(ctx, tx, input.Command); err != nil {
		return domain.AppendMessageResult{}, err
	}

	if existing, ok, err := r.findExistingMessage(ctx, tx, input.Command); err != nil {
		return domain.AppendMessageResult{}, err
	} else if ok {
		return replayResult(ctx, tx, existing, commandHash)
	}

	if err := r.ensureConversationSeq(ctx, tx, input); err != nil {
		return domain.AppendMessageResult{}, err
	}

	seq, err := r.allocateConversationSeq(ctx, tx, input)
	if err != nil {
		return domain.AppendMessageResult{}, err
	}

	messageID, err := r.messageID()
	if err != nil {
		return domain.AppendMessageResult{}, err
	}
	eventID, err := r.eventID()
	if err != nil {
		return domain.AppendMessageResult{}, err
	}
	acceptedAt := r.now()
	record, err := domain.NewAppendMessageRecord(input, messageID, eventID, seq, acceptedAt)
	if err != nil {
		return domain.AppendMessageResult{}, err
	}

	if err := r.insertMessage(ctx, tx, record); err != nil {
		return domain.AppendMessageResult{}, err
	}
	if err := r.insertTimelineEvent(ctx, tx, input, record); err != nil {
		return domain.AppendMessageResult{}, err
	}
	if err := r.insertOutboxEvent(ctx, tx, record); err != nil {
		return domain.AppendMessageResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AppendMessageResult{}, types.NewDBWriteFailed(err.Error())
	}

	return domain.AppendMessageResult{
		MessageID:        record.Message.MessageID,
		ConversationSeq:  record.Message.Seq,
		AcceptedAt:       record.Message.CreatedAt,
		IdempotentReplay: false,
	}, nil
}

type existingMessage struct {
	MessageID       types.MessageID
	ConversationSeq int64
	CommandHash     string
	CreatedAt       time.Time
}

func (r *MessageRepository) lockCommandIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	command types.SendMessageCommand,
) error {
	_, err := tx.Exec(ctx, `
SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
`, idempotencyLockKey(command))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func idempotencyLockKey(command types.SendMessageCommand) string {
	return fmt.Sprintf(
		"%s\x1f%s\x1f%s\x1f%s",
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.AuthContext.DeviceID,
		command.ClientMsgID,
	)
}

func (r *MessageRepository) findExistingMessage(
	ctx context.Context,
	tx pgx.Tx,
	command types.SendMessageCommand,
) (existingMessage, bool, error) {
	row := tx.QueryRow(ctx, `
SELECT message_id, conversation_seq, command_hash, created_at
FROM message_log
WHERE tenant_id = $1
  AND sender_id = $2
  AND device_id = $3
  AND client_msg_id = $4
FOR UPDATE
`,
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.AuthContext.DeviceID,
		command.ClientMsgID,
	)
	var existing existingMessage
	if err := row.Scan(&existing.MessageID, &existing.ConversationSeq, &existing.CommandHash, &existing.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return existingMessage{}, false, nil
		}
		return existingMessage{}, false, types.NewDBWriteFailed(err.Error())
	}
	return existing, true, nil
}

func replayResult(ctx context.Context, tx pgx.Tx, existing existingMessage, commandHash string) (domain.AppendMessageResult, error) {
	if existing.CommandHash != commandHash {
		return domain.AppendMessageResult{}, types.NewIdempotencyConflict("client_msg_id reused with different command")
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AppendMessageResult{}, types.NewDBWriteFailed(err.Error())
	}
	return domain.AppendMessageResult{
		MessageID:        existing.MessageID,
		ConversationSeq:  existing.ConversationSeq,
		AcceptedAt:       existing.CreatedAt,
		IdempotentReplay: true,
	}, nil
}

func (r *MessageRepository) ensureConversationSeq(ctx context.Context, tx pgx.Tx, input domain.AppendMessageInput) error {
	_, err := tx.Exec(ctx, `
INSERT INTO conversation_seq (tenant_id, conversation_id, current_seq)
VALUES ($1, $2, 0)
ON CONFLICT (tenant_id, conversation_id) DO NOTHING
`,
		input.Command.AuthContext.TenantID,
		input.Command.ConversationID,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (r *MessageRepository) allocateConversationSeq(ctx context.Context, tx pgx.Tx, input domain.AppendMessageInput) (int64, error) {
	row := tx.QueryRow(ctx, `
UPDATE conversation_seq
SET current_seq = current_seq + 1,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
RETURNING current_seq
`,
		input.Command.AuthContext.TenantID,
		input.Command.ConversationID,
	)
	var seq int64
	if err := row.Scan(&seq); err != nil {
		return 0, types.NewDBWriteFailed(err.Error())
	}
	return seq, nil
}

func (r *MessageRepository) insertMessage(ctx context.Context, tx pgx.Tx, record domain.AppendMessageRecord) error {
	_, err := tx.Exec(ctx, `
INSERT INTO message_log (
    tenant_id,
    conversation_id,
    conversation_seq,
    message_id,
    sender_id,
    device_id,
    client_msg_id,
    command_hash,
    message_type,
    payload_json,
    status,
    permission_version,
    classification,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $12, $13, $14)
`,
		record.Message.TenantID,
		record.Message.ConversationID,
		record.Message.Seq,
		record.Message.MessageID,
		record.Message.SenderID,
		record.Message.DeviceID,
		record.Message.ClientMsgID,
		record.Message.CommandHash,
		record.Message.MessageType,
		record.Message.PayloadJSON,
		record.Message.Status,
		record.Timeline.PermissionVersion,
		record.Timeline.Classification,
		record.Message.CreatedAt,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (r *MessageRepository) insertTimelineEvent(ctx context.Context, tx pgx.Tx, input domain.AppendMessageInput, record domain.AppendMessageRecord) error {
	_, err := tx.Exec(ctx, `
INSERT INTO conversation_timeline_events (
    tenant_id,
    conversation_id,
    seq,
    event_id,
    event_type,
    event_version,
    message_id,
    actor_id,
    fanout_mode,
    fanout_policy_version,
    permission_version,
    classification,
    mapping_version,
    trace_id,
    payload_json,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15::jsonb, $16)
`,
		record.Timeline.TenantID,
		record.Timeline.ConversationID,
		record.Timeline.ConversationSeq,
		record.Timeline.EventID,
		record.Timeline.EventType,
		record.Timeline.EventVersion,
		record.Timeline.MessageID,
		input.Command.AuthContext.UserID,
		record.Timeline.FanoutMode,
		record.Timeline.FanoutPolicyVersion,
		record.Timeline.PermissionVersion,
		record.Timeline.Classification,
		record.Timeline.MappingVersion,
		record.Timeline.TraceID,
		record.Timeline.PayloadJSON,
		record.Timeline.CreatedAt,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (r *MessageRepository) insertOutboxEvent(ctx context.Context, tx pgx.Tx, record domain.AppendMessageRecord) error {
	_, err := tx.Exec(ctx, `
INSERT INTO message_outbox (
    event_id,
    tenant_id,
    conversation_id,
    aggregate_version,
    event_type,
    event_version,
    partition_key,
    mapping_version,
    correlation_id,
    causation_id,
    producer,
    payload_json,
    trace_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13)
`,
		record.Outbox.EventID,
		record.Outbox.TenantID,
		record.Outbox.ConversationID,
		record.Outbox.AggregateVersion,
		record.Outbox.EventType,
		record.Outbox.EventVersion,
		record.Outbox.PartitionKey,
		record.Outbox.MappingVersion,
		record.Outbox.CorrelationID,
		record.Outbox.CausationID,
		record.Outbox.Producer,
		record.Outbox.PayloadJSON,
		record.Outbox.TraceID,
	)
	if err != nil {
		return types.NewOutboxWriteFailed(err.Error())
	}
	return nil
}

func newUUIDString() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16],
	), nil
}
