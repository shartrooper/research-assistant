package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user/research-assistant/internal/engine"
	"github.com/user/research-assistant/internal/event"
)

func main() {
	fmt.Println("Starting Research Assistant...")

	en := engine.New(32, func(ev event.Event) {
		switch ev.Type {
		case event.TypeHeartbeat:
			fmt.Printf("[MAIN] %s: System heartbeat at %v\n", ev.Type, ev.Data)
		default:
			fmt.Printf("[MAIN] Unknown event type: %s\n", ev.Type)
		}
	})

	en.Start()

	en.Publish(event.Event{
		Type: event.TypeHeartbeat,
		Data: time.Now(),
	})

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Println("Shutting down...")
	en.Stop()
	fmt.Println("Shutdown complete")
}
