package engine

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/user/research-assistant/internal/event"
)

func TestEngine_FSMBouncer(t *testing.T) {
	var inputProcessed int64
	var analysisCount int64

	handler := func(ev event.Event, p event.Publisher) {
		switch ev.Type {
		case event.TypeUserInputReceived:
			atomic.AddInt64(&inputProcessed, 1)
			fmt.Println("[TEST] Input received, starting analysis...")
			p.Publish(event.Event{Type: event.TypeAnalysisRequested})

		case event.TypeAnalysisRequested:
			atomic.AddInt64(&analysisCount, 1)
			// Simulate a delay that keeps the system in StateAnalyzing
			time.Sleep(100 * time.Millisecond)
			p.Publish(event.Event{Type: event.TypeSummaryRequested})
		}
	}

	en := New(10, 1, 1, handler)
	en.Start(context.Background())

	// 1. Send first input
	en.Publish(event.Event{Type: event.TypeUserInputReceived, Data: "Valid Task"})

	// 2. Immediately send second input while first is still "Analyzing"
	en.Publish(event.Event{Type: event.TypeUserInputReceived, Data: "Spam Task"})

	// Wait for processing to settle
	time.Sleep(300 * time.Millisecond)
	en.Stop()

	// We expect only ONE input to have been allowed by the bouncer
	if atomic.LoadInt64(&analysisCount) != 1 {
		t.Errorf("Bouncer failed: Expected 1 analysis to run, but got %d", analysisCount)
	}
}
