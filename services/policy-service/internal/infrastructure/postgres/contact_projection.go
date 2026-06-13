package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type ProjectionRepository struct {
	pool *pgxpool.Pool
}

func NewProjectionRepository(pool *pgxpool.Pool) *ProjectionRepository {
	return &ProjectionRepository{pool: pool}
}

func (repository *ProjectionRepository) ProjectContactEvent(
	ctx context.Context,
	command types.ProjectContactEventCommand,
) (types.ProjectContactEventResult, error) {
	if repository == nil || repository.pool == nil {
		return types.ProjectContactEventResult{}, types.NewDBWriteFailed("policy projection repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.ProjectContactEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer tx.Rollback(ctx)

	result := types.ProjectContactEventResult{}
	switch command.EventType {
	case types.ContactEventRequestCreated, types.ContactEventRequestDeclined, types.ContactEventRequestCanceled:
	case types.ContactEventRequestAccepted:
		projected, err := upsertContactEdge(ctx, tx, command.TenantID, command.SenderUserID, command.ReceiverUserID, types.ContactEdgeStatusActive, command.EdgeVersion, command.EventID)
		if err != nil {
			return types.ProjectContactEventResult{}, err
		}
		if projected {
			result.ProjectedEdges++
		}
		projected, err = upsertContactEdge(ctx, tx, command.TenantID, command.ReceiverUserID, command.SenderUserID, types.ContactEdgeStatusActive, command.EdgeVersion, command.EventID)
		if err != nil {
			return types.ProjectContactEventResult{}, err
		}
		if projected {
			result.ProjectedEdges++
		}
	case types.ContactEventEdgeDeleted, types.ContactEventEdgeBlocked, types.ContactEventEdgeUnblocked, types.ContactEventRemarkUpdated:
		status := projectionStatus(command)
		projected, err := upsertContactEdge(ctx, tx, command.TenantID, command.OwnerUserID, command.ContactUserID, status, command.EdgeVersion, command.EventID)
		if err != nil {
			return types.ProjectContactEventResult{}, err
		}
		if projected {
			result.ProjectedEdges = 1
		}
	default:
		return types.ProjectContactEventResult{}, types.NewInvalidArgument("unsupported contact event type")
	}
	if command.ShouldCheckpoint() {
		if err := upsertPolicyKafkaCheckpoint(ctx, tx, command); err != nil {
			return types.ProjectContactEventResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ProjectContactEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func projectionStatus(command types.ProjectContactEventCommand) string {
	switch command.EventType {
	case types.ContactEventEdgeDeleted:
		return types.ContactEdgeStatusDeleted
	case types.ContactEventEdgeBlocked:
		return types.ContactEdgeStatusBlocked
	case types.ContactEventEdgeUnblocked:
		return types.ContactEdgeStatusActive
	default:
		return command.Status
	}
}

func upsertContactEdge(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	ownerUserID types.UserID,
	contactUserID types.UserID,
	status string,
	edgeVersion int64,
	eventID string,
) (bool, error) {
	tag, err := tx.Exec(ctx, `
INSERT INTO policy_contact_edges_projection (
    tenant_id,
    owner_user_id,
    contact_user_id,
    status,
    edge_version,
    updated_by_event_id,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (tenant_id, owner_user_id, contact_user_id) DO UPDATE
SET status = EXCLUDED.status,
    edge_version = EXCLUDED.edge_version,
    updated_by_event_id = EXCLUDED.updated_by_event_id,
    updated_at = now()
WHERE policy_contact_edges_projection.edge_version < EXCLUDED.edge_version
`, tenantID, ownerUserID, contactUserID, status, edgeVersion, eventID)
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	return tag.RowsAffected() > 0, nil
}

func upsertPolicyKafkaCheckpoint(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectContactEventCommand,
) error {
	return upsertPolicyKafkaCheckpointValues(ctx, tx, command.ConsumerGroup, command.Topic, command.PartitionID, command.OffsetValue)
}

func upsertPolicyKafkaCheckpointValues(
	ctx context.Context,
	tx pgx.Tx,
	consumerGroup string,
	topic string,
	partitionID int32,
	offsetValue int64,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO policy_kafka_checkpoints (
    consumer_group,
    topic,
    partition_id,
    offset_value,
    updated_at
) VALUES ($1, $2, $3, $4, now())
ON CONFLICT (consumer_group, topic, partition_id) DO UPDATE
SET offset_value = GREATEST(policy_kafka_checkpoints.offset_value, EXCLUDED.offset_value),
    updated_at = now()
`, consumerGroup, topic, partitionID, offsetValue)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}
