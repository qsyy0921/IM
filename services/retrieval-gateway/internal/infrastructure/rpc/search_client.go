package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	searchv1 "github.com/qsyy0921/IM/api/proto/nexusim/search/v1"
	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type SearchClient struct {
	client  searchv1.SearchServiceClient
	timeout time.Duration
}

func NewSearchClient(client searchv1.SearchServiceClient, timeout time.Duration) SearchClient {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return SearchClient{client: client, timeout: timeout}
}

func DialSearchClient(_ context.Context, addr string, timeout time.Duration) (SearchClient, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return SearchClient{}, nil, errors.New("search-service address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return SearchClient{}, nil, err
	}
	return NewSearchClient(searchv1.NewSearchServiceClient(conn), timeout), conn.Close, nil
}

func (client SearchClient) SearchMessages(ctx context.Context, query types.SearchQuery) (types.SearchResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	callCtx = outgoingMetadataContext(callCtx, query.AuthContext)
	response, err := client.client.SearchMessages(callCtx, &searchv1.SearchMessagesRequest{
		AuthContext: &searchv1.AuthContext{
			TenantId:  string(query.AuthContext.TenantID),
			UserId:    string(query.AuthContext.UserID),
			DeviceId:  query.AuthContext.DeviceID,
			SessionId: query.AuthContext.SessionID,
			TraceId:   query.AuthContext.TraceID,
			RequestId: query.AuthContext.RequestID,
		},
		Query:          query.Query,
		ConversationId: string(query.ConversationID),
		AfterSeq:       query.AfterSeq,
		Limit:          int32(query.Limit),
	})
	if err != nil {
		return types.SearchResult{}, mapSearchError(err)
	}
	items := make([]types.SearchMessageEvidence, 0, len(response.GetItems()))
	for _, item := range response.GetItems() {
		items = append(items, types.SearchMessageEvidence{
			ConversationID:    types.ConversationID(item.GetConversationId()),
			MessageID:         item.GetMessageId(),
			ConversationSeq:   item.GetConversationSeq(),
			SourceEventID:     item.GetSourceEventId(),
			SenderID:          types.UserID(item.GetSenderId()),
			MessageType:       item.GetMessageType(),
			Snippet:           item.GetSnippet(),
			OccurredAt:        unixMillisToTime(item.GetOccurredAtUnixMs()),
			VisibilityVersion: item.GetVisibilityVersion(),
		})
	}
	return types.SearchResult{
		Items:             items,
		ProjectionVersion: response.GetProjectionVersion(),
	}, nil
}

func unixMillisToTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value)
}
