package postgres

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func (r *Repository) MarkPublishedMemberChanges(
	ctx context.Context,
	limit int,
) (types.MemberChangePublishProgressStats, error) {
	if r.pool == nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed("repository is not configured")
	}
	limit = types.NormalizeMemberChangeProgressLimit(limit)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, `
SELECT mcs.change_id
FROM member_change_saga mcs
JOIN message_outbox mo
  ON mo.tenant_id = mcs.tenant_id
 AND mo.event_id = mcs.outbox_event_id
 AND mo.conversation_id = mcs.conversation_id
WHERE mcs.status = $1
  AND mo.status = 'PUBLISHED'
  AND mo.published_at IS NOT NULL
  AND mo.producer = 'conversation-service'
  AND mo.event_type IN (
      'conversation.member.joined.v1',
      'conversation.member.left.v1',
      'conversation.member.removed.v1',
      'conversation.member.role_changed.v1',
      'conversation.member.owner_transferred.v1',
      'conversation.member.boundary_cancelled.v1'
  )
ORDER BY mcs.updated_at, mcs.change_id
LIMIT $2
FOR UPDATE OF mcs SKIP LOCKED
`,
		types.MemberChangeStatusOutboxEnqueued,
		limit,
	)
	if err != nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
	}

	changeIDs := make([]string, 0, limit)
	for rows.Next() {
		var changeID types.ChangeID
		if err := rows.Scan(&changeID); err != nil {
			rows.Close()
			return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
		}
		changeIDs = append(changeIDs, string(changeID))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
	}
	rows.Close()
	if len(changeIDs) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
		}
		return types.MemberChangePublishProgressStats{}, nil
	}

	if _, err := tx.Exec(ctx, `
UPDATE member_change_saga
SET status = $2,
    updated_at = now()
WHERE change_id = ANY($1)
`, changeIDs, types.MemberChangeStatusEventPublished); err != nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
	}
	if _, err := tx.Exec(ctx, `
UPDATE member_change_saga
SET status = $2,
    completed_at = COALESCE(completed_at, now()),
    updated_at = now()
WHERE change_id = ANY($1)
`, changeIDs, types.MemberChangeStatusDone); err != nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
	}
	return types.MemberChangePublishProgressStats{Advanced: len(changeIDs)}, nil
}
