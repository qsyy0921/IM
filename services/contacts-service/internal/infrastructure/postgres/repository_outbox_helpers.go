package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type edgeOutboxInput struct {
	TenantID       types.TenantID
	OwnerUserID    types.UserID
	ContactUserID  types.UserID
	EventType      string
	CorrelationID  string
	CausationID    string
	TraceID        string
	PreviousStatus types.ContactEdgeStatus
	Edge           contactEdgeRow
	Reason         string
}

func (r *Repository) insertEdgeOutbox(ctx context.Context, tx pgx.Tx, input edgeOutboxInput) error {
	eventID, err := r.eventID()
	if err != nil {
		return types.NewOutboxWriteFailed(err.Error())
	}
	partitionKey := partitionKeyFor(input.TenantID, input.OwnerUserID, input.ContactUserID)
	aggregateVersion, err := nextContactOutboxAggregateVersion(ctx, tx, input.TenantID, partitionKey)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"tenant_id":       input.TenantID,
		"owner_user_id":   input.OwnerUserID,
		"contact_user_id": input.ContactUserID,
		"status":          input.Edge.Status,
		"edge_version":    input.Edge.Version,
		"remark":          input.Edge.Remark,
		"group_name":      input.Edge.GroupName,
		"occurred_at":     r.now().Format(time.RFC3339Nano),
	}
	if input.PreviousStatus != "" {
		payload["previous_status"] = input.PreviousStatus
	}
	if input.Reason != "" {
		payload["reason"] = input.Reason
	}
	return insertContactOutbox(ctx, tx, contactOutboxInput{
		EventID:          eventID,
		TenantID:         input.TenantID,
		AggregateType:    "CONTACT_EDGE",
		AggregateID:      contactEdgeID(input.OwnerUserID, input.ContactUserID),
		AggregateVersion: aggregateVersion,
		EventType:        input.EventType,
		PartitionKey:     partitionKey,
		CorrelationID:    input.CorrelationID,
		CausationID:      input.CausationID,
		TraceID:          input.TraceID,
		Payload:          payload,
	})
}

func nextContactOutboxAggregateVersion(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, partitionKey string) (int64, error) {
	var version int64
	err := tx.QueryRow(ctx, `
SELECT COALESCE(MAX(aggregate_version), 0) + 1
FROM contacts_outbox
WHERE tenant_id = $1
  AND partition_key = $2
`, tenantID, partitionKey).Scan(&version)
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}
	return version, nil
}

type contactOutboxInput struct {
	EventID          string
	TenantID         types.TenantID
	AggregateType    string
	AggregateID      string
	AggregateVersion int64
	EventType        string
	PartitionKey     string
	CorrelationID    string
	CausationID      string
	TraceID          string
	Payload          map[string]any
}

func insertContactOutbox(ctx context.Context, tx pgx.Tx, input contactOutboxInput) error {
	payloadBytes, err := json.Marshal(input.Payload)
	if err != nil {
		return types.NewOutboxWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO contacts_outbox (
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
    causation_id,
    trace_id,
    payload_json,
    status,
    available_at,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'contacts-service', $10, $11, $12, $13, 'PENDING', now(), now(), now())
`, input.EventID, input.TenantID, input.AggregateType, input.AggregateID, input.AggregateVersion, input.EventType, contactsOutboxEventVersion, contactsOutboxMappingVersion, input.PartitionKey, input.CorrelationID, input.CausationID, input.TraceID, payloadBytes)
	if err != nil {
		return types.NewOutboxWriteFailed(err.Error())
	}
	return nil
}
