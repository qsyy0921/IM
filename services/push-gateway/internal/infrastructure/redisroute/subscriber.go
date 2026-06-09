package redisroute

import (
	"context"
	"encoding/json"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

type Subscriber struct {
	local  LocalRegistry
	client redis.UniversalClient
	config Config
}

func NewSubscriber(local LocalRegistry, client redis.UniversalClient, config Config) *Subscriber {
	if config.KeyPrefix == "" {
		config.KeyPrefix = defaultKeyPrefix
	}
	return &Subscriber{local: local, client: client, config: config}
}

func (subscriber *Subscriber) Run(ctx context.Context) error {
	pubsub := subscriber.client.Subscribe(ctx, GatewayChannel(subscriber.config.KeyPrefix, subscriber.config.GatewayID))
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		return err
	}
	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message, ok := <-channel:
			if !ok {
				return nil
			}
			var notification types.DeliveryNotification
			if err := json.Unmarshal([]byte(message.Payload), &notification); err != nil {
				return err
			}
			if _, err := subscriber.local.EnqueueNotification(ctx, notification); err != nil {
				return err
			}
		}
	}
}
