package main

import (
	"context"
	"net"
	"testing"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	contactsgrpc "github.com/qsyy0921/IM/services/contacts-service/internal/api/grpc"
	monitoringinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/monitoring"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestNewGRPCServerRecordsAuthFailures(t *testing.T) {
	t.Setenv("NEXUSIM_CONTACTS_AUTH_MODE", "metadata")
	metrics := monitoringinfra.NewGRPCMetrics()
	server, err := newGRPCServer(metrics)
	if err != nil {
		t.Fatalf("new grpc server: %v", err)
	}
	contactsgrpc.Register(server, contactsgrpc.NewServer(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	listener := bufconn.Listen(1024 * 1024)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithInsecure(),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	client := contactsv1.NewContactsServiceClient(conn)
	_, err = client.ListContacts(context.Background(), &contactsv1.ListContactsRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.TotalRequests != 1 || snapshot.TotalErrors != 1 || len(snapshot.Methods) != 1 {
		t.Fatalf("expected recorded auth failure, got %+v", snapshot)
	}
	if got := snapshot.Methods[0].Codes[codes.Unauthenticated.String()]; got != 1 {
		t.Fatalf("expected unauthenticated code count, got %+v", snapshot.Methods[0].Codes)
	}
}
