package engine

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/user/research-assistant/internal/event"
)

func TestEngine_Phase2Pipeline(t *testing.T) {
	var stagesCompleted int64

	// Each case in this handler represents a "Station" on the assembly line
	handler := func(ev event.Event, p event.Publisher) {
		switch ev.Type {
		case event.TypeUserInputReceived:
			atomic.AddInt64(&stagesCompleted, 1)
			p.Publish(event.Event{
				Type: event.TypeAnalysisRequested,
				Data: "Step 1 Done",
			})

		case event.TypeAnalysisRequested:
			atomic.AddInt64(&stagesCompleted, 1)
			p.Publish(event.Event{
				Type: event.TypeSummaryRequested,
				Data: "Step 2 Done",
			})

		case event.TypeSummaryRequested:
			atomic.AddInt64(&stagesCompleted, 1)
			p.Publish(event.Event{
				Type: event.TypeSummaryComplete,
				Data: "Step 3 Done",
			})

		case event.TypeSummaryComplete:
			atomic.AddInt64(&stagesCompleted, 1)
		}
	}

	en := New(10, 1, 1, handler)
	en.Start()

	// Start the pipeline
	en.Publish(event.Event{
		Type: event.TypeUserInputReceived,
		Data: "Starting Pipeline Test",
	})

	// Wait for the asynchronous stages to finish
	time.Sleep(100 * time.Millisecond)
	en.Stop()

	// We expect 4 total transitions:
	// 1. UserInputReceived (processed)
	// 2. AnalysisRequested (processed)
	// 3. SummaryRequested (processed)
	// 4. SummaryComplete (processed)
	expectedStages := int64(4)
	if actual := atomic.LoadInt64(&stagesCompleted); actual != expectedStages {
		t.Errorf("Pipeline incomplete. Expected %d stages to be processed, but got %d", expectedStages, actual)
	}

	fmt.Printf("--- Pipeline Test Successful: All %d stages verified ---\n", stagesCompleted)
}
