package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/user/research-assistant/internal/event"
)

func TestEngine_EventChaining(t *testing.T) {
	var analysisStarted int64
	var summaryFinished int64

	// This handler demonstrates "Chaining"
	// AnalysisRequested -> AnalysisComplete
	handler := func(ev event.Event, p event.Publisher) {
		switch ev.Type {
		case event.TypeAnalysisRequested:
			atomic.AddInt64(&analysisStarted, 1)
			// CHAIN: When analysis is requested, automatically publish a completion event
			p.Publish(event.Event{
				Type: event.TypeSummaryComplete,
				Data: "Summary result",
			})
		case event.TypeSummaryComplete:
			atomic.AddInt64(&summaryFinished, 1)
		}
	}

	en := New(10, 1, 1, handler)
	en.Start(context.Background())

	// 1. Publish the initial event
	en.Publish(event.Event{
		Type: event.TypeAnalysisRequested,
		Data: "Unit Test Task",
	})

	// 2. Wait for the chain to complete
	// We use a small sleep because the events are processed in the background
	time.Sleep(50 * time.Millisecond)
	en.Stop()

	// 3. Verify both stages of the chain were reached
	if atomic.LoadInt64(&analysisStarted) != 1 {
		t.Error("Chain failed: AnalysisRequested was never processed")
	}
	if atomic.LoadInt64(&summaryFinished) != 1 {
		t.Error("Chain failed: SummaryComplete was never triggered from the request")
	}
}
