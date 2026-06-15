package grpc

import (
	"context"
	"testing"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSendContactRequestMapsSourceMetadata(t *testing.T) {
	var captured types.SendContactRequestCommand
	server := NewServer(
		sendContactRequestExecutorFunc(func(_ context.Context, command types.SendContactRequestCommand) (types.SendContactRequestResult, error) {
			captured = command
			return types.SendContactRequestResult{
				RequestID:      "request-1",
				TenantID:       command.AuthContext.TenantID,
				SenderUserID:   command.AuthContext.UserID,
				ReceiverUserID: command.TargetUserID,
				Status:         types.ContactRequestStatusPending,
				SourceType:     command.SourceType,
				SourceRef:      command.SourceRef,
			}, nil
		}),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)

	response, err := server.SendContactRequest(context.Background(), &contactsv1.SendContactRequestRequest{
		AuthContext: &contactsv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "alice",
		},
		TargetUserId:   "bob",
		IdempotencyKey: "send-1",
		Message:        "hello",
		SourceType:     contactsv1.ContactRequestSourceType_CONTACT_REQUEST_SOURCE_TYPE_GROUP,
		SourceRef:      "conversation:conv-1",
	})
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	if captured.SourceType != types.ContactRequestSourceTypeGroup || captured.SourceRef != "conversation:conv-1" {
		t.Fatalf("unexpected captured source metadata: %+v", captured)
	}
	if response.GetSourceType() != contactsv1.ContactRequestSourceType_CONTACT_REQUEST_SOURCE_TYPE_GROUP ||
		response.GetSourceRef() != "conversation:conv-1" {
		t.Fatalf("unexpected response source metadata: %+v", response)
	}
}

func TestSendContactRequestRejectsUnknownSourceType(t *testing.T) {
	server := NewServer(
		sendContactRequestExecutorFunc(func(_ context.Context, command types.SendContactRequestCommand) (types.SendContactRequestResult, error) {
			if err := command.Validate(); err != nil {
				return types.SendContactRequestResult{}, err
			}
			t.Fatalf("expected unknown source type to fail validation, got %+v", command)
			return types.SendContactRequestResult{}, nil
		}),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)

	_, err := server.SendContactRequest(context.Background(), &contactsv1.SendContactRequestRequest{
		AuthContext: &contactsv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "alice",
		},
		TargetUserId:   "bob",
		IdempotencyKey: "send-1",
		Message:        "hello",
		SourceType:     contactsv1.ContactRequestSourceType(99),
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for unknown source type, got %v", err)
	}
}

func TestListContactRequestsMapsSourceMetadata(t *testing.T) {
	server := NewServer(
		nil, nil, nil, nil, nil,
		listContactRequestsExecutorFunc(func(_ context.Context, _ types.ListContactRequestsCommand) (types.ListContactRequestsResult, error) {
			return types.ListContactRequestsResult{
				TenantID:  "tenant-1",
				UserID:    "bob",
				Direction: types.ContactRequestListDirectionIncoming,
				Status:    types.ContactRequestStatusPending,
				Requests: []types.ContactRequestItem{{
					RequestID:      "request-1",
					SenderUserID:   "alice",
					ReceiverUserID: "bob",
					Status:         types.ContactRequestStatusPending,
					Message:        "hello",
					SourceType:     types.ContactRequestSourceTypeInviteLink,
					SourceRef:      "invite:hash-1",
				}},
			}, nil
		}),
		nil, nil, nil, nil, nil, nil, nil,
	)

	response, err := server.ListContactRequests(context.Background(), &contactsv1.ListContactRequestsRequest{
		AuthContext: &contactsv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "bob",
		},
		Direction: contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_INCOMING,
		Status:    contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_PENDING,
	})
	if err != nil {
		t.Fatalf("list contact requests: %v", err)
	}
	if len(response.GetRequests()) != 1 {
		t.Fatalf("expected one request, got %+v", response)
	}
	item := response.GetRequests()[0]
	if item.GetSourceType() != contactsv1.ContactRequestSourceType_CONTACT_REQUEST_SOURCE_TYPE_INVITE_LINK ||
		item.GetSourceRef() != "invite:hash-1" {
		t.Fatalf("unexpected listed source metadata: %+v", item)
	}
}

func TestContactPrivacyMapsPolicySource(t *testing.T) {
	server := NewServer(
		nil,
		getContactPrivacyExecutorFunc(func(_ context.Context, command types.GetContactPrivacyCommand) (types.GetContactPrivacyResult, error) {
			return types.GetContactPrivacyResult{
				TenantID: command.AuthContext.TenantID,
				UserID:   command.AuthContext.UserID,
				Settings: types.ContactPrivacySettings{
					AllowContactRequests: false,
					Version:              3,
					UpdatedAtUnixMS:      1234,
					PolicySource:         types.ContactPrivacyPolicySourceTenantDefault,
				},
			}, nil
		}),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)

	response, err := server.GetContactPrivacy(context.Background(), &contactsv1.GetContactPrivacyRequest{
		AuthContext: &contactsv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "bob",
		},
	})
	if err != nil {
		t.Fatalf("get contact privacy: %v", err)
	}
	if response.GetSettings().GetPolicySource() != contactsv1.ContactPrivacyPolicySource_CONTACT_PRIVACY_POLICY_SOURCE_TENANT_DEFAULT {
		t.Fatalf("unexpected privacy policy source: %+v", response.GetSettings())
	}
}

type sendContactRequestExecutorFunc func(context.Context, types.SendContactRequestCommand) (types.SendContactRequestResult, error)

func (f sendContactRequestExecutorFunc) Execute(ctx context.Context, command types.SendContactRequestCommand) (types.SendContactRequestResult, error) {
	return f(ctx, command)
}

type getContactPrivacyExecutorFunc func(context.Context, types.GetContactPrivacyCommand) (types.GetContactPrivacyResult, error)

func (f getContactPrivacyExecutorFunc) Execute(ctx context.Context, command types.GetContactPrivacyCommand) (types.GetContactPrivacyResult, error) {
	return f(ctx, command)
}

type listContactRequestsExecutorFunc func(context.Context, types.ListContactRequestsCommand) (types.ListContactRequestsResult, error)

func (f listContactRequestsExecutorFunc) Execute(ctx context.Context, command types.ListContactRequestsCommand) (types.ListContactRequestsResult, error) {
	return f(ctx, command)
}
