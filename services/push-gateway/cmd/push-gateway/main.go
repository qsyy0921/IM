package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	wsapi "github.com/qsyy0921/IM/services/push-gateway/internal/api/websocket"
	"github.com/qsyy0921/IM/services/push-gateway/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/kafka"
	"github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/memory"
	redisroute "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/redisroute"
	rpcinfra "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/rpc"
	"github.com/qsyy0921/IM/services/push-gateway/internal/trigger/delivery"
	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_PUSH_GATEWAY_MODE"))
	switch mode {
	case "", "noop":
		log.Println("push-gateway runtime wiring is idle; set NEXUSIM_PUSH_GATEWAY_MODE=ws|delivery-consumer|all")
		return nil
	case "ws":
		return runRuntime(true, false)
	case "delivery-consumer":
		return runRuntime(false, true)
	case "all":
		return runRuntime(true, true)
	default:
		return errors.New("unsupported NEXUSIM_PUSH_GATEWAY_MODE")
	}
}

func runRuntime(enableWS bool, enableConsumer bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	localRegistry := memory.NewRegistryWithConfig(memory.Config{
		ResumeBufferTTL: envDuration("NEXUSIM_PUSH_RESUME_BUFFER_TTL", 10*time.Minute),
	})
	registry := app.SessionRegistry(localRegistry)
	errs := make(chan error, 4)
	var closers []func() error

	var redisClient redis.UniversalClient
	var redisRegistry *redisroute.Registry
	var redisSubscriber *redisroute.Subscriber
	routeBackend := envString("NEXUSIM_PUSH_ROUTE_BACKEND", "memory")
	if routeBackend == "redis" {
		gatewayID := envString("NEXUSIM_PUSH_GATEWAY_ID", defaultGatewayID())
		redisClient = redis.NewClient(&redis.Options{
			Addr:     envString("NEXUSIM_PUSH_REDIS_ADDR", "127.0.0.1:6379"),
			Password: os.Getenv("NEXUSIM_PUSH_REDIS_PASSWORD"),
			DB:       envIntAllowZero("NEXUSIM_PUSH_REDIS_DB", 0),
		})
		if err := redisClient.Ping(ctx).Err(); err != nil {
			return err
		}
		routeConfig := redisroute.Config{
			GatewayID: gatewayID,
			KeyPrefix: envString("NEXUSIM_PUSH_REDIS_KEY_PREFIX", "nexusim:push"),
			RouteTTL:  envDuration("NEXUSIM_PUSH_ROUTE_TTL", 90*time.Second),
			ResumeTTL: envDuration("NEXUSIM_PUSH_RESUME_BUFFER_TTL", 10*time.Minute),
		}
		redisRegistry = redisroute.NewRegistry(localRegistry, redisClient, routeConfig)
		redisRegistry.StartCleanupLoop(ctx, envDurationAllowZero("NEXUSIM_PUSH_ROUTE_CLEANUP_INTERVAL", 30*time.Second))
		registry = redisRegistry
		closers = append(closers, redisClient.Close)
		if enableWS {
			redisSubscriber = redisroute.NewSubscriber(localRegistry, redisClient, routeConfig)
			go func() {
				log.Printf("push-gateway redis route subscriber started for gateway_id=%s", gatewayID)
				errs <- redisSubscriber.Run(ctx)
			}()
		}
	} else if routeBackend != "memory" {
		return errors.New("unsupported NEXUSIM_PUSH_ROUTE_BACKEND")
	}

	if enableWS {
		deliveryAddr := envString("NEXUSIM_DELIVERY_GRPC_ADDR", "127.0.0.1:10497")
		deliveryClient, closeDelivery, err := rpcinfra.DialDeliveryClient(
			ctx,
			deliveryAddr,
			envDuration("NEXUSIM_DELIVERY_GRPC_TIMEOUT", 500*time.Millisecond),
		)
		if err != nil {
			return err
		}
		closers = append(closers, closeDelivery)
		server := wsapi.NewServer(
			app.NewConnectSessionUseCase(registry),
			app.NewDisconnectSessionUseCase(registry),
			app.NewHandleClientFrameUseCase(deliveryClient),
			wsapi.Config{
				QueueSize:         envInt("NEXUSIM_PUSH_SESSION_QUEUE_SIZE", 256),
				HeartbeatInterval: envDuration("NEXUSIM_PUSH_HEARTBEAT_INTERVAL", 30*time.Second),
				WriteTimeout:      envDuration("NEXUSIM_PUSH_WRITE_TIMEOUT", 2*time.Second),
				WriteDelay:        envDuration("NEXUSIM_PUSH_TEST_WRITE_DELAY", 0),
			},
		)
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/metrics", func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(pushDebugMetrics{
				Metrics:              localRegistry.Metrics(),
				RedisRegistryMetrics: redisRouteRegistryMetrics(redisRegistry),
				RedisSubscriberStats: redisRouteSubscriberMetrics(redisSubscriber),
			})
		})
		mux.Handle("/", server)
		httpServer := &http.Server{
			Addr:              envString("NEXUSIM_PUSH_WS_ADDR", "0.0.0.0:10496"),
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			log.Printf("push-gateway websocket started on %s", httpServer.Addr)
			err := httpServer.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				err = context.Canceled
			}
			errs <- err
		}()
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(shutdownCtx)
		}()
	}

	if enableConsumer {
		topic := envString("NEXUSIM_DELIVERY_EVENTS_TOPIC", delivery.TopicDeliveryEvents)
		if topic != delivery.TopicDeliveryEvents {
			return errors.New("push-gateway may only consume im.delivery.events")
		}
		consumer, err := kafkainfra.NewReaderConsumer(kafkainfra.ReaderConfig{
			Brokers: splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS")),
			Topic:   topic,
			GroupID: envString("NEXUSIM_PUSH_CONSUMER_GROUP", "nexusim-push-gateway"),
		})
		if err != nil {
			return err
		}
		closers = append(closers, consumer.Close)
		worker := delivery.NewWorker(consumer, app.NewNotifyDeliveryUseCase(registry))
		go func() {
			log.Printf("push-gateway delivery consumer started")
			errs <- worker.Run(ctx)
		}()
	}

	var err error
	select {
	case err = <-errs:
	case <-ctx.Done():
		err = context.Canceled
	}
	stop()
	for _, closeFn := range closers {
		if closeErr := closeFn(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	return err
}

type pushDebugMetrics struct {
	memory.Metrics
	RedisRegistryMetrics redisroute.Metrics `json:"redis_registry_metrics,omitempty"`
	RedisSubscriberStats redisroute.Metrics `json:"redis_subscriber_metrics,omitempty"`
}

func redisRouteRegistryMetrics(registry *redisroute.Registry) redisroute.Metrics {
	if registry == nil {
		return redisroute.Metrics{}
	}
	return registry.Metrics()
}

func redisRouteSubscriberMetrics(subscriber *redisroute.Subscriber) redisroute.Metrics {
	if subscriber == nil {
		return redisroute.Metrics{}
	}
	return subscriber.Metrics()
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envIntAllowZero(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envDurationAllowZero(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	if value == "0" {
		return 0
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func defaultGatewayID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "gateway"
	}
	return hostname + "-" + strconv.Itoa(os.Getpid())
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
