package pubsub_test

import (
	"context"
	"testing"
	"time"

	"github.com/user/research-assistant/internal/config"
	"github.com/user/research-assistant/internal/event"
	"github.com/user/research-assistant/internal/pubsub"
)

func TestRedisPubSub_Integration(t *testing.T) {
	// Load environment variables
	config.LoadEnv()
	addr := config.GetEnv("REDIS_ADDR", "localhost:6379")
	password := config.GetEnv("REDIS_PASSWORD", "")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	defer cancel()

	ps := pubsub.NewRedisPubSub(addr, password)

	contextID := "test-context-123"
	testEvent := event.Event{
		Type: event.TypeSearchRequested,
		Data: map[string]string{"query": "hello world"},
	}

	ch, err := ps.SubscribeEvents(ctx, contextID)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Small delay to ensure subscription is active
	time.Sleep(100 * time.Millisecond)

	if err := ps.PublishEvent(ctx, contextID, testEvent); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	select {
	case received := <-ch:
		if received.Type != testEvent.Type {
			t.Errorf("Expected type %s, got %s", testEvent.Type, received.Type)
		}
	case <-ctx.Done():
		t.Fatal("Timed out waiting for event")
	}
}
