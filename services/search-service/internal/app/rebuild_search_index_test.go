package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/search-service/internal/types"
)

func TestRebuildSearchIndexDryRunScansWithoutWriter(t *testing.T) {
	source := &fakeSearchIndexSource{
		batches: [][]types.SearchIndexDocument{
			{
				{TenantID: "tenant-1", ConversationID: "conv-1", MessageID: "m1"},
				{TenantID: "tenant-1", ConversationID: "conv-1", MessageID: "m2"},
			},
			{},
		},
	}
	useCase := NewRebuildSearchIndexUseCase(source, nil)
	result, err := useCase.Execute(context.Background(), types.RebuildSearchIndexCommand{
		TenantID:  "tenant-1",
		BatchSize: 2,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.DryRun || result.Scanned != 2 || result.Indexed != 0 || result.Batches != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRebuildSearchIndexExecuteIndexesAndRefreshes(t *testing.T) {
	source := &fakeSearchIndexSource{
		batches: [][]types.SearchIndexDocument{
			{
				{TenantID: "tenant-1", ConversationID: "conv-1", MessageID: "m1"},
				{TenantID: "tenant-1", ConversationID: "conv-1", MessageID: "m2"},
			},
			{
				{TenantID: "tenant-1", ConversationID: "conv-2", MessageID: "m3"},
			},
		},
	}
	writer := &fakeSearchIndexWriter{}
	useCase := NewRebuildSearchIndexUseCase(source, writer)
	result, err := useCase.Execute(context.Background(), types.RebuildSearchIndexCommand{
		TenantID:  "tenant-1",
		BatchSize: 2,
		Execute:   true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.DryRun || result.Scanned != 3 || result.Indexed != 3 || result.Batches != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if writer.ensureCalls != 1 || writer.refreshCalls != 1 || len(writer.indexed) != 2 {
		t.Fatalf("unexpected writer calls: ensure=%d refresh=%d indexed=%d", writer.ensureCalls, writer.refreshCalls, len(writer.indexed))
	}
	if source.after[1].ConversationID != "conv-1" || source.after[1].MessageID != "m2" {
		t.Fatalf("second batch cursor not advanced: %+v", source.after)
	}
}

func TestRebuildSearchIndexExecuteRequiresWriter(t *testing.T) {
	useCase := NewRebuildSearchIndexUseCase(&fakeSearchIndexSource{}, nil)
	_, err := useCase.Execute(context.Background(), types.RebuildSearchIndexCommand{
		TenantID: "tenant-1",
		Execute:  true,
	})
	if !errors.Is(err, types.ErrSearchUnavailable) {
		t.Fatalf("err=%v want ErrSearchUnavailable", err)
	}
}

type fakeSearchIndexSource struct {
	batches [][]types.SearchIndexDocument
	after   []types.SearchIndexCursor
}

func (source *fakeSearchIndexSource) ListSearchIndexDocuments(
	_ context.Context,
	_ types.RebuildSearchIndexCommand,
	after types.SearchIndexCursor,
	_ int,
) ([]types.SearchIndexDocument, error) {
	source.after = append(source.after, after)
	if len(source.batches) == 0 {
		return nil, nil
	}
	batch := source.batches[0]
	source.batches = source.batches[1:]
	return batch, nil
}

type fakeSearchIndexWriter struct {
	ensureCalls  int
	refreshCalls int
	indexed      [][]types.SearchIndexDocument
}

func (writer *fakeSearchIndexWriter) EnsureSearchIndex(context.Context) error {
	writer.ensureCalls++
	return nil
}

func (writer *fakeSearchIndexWriter) IndexSearchDocuments(_ context.Context, documents []types.SearchIndexDocument) error {
	copied := append([]types.SearchIndexDocument(nil), documents...)
	writer.indexed = append(writer.indexed, copied)
	return nil
}

func (writer *fakeSearchIndexWriter) RefreshSearchIndex(context.Context) error {
	writer.refreshCalls++
	return nil
}
