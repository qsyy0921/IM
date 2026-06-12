package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	policygrpc "github.com/qsyy0921/IM/services/policy-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/policy-service/internal/app"
	"github.com/qsyy0921/IM/services/policy-service/internal/domain"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("policy-service runtime wiring is idle; set NEXUSIM_POLICY_SERVICE_MODE=grpc")
		return nil
	case "grpc":
		return runGRPC()
	default:
		return errors.New("unsupported NEXUSIM_POLICY_SERVICE_MODE")
	}
}

func runGRPC() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := envString("NEXUSIM_POLICY_GRPC_ADDR", "0.0.0.0:10800")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	policy := domain.StaticMessagePolicy{
		Allowed:           envBool("NEXUSIM_POLICY_MESSAGE_ALLOWED", true),
		PermissionVersion: envInt64("NEXUSIM_POLICY_PERMISSION_VERSION", 1),
		Classification:    envString("NEXUSIM_POLICY_CLASSIFICATION", "INTERNAL"),
		Reason:            envString("NEXUSIM_POLICY_DENY_REASON", ""),
	}
	server := grpc.NewServer()
	policygrpc.Register(server, policygrpc.NewServer(app.NewCheckMessageActionUseCase(policy)))
	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()
	log.Printf("policy-service grpc listening on %s", addr)
	if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	return ctx.Err()
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func envInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
