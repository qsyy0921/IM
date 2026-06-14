package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestMarkPublishedMemberChangesUseCaseExecutesRepository(t *testing.T) {
	repository := &fakeMemberChangeProgressRepository{
		stats: types.MemberChangePublishProgressStats{Advanced: 3},
	}
	useCase := NewMarkPublishedMemberChangesUseCase(repository, 50)

	result, err := useCase.Execute(context.Background())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Advanced != 3 || repository.limit != 50 || repository.calls != 1 {
		t.Fatalf("unexpected result=%+v repository=%+v", result, repository)
	}
}

func TestMarkPublishedMemberChangesUseCaseDefaultsLimit(t *testing.T) {
	repository := &fakeMemberChangeProgressRepository{}
	useCase := NewMarkPublishedMemberChangesUseCase(repository, 0)

	if _, err := useCase.Execute(context.Background()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if repository.limit != types.DefaultMemberChangeProgressLimit {
		t.Fatalf("expected default limit 100, got %d", repository.limit)
	}
}

func TestMarkPublishedMemberChangesUseCaseCapsLimit(t *testing.T) {
	repository := &fakeMemberChangeProgressRepository{}
	useCase := NewMarkPublishedMemberChangesUseCase(repository, types.MaxMemberChangeProgressLimit+1)

	if _, err := useCase.Execute(context.Background()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if repository.limit != types.MaxMemberChangeProgressLimit {
		t.Fatalf("expected capped limit %d, got %d", types.MaxMemberChangeProgressLimit, repository.limit)
	}
}

func TestMarkPublishedMemberChangesUseCasePropagatesError(t *testing.T) {
	wantErr := types.NewDBWriteFailed("update failed")
	useCase := NewMarkPublishedMemberChangesUseCase(&fakeMemberChangeProgressRepository{err: wantErr}, 10)

	_, err := useCase.Execute(context.Background())
	if !errors.Is(err, types.ErrDBWriteFailed) {
		t.Fatalf("expected db write failed, got %v", err)
	}
}

type fakeMemberChangeProgressRepository struct {
	calls int
	limit int
	stats types.MemberChangePublishProgressStats
	err   error
}

func (f *fakeMemberChangeProgressRepository) MarkPublishedMemberChanges(
	_ context.Context,
	limit int,
) (types.MemberChangePublishProgressStats, error) {
	f.calls++
	f.limit = limit
	if f.err != nil {
		return types.MemberChangePublishProgressStats{}, f.err
	}
	return f.stats, nil
}
