package engine

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/user/research-assistant/internal/event"
)

func TestEngine_AsynchronousBursts(t *testing.T) {
	var processedCount int64

	// just counts events
	handler := func(ev event.Event, p event.Publisher) {
		atomic.AddInt64(&processedCount, 1)
	}
	en := New(100, 3, 10, handler)
	en.Start()

	// Parameters for the test
	numProducers := 5
	eventsPerProducer := 20
	timeframe := 500 * time.Millisecond

	start := time.Now()

	// Send events asynchronously
	for i := range numProducers {
		go func(id int) {
			for j := range eventsPerProducer {
				evType := event.TypeHeartbeat
				if j%2 == 0 {
					evType = event.TypeLog
				} else if j%3 == 0 {
					evType = event.TypeError
				}

				en.Publish(event.Event{
					Type: evType,
					Data: j,
				})
			}
		}(i)
	}

	// Wait for a bit to let them arrive, then stop
	time.Sleep(200 * time.Millisecond)
	en.Stop()

	elapsed := time.Since(start)
	expectedTotal := int64(numProducers * eventsPerProducer)
	if processedCount == expectedTotal {
		t.Logf("Test passed: %d events processed in %v", processedCount, elapsed)
	}
	if processedCount != expectedTotal {
		t.Errorf("Expected %d events, but got %d", expectedTotal, processedCount)
	}

	if elapsed > timeframe {
		t.Errorf("Test took too long: %v (max %v)", elapsed, timeframe)
	}
}
