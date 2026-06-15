package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type identityOutboxPayload struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id"`
	SessionID string `json:"session_id,omitempty"`
	Status    string `json:"status"`
	RevokedBy string `json:"revoked_by"`
	Reason    string `json:"reason"`
	RevokedAt string `json:"revoked_at"`
}

type outboxEventInput struct {
	TenantID         types.TenantID
	AggregateType    string
	AggregateID      string
	AggregateVersion int64
	EventType        string
	PartitionKey     string
	TraceID          string
	CorrelationID    string
	Payload          identityOutboxPayload
}

func (r *Repository) insertDeviceRevokedOutbox(ctx context.Context, tx pgx.Tx, row deviceRow, command types.RevokeDeviceCommand, revokedAt time.Time) error {
	payload := identityOutboxPayload{
		TenantID:  string(row.TenantID),
		UserID:    string(row.UserID),
		DeviceID:  string(row.DeviceID),
		Status:    string(row.Status),
		RevokedBy: string(command.AdminContext.OperatorUserID),
		Reason:    command.Reason,
		RevokedAt: revokedAt.Format(time.RFC3339Nano),
	}
	return r.insertOutboxEvent(ctx, tx, outboxEventInput{
		TenantID:         row.TenantID,
		AggregateType:    "identity_device",
		AggregateID:      identityDeviceAggregateID(row.UserID, row.DeviceID),
		AggregateVersion: revokedAt.UnixMilli(),
		EventType:        types.IdentityEventDeviceRevoked,
		PartitionKey:     identityPartitionKey(row.TenantID, row.UserID, row.DeviceID),
		TraceID:          command.AdminContext.TraceID,
		CorrelationID:    command.AdminContext.RequestID,
		Payload:          payload,
	})
}

func (r *Repository) insertSessionRevokedOutbox(ctx context.Context, tx pgx.Tx, row sessionRow, command types.RevokeSessionCommand, revokedAt time.Time) error {
	payload := identityOutboxPayload{
		TenantID:  string(row.TenantID),
		UserID:    string(row.UserID),
		DeviceID:  string(row.DeviceID),
		SessionID: string(row.SessionID),
		Status:    string(row.Status),
		RevokedBy: string(command.AdminContext.OperatorUserID),
		Reason:    command.Reason,
		RevokedAt: revokedAt.Format(time.RFC3339Nano),
	}
	return r.insertOutboxEvent(ctx, tx, outboxEventInput{
		TenantID:         row.TenantID,
		AggregateType:    "identity_session",
		AggregateID:      identitySessionAggregateID(row.UserID, row.DeviceID, row.SessionID),
		AggregateVersion: revokedAt.UnixMilli(),
		EventType:        types.IdentityEventSessionRevoked,
		PartitionKey:     identityPartitionKey(row.TenantID, row.UserID, row.DeviceID),
		TraceID:          command.AdminContext.TraceID,
		CorrelationID:    command.AdminContext.RequestID,
		Payload:          payload,
	})
}

func (r *Repository) insertOutboxEvent(ctx context.Context, tx pgx.Tx, input outboxEventInput) error {
	eventID, err := r.eventID()
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	payloadJSON, err := json.Marshal(input.Payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO identity_outbox (
    event_id,
    tenant_id,
    aggregate_type,
    aggregate_id,
    aggregate_version,
    event_type,
    event_version,
    mapping_version,
    partition_key,
    producer,
    correlation_id,
    trace_id,
    payload_json
) VALUES ($1, $2, $3, $4, $5, $6, 'v1', 1, $7, 'identity-service', $8, $9, $10::jsonb)
`, eventID, input.TenantID, input.AggregateType, input.AggregateID, input.AggregateVersion, input.EventType, input.PartitionKey, input.CorrelationID, input.TraceID, string(payloadJSON))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func identityPartitionKey(tenantID types.TenantID, userID types.UserID, deviceID types.DeviceID) string {
	return fmt.Sprintf("%s:%s:%s", tenantID, userID, deviceID)
}

func identityDeviceAggregateID(userID types.UserID, deviceID types.DeviceID) string {
	return fmt.Sprintf("%s:%s", userID, deviceID)
}

func identitySessionAggregateID(userID types.UserID, deviceID types.DeviceID, sessionID types.SessionID) string {
	return fmt.Sprintf("%s:%s:%s", userID, deviceID, sessionID)
}
