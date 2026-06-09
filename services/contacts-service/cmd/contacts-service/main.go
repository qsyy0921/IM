package main

import (
	"context"
	"log"
	"net"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	contactsgrpc "github.com/qsyy0921/IM/services/contacts-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/contacts-service/internal/app"
	"github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/postgres"
	"google.golang.org/grpc"
)

func main() {
	mode := envOrDefault("NEXUSIM_CONTACTS_SERVICE_MODE", "noop")
	switch mode {
	case "noop":
		log.Printf("contacts-service noop mode")
	case "grpc":
		runGRPC()
	default:
		log.Fatalf("unsupported NEXUSIM_CONTACTS_SERVICE_MODE %q", mode)
	}
}

func runGRPC() {
	addr := envOrDefault("NEXUSIM_CONTACTS_GRPC_ADDR", "0.0.0.0:10500")
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		log.Fatal("NEXUSIM_PG_DSN is required in grpc mode")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("open postgres pool: %v", err)
	}
	defer pool.Close()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen contacts grpc: %v", err)
	}
	repository := postgres.NewRepository(pool)
	server := grpc.NewServer()
	contactsgrpc.Register(server, contactsgrpc.NewServer(
		app.NewSendContactRequestUseCase(repository),
		app.NewRespondContactRequestUseCase(repository),
		app.NewListContactsUseCase(repository),
		app.NewGetContactStateUseCase(repository),
	))
	log.Printf("contacts-service grpc listening on %s", addr)
	if err := server.Serve(listener); err != nil {
		log.Fatalf("serve contacts grpc: %v", err)
	}
}

func envOrDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
