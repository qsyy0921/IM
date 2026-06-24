package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/memory-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

type memoryQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) QueryMemoryEvents(
	ctx context.Context,
	command types.QueryMemoryEventsCommand,
	fetchLimit int,
) ([]types.StructuredMemoryEvent, int64, error) {
	if repository.pool == nil {
		return nil, 0, types.NewDBReadFailed("memory repository is not configured")
	}
	if fetchLimit <= 0 {
		return nil, 0, nil
	}
	statuses := command.Statuses
	if len(statuses) == 0 {
		statuses = []string{types.MemoryStatusActive}
	}

	args := []any{
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.AfterValidFromSeq,
		statuses,
		fetchLimit,
	}
	filters := []string{"e.tenant_id = $1", "COALESCE(e.valid_from_seq, 0) > $3", "e.status = ANY($4::text[])"}
	if command.Scope != "" {
		args = append(args, command.Scope)
		filters = append(filters, fmt.Sprintf("e.scope_type = $%d", len(args)))
	}
	if command.ScopeID != "" {
		args = append(args, command.ScopeID)
		filters = append(filters, fmt.Sprintf("e.scope_id = $%d", len(args)))
	}
	if command.ConversationID != "" {
		args = append(args, command.ConversationID)
		filters = append(filters, fmt.Sprintf("e.conversation_id = $%d", len(args)))
	}
	if command.ActorUserID != "" {
		args = append(args, string(command.ActorUserID))
		filters = append(filters, fmt.Sprintf("e.actor_user_ids ? $%d", len(args)))
	}
	if command.AtConversationSeq > 0 {
		args = append(args, command.AtConversationSeq)
		filters = append(filters,
			fmt.Sprintf("COALESCE(e.valid_from_seq, 0) <= $%d", len(args)),
			fmt.Sprintf("(e.valid_to_seq IS NULL OR e.valid_to_seq = 0 OR e.valid_to_seq >= $%d)", len(args)),
		)
	}
	if command.Topic != "" {
		args = append(args, command.Topic)
		filters = append(filters, fmt.Sprintf("e.topic = $%d", len(args)))
	}
	if query := command.NormalizedQuery(); query != "" {
		args = append(args, "%"+escapeLike(query)+"%")
		filters = append(filters, fmt.Sprintf("e.fact_text ILIKE $%d ESCAPE '\\'", len(args)))
	}

	rows, err := repository.pool.Query(ctx, `
SELECT
	e.memory_event_id,
	e.scope_type,
	e.scope_id,
	e.conversation_id,
	e.topic,
	e.event_type,
	e.status,
	e.review_state,
	e.fact_text,
	e.actor_user_ids::text,
	e.audience_user_ids::text,
	COALESCE(e.valid_from_seq, 0),
	COALESCE(e.valid_to_seq, 0),
	e.valid_from_at,
	e.valid_to_at,
	e.supersedes_event_ids::text,
	e.contradicts_event_ids::text,
	e.confidence::float8,
	e.visibility_version,
	e.extraction_version,
	e.updated_at,
	COALESCE(MAX(e.source_projection_version) OVER (), 0) AS projection_version
FROM memory_structured_events e
WHERE `+strings.Join(filters, "\n  AND ")+`
  AND e.review_state <> 'REJECTED'
  AND EXISTS (
	SELECT 1
	FROM memory_event_source_refs s
	LEFT JOIN memory_membership_projection m
	  ON m.tenant_id = s.tenant_id
	 AND m.conversation_id = s.conversation_id
	 AND m.user_id = $2
	WHERE s.tenant_id = e.tenant_id
	  AND s.memory_event_id = e.memory_event_id
	  AND (
		s.conversation_id = ''
		OR (
			m.status <> 'BANNED'
			AND m.join_seq <= s.conversation_seq
			AND (m.leave_seq IS NULL OR m.leave_seq >= s.conversation_seq)
		)
	  )
  )
ORDER BY COALESCE(e.valid_from_seq, 0) ASC, e.memory_event_id ASC
LIMIT $5
`, args...)
	if err != nil {
		return nil, 0, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	items, projectionVersion, err := scanMemoryEvents(rows)
	if err != nil {
		return nil, 0, err
	}
	if err := repository.attachSourceRefs(ctx, command.AuthContext.TenantID, items); err != nil {
		return nil, 0, err
	}
	return items, projectionVersion, nil
}

func (repository *Repository) GetMemoryEvent(
	ctx context.Context,
	command types.GetMemoryEventCommand,
) (types.StructuredMemoryEvent, []types.MemoryGraphEdge, error) {
	if repository.pool == nil {
		return types.StructuredMemoryEvent{}, nil, types.NewDBReadFailed("memory repository is not configured")
	}
	rows, err := repository.pool.Query(ctx, `
SELECT
	e.memory_event_id,
	e.scope_type,
	e.scope_id,
	e.conversation_id,
	e.topic,
	e.event_type,
	e.status,
	e.review_state,
	e.fact_text,
	e.actor_user_ids::text,
	e.audience_user_ids::text,
	COALESCE(e.valid_from_seq, 0),
	COALESCE(e.valid_to_seq, 0),
	e.valid_from_at,
	e.valid_to_at,
	e.supersedes_event_ids::text,
	e.contradicts_event_ids::text,
	e.confidence::float8,
	e.visibility_version,
	e.extraction_version,
	e.updated_at,
	COALESCE(e.source_projection_version, 0)
FROM memory_structured_events e
WHERE e.tenant_id = $1
  AND e.memory_event_id = $3
  AND e.status <> 'DELETED'
  AND e.review_state <> 'REJECTED'
  AND EXISTS (
	SELECT 1
	FROM memory_event_source_refs s
	LEFT JOIN memory_membership_projection m
	  ON m.tenant_id = s.tenant_id
	 AND m.conversation_id = s.conversation_id
	 AND m.user_id = $2
	WHERE s.tenant_id = e.tenant_id
	  AND s.memory_event_id = e.memory_event_id
	  AND (
		s.conversation_id = ''
		OR (
			m.status <> 'BANNED'
			AND m.join_seq <= s.conversation_seq
			AND (m.leave_seq IS NULL OR m.leave_seq >= s.conversation_seq)
		)
	  )
  )
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.MemoryEventID)
	if err != nil {
		return types.StructuredMemoryEvent{}, nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()
	items, _, err := scanMemoryEvents(rows)
	if err != nil {
		return types.StructuredMemoryEvent{}, nil, err
	}
	if len(items) == 0 {
		return types.StructuredMemoryEvent{}, nil, types.ErrMemoryNotFound
	}
	if err := repository.attachSourceRefs(ctx, command.AuthContext.TenantID, items); err != nil {
		return types.StructuredMemoryEvent{}, nil, err
	}
	edges, err := repository.loadGraphEdges(ctx, command.AuthContext.TenantID, command.MemoryEventID)
	if err != nil {
		return types.StructuredMemoryEvent{}, nil, err
	}
	return items[0], edges, nil
}

func (repository *Repository) ListProfileAggregates(
	ctx context.Context,
	command types.ListProfileAggregatesCommand,
	fetchLimit int,
) ([]types.ProfileAggregate, error) {
	if repository.pool == nil {
		return nil, types.NewDBReadFailed("memory repository is not configured")
	}
	if fetchLimit <= 0 {
		return nil, nil
	}
	statuses := command.Statuses
	if len(statuses) == 0 {
		statuses = []string{types.MemoryStatusActive}
	}
	args := []any{command.AuthContext.TenantID, command.SubjectUserID, statuses, fetchLimit}
	filters := []string{"tenant_id = $1", "subject_user_id = $2", "status = ANY($3::text[])"}
	if command.AggregateType != "" {
		args = append(args, command.AggregateType)
		filters = append(filters, fmt.Sprintf("aggregate_type = $%d", len(args)))
	}
	rows, err := repository.pool.Query(ctx, `
SELECT
	profile_id,
	subject_user_id,
	aggregate_type,
	aggregate_key,
	status,
	review_state,
	summary_text,
	supporting_memory_event_ids::text,
	confidence::float8,
	valid_from_at,
	valid_to_at,
	updated_at
FROM memory_profile_aggregates
WHERE `+strings.Join(filters, "\n  AND ")+`
  AND review_state <> 'REJECTED'
  AND (
	status <> 'ACTIVE'
	OR jsonb_array_length(supporting_memory_event_ids) = 0
	OR NOT EXISTS (
		SELECT 1
		FROM jsonb_array_elements_text(supporting_memory_event_ids) AS support(memory_event_id)
		WHERE NOT EXISTS (
			SELECT 1
			FROM memory_structured_events e
			WHERE e.tenant_id = memory_profile_aggregates.tenant_id
			  AND e.memory_event_id = support.memory_event_id
			  AND e.status = 'ACTIVE'
			  AND e.review_state = 'APPROVED'
		)
	)
  )
ORDER BY updated_at DESC, profile_id ASC
LIMIT $4
`, args...)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()
	items := make([]types.ProfileAggregate, 0, fetchLimit)
	for rows.Next() {
		var item types.ProfileAggregate
		var supporting string
		var validFromAt, validToAt sql.NullTime
		if err := rows.Scan(
			&item.ProfileID,
			&item.SubjectUserID,
			&item.AggregateType,
			&item.AggregateKey,
			&item.Status,
			&item.ReviewState,
			&item.SummaryText,
			&supporting,
			&item.Confidence,
			&validFromAt,
			&validToAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		item.SupportingMemoryEventIDs = decodeStringSlice(supporting)
		item.ValidFromAt = nullTimeValue(validFromAt)
		item.ValidToAt = nullTimeValue(validToAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return items, nil
}

func (repository *Repository) RecomputeProfileAggregate(
	ctx context.Context,
	command types.RecomputeProfileAggregateCommand,
) (types.ProfileAggregate, int, bool, error) {
	if repository.pool == nil {
		return types.ProfileAggregate{}, 0, false, types.NewDBWriteFailed("memory repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.ProfileAggregate{}, 0, false, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	supports, err := loadProfileSupports(ctx, tx, command)
	if err != nil {
		return types.ProfileAggregate{}, 0, false, err
	}
	minSupportCount := command.EffectiveMinSupportCount()
	if len(supports) < minSupportCount {
		item, err := archiveProfileAggregate(ctx, tx, command, len(supports))
		if err != nil {
			return types.ProfileAggregate{}, 0, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return types.ProfileAggregate{}, 0, false, types.NewDBWriteFailed(err.Error())
		}
		return item, len(supports), false, nil
	}

	item, err := upsertProfileAggregate(ctx, tx, command, supports)
	if err != nil {
		return types.ProfileAggregate{}, 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ProfileAggregate{}, 0, false, types.NewDBWriteFailed(err.Error())
	}
	return item, len(supports), true, nil
}

func (repository *Repository) SubmitMemoryCandidate(
	ctx context.Context,
	command types.SubmitMemoryCandidateCommand,
) (types.StructuredMemoryEvent, error) {
	if repository.pool == nil {
		return types.StructuredMemoryEvent{}, types.NewDBWriteFailed("memory repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.StructuredMemoryEvent{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := ensureCandidateSourceRefsVisible(ctx, tx, command.AuthContext, command.SourceRefs); err != nil {
		return types.StructuredMemoryEvent{}, err
	}
	if err := insertMemoryCandidate(ctx, tx, command); err != nil {
		return types.StructuredMemoryEvent{}, err
	}
	item, err := loadMemoryEventByID(ctx, tx, command.AuthContext.TenantID, command.CandidateID)
	if err != nil {
		return types.StructuredMemoryEvent{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.StructuredMemoryEvent{}, types.NewDBWriteFailed(err.Error())
	}
	return item, nil
}

func (repository *Repository) ReviewMemoryCandidate(
	ctx context.Context,
	command types.ReviewMemoryCandidateCommand,
) (types.StructuredMemoryEvent, error) {
	if repository.pool == nil {
		return types.StructuredMemoryEvent{}, types.NewDBWriteFailed("memory repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.StructuredMemoryEvent{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	var statusValue string
	var reviewState string
	if err := tx.QueryRow(ctx, `
SELECT status, review_state
FROM memory_structured_events
WHERE tenant_id = $1
  AND memory_event_id = $2
FOR UPDATE
`, command.AuthContext.TenantID, command.MemoryEventID).Scan(&statusValue, &reviewState); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.StructuredMemoryEvent{}, types.ErrMemoryNotFound
		}
		return types.StructuredMemoryEvent{}, types.NewDBReadFailed(err.Error())
	}
	if statusValue != types.MemoryStatusPending ||
		(reviewState != types.MemoryReviewNeedsReview && reviewState != types.MemoryReviewUnreviewed) {
		return types.StructuredMemoryEvent{}, types.ErrInvalidArgument
	}
	visible, err := isMemoryEventVisibleForUser(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.MemoryEventID)
	if err != nil {
		return types.StructuredMemoryEvent{}, err
	}
	if !visible {
		return types.StructuredMemoryEvent{}, types.ErrPermissionDenied
	}
	nextStatus := types.MemoryStatusRejected
	nextReview := types.MemoryReviewRejected
	if command.Decision == types.MemoryReviewDecisionApprove {
		nextStatus = types.MemoryStatusActive
		nextReview = types.MemoryReviewApproved
	}
	if _, err := tx.Exec(ctx, `
UPDATE memory_structured_events
SET status = $3,
    review_state = $4,
    updated_at = now()
WHERE tenant_id = $1
  AND memory_event_id = $2
`, command.AuthContext.TenantID, command.MemoryEventID, nextStatus, nextReview); err != nil {
		return types.StructuredMemoryEvent{}, types.NewDBWriteFailed(err.Error())
	}
	item, err := loadMemoryEventByID(ctx, tx, command.AuthContext.TenantID, command.MemoryEventID)
	if err != nil {
		return types.StructuredMemoryEvent{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.StructuredMemoryEvent{}, types.NewDBWriteFailed(err.Error())
	}
	return item, nil
}

func ensureCandidateSourceRefsVisible(ctx context.Context, tx pgx.Tx, auth types.AuthContext, refs []types.SourceRef) error {
	for _, ref := range refs {
		var visible bool
		if err := tx.QueryRow(ctx, `
SELECT EXISTS (
	SELECT 1
	FROM memory_membership_projection
	WHERE tenant_id = $1
	  AND conversation_id = $2
	  AND user_id = $3
	  AND status <> 'BANNED'
	  AND join_seq <= $4
	  AND (leave_seq IS NULL OR leave_seq >= $4)
)
`, auth.TenantID, ref.ConversationID, auth.UserID, ref.ConversationSeq).Scan(&visible); err != nil {
			return types.NewDBReadFailed(err.Error())
		}
		if !visible {
			return types.ErrPermissionDenied
		}
	}
	return nil
}

func insertMemoryCandidate(ctx context.Context, tx pgx.Tx, command types.SubmitMemoryCandidateCommand) error {
	actorJSON, _ := json.Marshal(command.ActorUserIDs)
	audienceJSON, _ := json.Marshal(command.AudienceUserIDs)
	supersedesJSON, _ := json.Marshal(command.SupersedesEventIDs)
	contradictsJSON, _ := json.Marshal(command.ContradictsEventIDs)
	_, err := tx.Exec(ctx, `
INSERT INTO memory_structured_events (
	tenant_id,
	memory_event_id,
	scope_type,
	scope_id,
	conversation_id,
	topic,
	event_type,
	status,
	review_state,
	fact_text,
	actor_user_ids,
	audience_user_ids,
	valid_from_seq,
	valid_to_seq,
	supersedes_event_ids,
	contradicts_event_ids,
	confidence,
	visibility_version,
	extraction_version,
	source_projection_version,
	updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, 'PENDING', 'NEEDS_REVIEW', $8, $9::jsonb, $10::jsonb, $11, $12, $13::jsonb, $14::jsonb, $15, $16, $17, $18, now())
`, command.AuthContext.TenantID, command.CandidateID, command.Scope, command.ScopeID, command.ConversationID, command.Topic, command.EventType, command.FactText, string(actorJSON), string(audienceJSON), command.ValidFromSeq, nullableInt64(command.ValidToSeq), string(supersedesJSON), string(contradictsJSON), command.Confidence, command.VisibilityVersion, command.ExtractionVersion, maxSourceProjectionVersion(command.SourceRefs))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	for _, ref := range command.SourceRefs {
		if err := insertCandidateSourceRef(ctx, tx, command.AuthContext.TenantID, command.CandidateID, ref); err != nil {
			return err
		}
	}
	return nil
}

func insertCandidateSourceRef(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, memoryEventID string, ref types.SourceRef) error {
	sourceRefID := strings.Join([]string{
		ref.SourceType,
		ref.SourceID,
		ref.SourceEventID,
		string(ref.ConversationID),
		fmt.Sprintf("%d", ref.ConversationSeq),
	}, ":")
	_, err := tx.Exec(ctx, `
INSERT INTO memory_event_source_refs (
	tenant_id,
	memory_event_id,
	source_ref_id,
	source_type,
	source_id,
	source_event_id,
	conversation_id,
	conversation_seq,
	occurred_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`, tenantID, memoryEventID, sourceRefID, ref.SourceType, ref.SourceID, ref.SourceEventID, ref.ConversationID, ref.ConversationSeq, nullableTime(ref.OccurredAt))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func loadMemoryEventByID(ctx context.Context, querier memoryQuerier, tenantID types.TenantID, memoryEventID string) (types.StructuredMemoryEvent, error) {
	rows, err := querier.Query(ctx, `
SELECT
	e.memory_event_id,
	e.scope_type,
	e.scope_id,
	e.conversation_id,
	e.topic,
	e.event_type,
	e.status,
	e.review_state,
	e.fact_text,
	e.actor_user_ids::text,
	e.audience_user_ids::text,
	COALESCE(e.valid_from_seq, 0),
	COALESCE(e.valid_to_seq, 0),
	e.valid_from_at,
	e.valid_to_at,
	e.supersedes_event_ids::text,
	e.contradicts_event_ids::text,
	e.confidence::float8,
	e.visibility_version,
	e.extraction_version,
	e.updated_at,
	COALESCE(e.source_projection_version, 0)
FROM memory_structured_events e
WHERE e.tenant_id = $1
  AND e.memory_event_id = $2
`, tenantID, memoryEventID)
	if err != nil {
		return types.StructuredMemoryEvent{}, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()
	items, _, err := scanMemoryEvents(rows)
	if err != nil {
		return types.StructuredMemoryEvent{}, err
	}
	if len(items) == 0 {
		return types.StructuredMemoryEvent{}, types.ErrMemoryNotFound
	}
	if err := attachSourceRefsWithQuerier(ctx, querier, tenantID, items); err != nil {
		return types.StructuredMemoryEvent{}, err
	}
	return items[0], nil
}

func isMemoryEventVisibleForUser(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, memoryEventID string) (bool, error) {
	var visible bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
	SELECT 1
	FROM memory_event_source_refs s
	LEFT JOIN memory_membership_projection m
	  ON m.tenant_id = s.tenant_id
	 AND m.conversation_id = s.conversation_id
	 AND m.user_id = $3
	WHERE s.tenant_id = $1
	  AND s.memory_event_id = $2
	  AND (
		s.conversation_id = ''
		OR (
			m.status <> 'BANNED'
			AND m.join_seq <= s.conversation_seq
			AND (m.leave_seq IS NULL OR m.leave_seq >= s.conversation_seq)
		)
	  )
)
`, tenantID, memoryEventID, userID).Scan(&visible); err != nil {
		return false, types.NewDBReadFailed(err.Error())
	}
	return visible, nil
}

func nullableInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func maxSourceProjectionVersion(refs []types.SourceRef) int64 {
	var maxValue int64
	for _, ref := range refs {
		if ref.ConversationSeq > maxValue {
			maxValue = ref.ConversationSeq
		}
	}
	return maxValue
}

type profileSupport struct {
	MemoryEventID string
	FactText      string
	Confidence    float64
	ValidFromAt   time.Time
	UpdatedAt     time.Time
}

func loadProfileSupports(ctx context.Context, tx pgx.Tx, command types.RecomputeProfileAggregateCommand) ([]profileSupport, error) {
	rows, err := tx.Query(ctx, `
SELECT
	e.memory_event_id,
	e.fact_text,
	e.confidence::float8,
	e.valid_from_at,
	e.updated_at
FROM memory_structured_events e
WHERE e.tenant_id = $1
  AND e.event_type = 'PROFILE_SIGNAL'
  AND e.status = 'ACTIVE'
  AND e.review_state = 'APPROVED'
  AND e.actor_user_ids ? $2
  AND e.topic = $3
  AND EXISTS (
	SELECT 1
	FROM memory_event_source_refs s
	LEFT JOIN memory_membership_projection m
	  ON m.tenant_id = s.tenant_id
	 AND m.conversation_id = s.conversation_id
	 AND m.user_id = $4
	WHERE s.tenant_id = e.tenant_id
	  AND s.memory_event_id = e.memory_event_id
	  AND (
		s.conversation_id = ''
		OR (
			m.status <> 'BANNED'
			AND m.join_seq <= s.conversation_seq
			AND (m.leave_seq IS NULL OR m.leave_seq >= s.conversation_seq)
		)
	  )
  )
ORDER BY COALESCE(e.valid_from_seq, 0), e.memory_event_id
FOR UPDATE OF e
`, command.AuthContext.TenantID, string(command.SubjectUserID), command.AggregateKey, string(command.AuthContext.UserID))
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	var supports []profileSupport
	for rows.Next() {
		var support profileSupport
		var validFromAt sql.NullTime
		if err := rows.Scan(&support.MemoryEventID, &support.FactText, &support.Confidence, &validFromAt, &support.UpdatedAt); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		support.ValidFromAt = nullTimeValue(validFromAt)
		supports = append(supports, support)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return supports, nil
}

func archiveProfileAggregate(ctx context.Context, tx pgx.Tx, command types.RecomputeProfileAggregateCommand, supportCount int) (types.ProfileAggregate, error) {
	row := tx.QueryRow(ctx, `
UPDATE memory_profile_aggregates
SET status = 'ARCHIVED',
    review_state = 'NEEDS_REVIEW',
    summary_text = $5,
    confidence = 0,
    updated_at = now()
WHERE tenant_id = $1
  AND subject_user_id = $2
  AND aggregate_type = $3
  AND aggregate_key = $4
  AND status IN ('PENDING', 'ACTIVE')
RETURNING
	profile_id,
	subject_user_id,
	aggregate_type,
	aggregate_key,
	status,
	review_state,
	summary_text,
	supporting_memory_event_ids::text,
	confidence::float8,
	valid_from_at,
	valid_to_at,
	updated_at
`, command.AuthContext.TenantID, string(command.SubjectUserID), command.AggregateType, command.AggregateKey, fmt.Sprintf("profile aggregate archived: only %d approved supporting memory events", supportCount))
	item, err := scanProfileAggregateRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.ProfileAggregate{}, nil
	}
	if err != nil {
		return types.ProfileAggregate{}, err
	}
	return item, nil
}

func upsertProfileAggregate(ctx context.Context, tx pgx.Tx, command types.RecomputeProfileAggregateCommand, supports []profileSupport) (types.ProfileAggregate, error) {
	supportIDs := make([]string, 0, len(supports))
	facts := make([]string, 0, len(supports))
	var confidenceTotal float64
	var validFromAt time.Time
	var updatedBy string
	for _, support := range supports {
		supportIDs = append(supportIDs, support.MemoryEventID)
		if fact := strings.TrimSpace(support.FactText); fact != "" {
			facts = append(facts, fact)
		}
		confidenceTotal += support.Confidence
		if !support.ValidFromAt.IsZero() && (validFromAt.IsZero() || support.ValidFromAt.Before(validFromAt)) {
			validFromAt = support.ValidFromAt
		}
		updatedBy = support.MemoryEventID
	}
	supportJSON, _ := json.Marshal(supportIDs)
	confidence := confidenceTotal / float64(len(supports))
	summary := profileAggregateSummary(command, facts, len(supports))
	row := tx.QueryRow(ctx, `
INSERT INTO memory_profile_aggregates (
	tenant_id,
	profile_id,
	subject_user_id,
	aggregate_type,
	aggregate_key,
	status,
	review_state,
	summary_text,
	supporting_memory_event_ids,
	confidence,
	valid_from_at,
	updated_by_memory_event_id,
	created_at,
	updated_at
) VALUES (
	$1, $2, $3, $4, $5,
	'ACTIVE', 'APPROVED', $6, $7::jsonb, $8, $9, $10, now(), now()
)
ON CONFLICT (tenant_id, profile_id) DO UPDATE SET
	subject_user_id = EXCLUDED.subject_user_id,
	aggregate_type = EXCLUDED.aggregate_type,
	aggregate_key = EXCLUDED.aggregate_key,
	status = EXCLUDED.status,
	review_state = EXCLUDED.review_state,
	summary_text = EXCLUDED.summary_text,
	supporting_memory_event_ids = EXCLUDED.supporting_memory_event_ids,
	confidence = EXCLUDED.confidence,
	valid_from_at = EXCLUDED.valid_from_at,
	valid_to_at = NULL,
	updated_by_memory_event_id = EXCLUDED.updated_by_memory_event_id,
	updated_at = now()
RETURNING
	profile_id,
	subject_user_id,
	aggregate_type,
	aggregate_key,
	status,
	review_state,
	summary_text,
	supporting_memory_event_ids::text,
	confidence::float8,
	valid_from_at,
	valid_to_at,
	updated_at
`, command.AuthContext.TenantID, profileAggregateID(command), string(command.SubjectUserID), command.AggregateType, command.AggregateKey, summary, string(supportJSON), confidence, nullableTime(validFromAt), updatedBy)
	return scanProfileAggregateRow(row)
}

type profileAggregateScanner interface {
	Scan(dest ...any) error
}

func scanProfileAggregateRow(row profileAggregateScanner) (types.ProfileAggregate, error) {
	var item types.ProfileAggregate
	var supporting string
	var validFromAt, validToAt sql.NullTime
	if err := row.Scan(
		&item.ProfileID,
		&item.SubjectUserID,
		&item.AggregateType,
		&item.AggregateKey,
		&item.Status,
		&item.ReviewState,
		&item.SummaryText,
		&supporting,
		&item.Confidence,
		&validFromAt,
		&validToAt,
		&item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.ProfileAggregate{}, err
		}
		return types.ProfileAggregate{}, types.NewDBReadFailed(err.Error())
	}
	item.SupportingMemoryEventIDs = decodeStringSlice(supporting)
	item.ValidFromAt = nullTimeValue(validFromAt)
	item.ValidToAt = nullTimeValue(validToAt)
	return item, nil
}

func profileAggregateID(command types.RecomputeProfileAggregateCommand) string {
	hash := sha256.Sum256([]byte(strings.Join([]string{
		string(command.AuthContext.TenantID),
		string(command.SubjectUserID),
		command.AggregateType,
		command.AggregateKey,
	}, "\x00")))
	return "profile-" + hex.EncodeToString(hash[:8])
}

func profileAggregateSummary(command types.RecomputeProfileAggregateCommand, facts []string, supportCount int) string {
	prefix := fmt.Sprintf("%s/%s profile supported by %d approved memory events", command.AggregateType, command.AggregateKey, supportCount)
	if len(facts) == 0 {
		return prefix
	}
	summary := prefix + ": " + strings.Join(facts, " | ")
	if len(summary) > 512 {
		return summary[:512]
	}
	return summary
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func (repository *Repository) ProjectTimelineEvent(ctx context.Context, command types.ProjectTimelineEventCommand) (types.ProjectTimelineEventResult, error) {
	if repository.pool == nil {
		return types.ProjectTimelineEventResult{}, types.NewDBWriteFailed("memory repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.ProjectTimelineEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	result := types.ProjectTimelineEventResult{
		TenantID:        command.TenantID,
		EventID:         command.EventID,
		ConversationID:  command.ConversationID,
		ConversationSeq: command.ConversationSeq,
	}
	switch command.EventType {
	case types.TimelineEventMessagePersisted, types.TimelineEventMessageEdited:
		if command.ProjectMemory {
			if err := upsertMemoryCandidate(ctx, tx, command); err != nil {
				return types.ProjectTimelineEventResult{}, err
			}
			result.ProjectedMemory = true
		}
	case types.TimelineEventMessageRevoked, types.TimelineEventMessageDeleted:
		if err := tombstoneMemoryByMessage(ctx, tx, command); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedMemory = true
	case types.TimelineEventConversationMemberJoined:
		if err := upsertMembership(ctx, tx, command, command.TargetUserID, command.MemberRole, types.MemoryMemberStatusActive, true); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedMember = true
	case types.TimelineEventConversationMemberLeft:
		if err := upsertMembership(ctx, tx, command, command.TargetUserID, command.MemberRole, types.MemoryMemberStatusLeft, false); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedMember = true
	case types.TimelineEventConversationMemberRemoved:
		if err := upsertMembership(ctx, tx, command, command.TargetUserID, command.MemberRole, types.MemoryMemberStatusRemoved, false); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedMember = true
	case types.TimelineEventConversationMemberRoleChanged:
		if err := upsertMembership(ctx, tx, command, command.TargetUserID, command.MemberRole, types.MemoryMemberStatusActive, false); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedMember = true
	case types.TimelineEventConversationMemberOwnerTransferred:
		if err := upsertMembership(ctx, tx, command, command.PreviousOwnerUserID, command.PreviousOwnerRole, types.MemoryMemberStatusActive, false); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		if err := upsertMembership(ctx, tx, command, command.NewOwnerUserID, command.NewOwnerRole, types.MemoryMemberStatusActive, false); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedMember = true
	case types.TimelineEventConversationMemberBoundaryCancelled:
		result.ProjectedMember = false
	default:
		return types.ProjectTimelineEventResult{}, types.NewUnsupportedPayload("unsupported timeline event type")
	}
	if err := upsertCheckpoint(ctx, tx, command); err != nil {
		return types.ProjectTimelineEventResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ProjectTimelineEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func upsertMemoryCandidate(ctx context.Context, tx pgx.Tx, command types.ProjectTimelineEventCommand) error {
	memoryEventID := strings.TrimSpace(command.MemoryEventID)
	if memoryEventID == "" {
		memoryEventID = command.EventID
	}
	actors := []string{}
	if command.SenderID != "" {
		actors = append(actors, string(command.SenderID))
	}
	actorJSON, _ := json.Marshal(actors)
	emptyJSON := []byte("[]")
	extractionVersion := strings.TrimSpace(command.ExtractionVersion)
	if extractionVersion == "" {
		extractionVersion = "rules-v0.1"
	}
	_, err := tx.Exec(ctx, `
INSERT INTO memory_structured_events (
	tenant_id,
	memory_event_id,
	scope_type,
	scope_id,
	conversation_id,
	topic,
	event_type,
	status,
	review_state,
	fact_text,
	actor_user_ids,
	audience_user_ids,
	valid_from_seq,
	supersedes_event_ids,
	contradicts_event_ids,
	confidence,
	visibility_version,
	extraction_version,
	source_projection_version,
	updated_at
) VALUES ($1, $2, 'CONVERSATION', $3, $3, $4, $5, 'PENDING', $6, $7, $8::jsonb, $9::jsonb, $10, $9::jsonb, $9::jsonb, $11, $12, $13, $10, now())
ON CONFLICT (tenant_id, memory_event_id) DO UPDATE SET
	scope_type = EXCLUDED.scope_type,
	scope_id = EXCLUDED.scope_id,
	conversation_id = EXCLUDED.conversation_id,
	topic = EXCLUDED.topic,
	event_type = EXCLUDED.event_type,
	review_state = EXCLUDED.review_state,
	fact_text = EXCLUDED.fact_text,
	actor_user_ids = EXCLUDED.actor_user_ids,
	valid_from_seq = EXCLUDED.valid_from_seq,
	confidence = EXCLUDED.confidence,
	visibility_version = EXCLUDED.visibility_version,
	extraction_version = EXCLUDED.extraction_version,
	source_projection_version = EXCLUDED.source_projection_version,
	updated_at = now()
`, command.TenantID, memoryEventID, command.ConversationID, command.TopicText, command.MemoryEventType, command.MemoryReviewState, command.FactText, string(actorJSON), string(emptyJSON), command.ConversationSeq, command.MemoryConfidence, command.PermissionVersion, extractionVersion)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return upsertSourceRef(ctx, tx, command, memoryEventID)
}

func upsertSourceRef(ctx context.Context, tx pgx.Tx, command types.ProjectTimelineEventCommand, memoryEventID string) error {
	sourceRefID := "message:" + command.MessageID + ":" + command.EventID
	_, err := tx.Exec(ctx, `
INSERT INTO memory_event_source_refs (
	tenant_id,
	memory_event_id,
	source_ref_id,
	source_type,
	source_id,
	source_event_id,
	conversation_id,
	conversation_seq,
	occurred_at
) VALUES ($1, $2, $3, 'MESSAGE', $4, $5, $6, $7, now())
ON CONFLICT (tenant_id, memory_event_id, source_ref_id) DO UPDATE SET
	source_id = EXCLUDED.source_id,
	source_event_id = EXCLUDED.source_event_id,
	conversation_id = EXCLUDED.conversation_id,
	conversation_seq = EXCLUDED.conversation_seq,
	occurred_at = EXCLUDED.occurred_at
`, command.TenantID, memoryEventID, sourceRefID, command.MessageID, command.EventID, command.ConversationID, command.ConversationSeq)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func tombstoneMemoryByMessage(ctx context.Context, tx pgx.Tx, command types.ProjectTimelineEventCommand) error {
	_, err := tx.Exec(ctx, `
UPDATE memory_structured_events e
SET status = 'DELETED',
    review_state = 'REJECTED',
    source_projection_version = GREATEST(e.source_projection_version, $5),
    updated_at = now()
FROM memory_event_source_refs s
WHERE s.tenant_id = e.tenant_id
  AND s.memory_event_id = e.memory_event_id
  AND s.tenant_id = $1
  AND s.conversation_id = $2
  AND s.source_type = 'MESSAGE'
  AND s.source_id = $3
  AND s.conversation_seq <= $4
`, command.TenantID, command.ConversationID, command.MessageID, command.ConversationSeq, command.ConversationSeq)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertMembership(ctx context.Context, tx pgx.Tx, command types.ProjectTimelineEventCommand, userID types.UserID, role string, status string, resetJoin bool) error {
	if userID == "" {
		return types.NewInvalidArgument("target_user_id is required")
	}
	joinSeq := command.ConversationSeq
	leaveSeq := any(nil)
	if status != types.MemoryMemberStatusActive {
		leaveSeq = command.ConversationSeq
	}
	if resetJoin {
		leaveSeq = nil
	}
	_, err := tx.Exec(ctx, `
INSERT INTO memory_membership_projection (
	tenant_id,
	conversation_id,
	user_id,
	role,
	status,
	join_seq,
	leave_seq,
	member_version,
	permission_version,
	updated_by_event_id,
	updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
ON CONFLICT (tenant_id, conversation_id, user_id) DO UPDATE SET
	role = CASE WHEN EXCLUDED.role <> '' THEN EXCLUDED.role ELSE memory_membership_projection.role END,
	status = EXCLUDED.status,
	join_seq = CASE WHEN $11 THEN EXCLUDED.join_seq ELSE memory_membership_projection.join_seq END,
	leave_seq = EXCLUDED.leave_seq,
	member_version = EXCLUDED.member_version,
	permission_version = EXCLUDED.permission_version,
	updated_by_event_id = EXCLUDED.updated_by_event_id,
	updated_at = now()
`, command.TenantID, command.ConversationID, userID, role, status, joinSeq, leaveSeq, command.MemberVersion, command.PermissionVersion, command.EventID, resetJoin)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertCheckpoint(ctx context.Context, tx pgx.Tx, command types.ProjectTimelineEventCommand) error {
	_, err := tx.Exec(ctx, `
INSERT INTO memory_projection_checkpoints (
	consumer_group,
	topic,
	partition_id,
	offset_value,
	updated_at
) VALUES ($1, $2, $3, $4, now())
ON CONFLICT (consumer_group, topic, partition_id) DO UPDATE SET
	offset_value = GREATEST(memory_projection_checkpoints.offset_value, EXCLUDED.offset_value),
	updated_at = now()
`, command.ConsumerGroup, command.Topic, command.PartitionID, command.OffsetValue)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func scanMemoryEvents(rows pgx.Rows) ([]types.StructuredMemoryEvent, int64, error) {
	items := []types.StructuredMemoryEvent{}
	var projectionVersion int64
	for rows.Next() {
		var item types.StructuredMemoryEvent
		var actorJSON, audienceJSON, supersedesJSON, contradictsJSON string
		var validFromAt, validToAt sql.NullTime
		if err := rows.Scan(
			&item.MemoryEventID,
			&item.Scope,
			&item.ScopeID,
			&item.ConversationID,
			&item.Topic,
			&item.EventType,
			&item.Status,
			&item.ReviewState,
			&item.FactText,
			&actorJSON,
			&audienceJSON,
			&item.ValidFromSeq,
			&item.ValidToSeq,
			&validFromAt,
			&validToAt,
			&supersedesJSON,
			&contradictsJSON,
			&item.Confidence,
			&item.VisibilityVersion,
			&item.ExtractionVersion,
			&item.UpdatedAt,
			&projectionVersion,
		); err != nil {
			return nil, 0, types.NewDBReadFailed(err.Error())
		}
		item.ActorUserIDs = decodeStringSlice(actorJSON)
		item.AudienceUserIDs = decodeStringSlice(audienceJSON)
		item.SupersedesEventIDs = decodeStringSlice(supersedesJSON)
		item.ContradictsEventIDs = decodeStringSlice(contradictsJSON)
		item.ValidFromAt = nullTimeValue(validFromAt)
		item.ValidToAt = nullTimeValue(validToAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, types.NewDBReadFailed(err.Error())
	}
	return items, projectionVersion, nil
}

func (repository *Repository) attachSourceRefs(ctx context.Context, tenantID types.TenantID, items []types.StructuredMemoryEvent) error {
	return attachSourceRefsWithQuerier(ctx, repository.pool, tenantID, items)
}

func attachSourceRefsWithQuerier(ctx context.Context, querier memoryQuerier, tenantID types.TenantID, items []types.StructuredMemoryEvent) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	index := make(map[string]int, len(items))
	for i := range items {
		ids = append(ids, items[i].MemoryEventID)
		index[items[i].MemoryEventID] = i
	}
	rows, err := querier.Query(ctx, `
SELECT memory_event_id, source_type, source_id, source_event_id, conversation_id, COALESCE(conversation_seq, 0), occurred_at
FROM memory_event_source_refs
WHERE tenant_id = $1
  AND memory_event_id = ANY($2::text[])
ORDER BY memory_event_id, source_ref_id
`, tenantID, ids)
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()
	for rows.Next() {
		var memoryEventID string
		var ref types.SourceRef
		var occurredAt sql.NullTime
		if err := rows.Scan(&memoryEventID, &ref.SourceType, &ref.SourceID, &ref.SourceEventID, &ref.ConversationID, &ref.ConversationSeq, &occurredAt); err != nil {
			return types.NewDBReadFailed(err.Error())
		}
		ref.OccurredAt = nullTimeValue(occurredAt)
		if idx, ok := index[memoryEventID]; ok {
			items[idx].SourceRefs = append(items[idx].SourceRefs, ref)
		}
	}
	if err := rows.Err(); err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	return nil
}

func nullTimeValue(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func (repository *Repository) loadGraphEdges(ctx context.Context, tenantID types.TenantID, memoryEventID string) ([]types.MemoryGraphEdge, error) {
	rows, err := repository.pool.Query(ctx, `
SELECT edge_id, from_memory_event_id, to_memory_event_id, relation_type, confidence::float8, source_refs::text
FROM memory_graph_edges
WHERE tenant_id = $1
  AND (from_memory_event_id = $2 OR to_memory_event_id = $2)
ORDER BY created_at ASC, edge_id ASC
`, tenantID, memoryEventID)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()
	var edges []types.MemoryGraphEdge
	for rows.Next() {
		var edge types.MemoryGraphEdge
		var sourceRefsJSON string
		if err := rows.Scan(&edge.EdgeID, &edge.FromMemoryEventID, &edge.ToMemoryEventID, &edge.RelationType, &edge.Confidence, &sourceRefsJSON); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		edge.SourceRefs = decodeSourceRefs(sourceRefsJSON)
		edges = append(edges, edge)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return edges, nil
}

func decodeStringSlice(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

func decodeSourceRefs(raw string) []types.SourceRef {
	var refs []types.SourceRef
	_ = json.Unmarshal([]byte(raw), &refs)
	return refs
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

func IsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func UTCNow() time.Time {
	return time.Now().UTC()
}
