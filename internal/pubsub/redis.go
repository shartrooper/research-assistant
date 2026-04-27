package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/user/research-assistant/internal/event"
)

type RedisPubSub struct {
	client *redis.Client
}

func NewRedisPubSub(addr, password string) *RedisPubSub {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})
	return &RedisPubSub{client: rdb}
}

func (ps *RedisPubSub) Ping(ctx context.Context) error {
	return ps.client.Ping(ctx).Err()
}

func (ps *RedisPubSub) PublishEvent(ctx context.Context, contextID string, ev event.Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	channel := fmt.Sprintf("agent:events:%s", contextID)
	return ps.client.Publish(ctx, channel, payload).Err()
}

func (ps *RedisPubSub) SubscribeEvents(ctx context.Context, contextID string) (<-chan event.Event, error) {
	channel := fmt.Sprintf("agent:events:%s", contextID)
	pubsub := ps.client.Subscribe(ctx, channel)

	// Check if subscription was successful
	_, err := pubsub.Receive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to receive from pubsub: %w", err)
	}

	ch := make(chan event.Event)

	go func() {
		defer close(ch)
		defer pubsub.Close()

		redisCh := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-redisCh:
				if !ok {
					return
				}
				var ev event.Event
				if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
					// In a real app, we might want to log this or handle it better
					continue
				}
				ch <- ev
			}
		}
	}()

	return ch, nil
}
