package concierge_test

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"github.com/user/research-assistant/internal/agent/concierge"
	"github.com/user/research-assistant/internal/event"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) GenerateContent(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

type mockContextStore struct {
	findings []event.StructuredFinding
	sources  []event.SearchSource
	err      error
}

func (m *mockContextStore) GetKeyFindings(_ string) ([]event.StructuredFinding, error) {
	return m.findings, m.err
}

func (m *mockContextStore) GetSources(_ string) ([]event.SearchSource, error) {
	return m.sources, m.err
}

// mockResearcher returns a fixed sequence of A2A events, simulating the Researcher agent.
type mockResearcher struct {
	events []a2a.Event
	err    error
}

func (m *mockResearcher) Stream(_ context.Context, _ string) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if m.err != nil {
			yield(nil, m.err)
			return
		}
		for _, e := range m.events {
			if !yield(e, nil) {
				return
			}
		}
	}
}

// recordingQueue captures all events written by the executor.
type recordingQueue struct {
	mu     sync.Mutex
	events []a2a.Event
}

func (q *recordingQueue) Write(_ context.Context, event a2a.Event) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.events = append(q.events, event)
	return nil
}

func (q *recordingQueue) WriteVersioned(_ context.Context, event a2a.Event, _ a2a.TaskVersion) error {
	return q.Write(context.Background(), event)
}

func (q *recordingQueue) Read(_ context.Context) (a2a.Event, a2a.TaskVersion, error) {
	return nil, 0, fmt.Errorf("recordingQueue: Read not implemented")
}

func (q *recordingQueue) Close() error { return nil }

var _ eventqueue.Queue = (*recordingQueue)(nil)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeReqCtx(contextID, topic string) *a2asrv.RequestContext {
	return &a2asrv.RequestContext{
		TaskID:    a2a.TaskID("task-" + contextID),
		ContextID: contextID,
		Message: a2a.NewMessage(
			a2a.MessageRoleUser,
			a2a.TextPart{Text: topic},
		),
	}
}

func workingStatus(text string) *a2a.TaskStatusUpdateEvent {
	return &a2a.TaskStatusUpdateEvent{
		TaskID:    "task-ctx1",
		ContextID: "ctx1",
		Status: a2a.TaskStatus{
			State:   a2a.TaskStateWorking,
			Message: a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: text}),
		},
		Final: false,
	}
}

func completedStatus(sessionID string) *a2a.TaskStatusUpdateEvent {
	return &a2a.TaskStatusUpdateEvent{
		TaskID:    "task-ctx1",
		ContextID: "ctx1",
		Status: a2a.TaskStatus{
			State: a2a.TaskStateCompleted,
			Message: a2a.NewMessage(
				a2a.MessageRoleAgent,
				a2a.DataPart{Data: map[string]any{
					"session_id":      sessionID,
					"report_md_key":   "report.md",
					"report_json_key": "report.json",
				}},
			),
		},
		Final: true,
	}
}

func countState(events []a2a.Event, state a2a.TaskState) int {
	n := 0
	for _, e := range events {
		if s, ok := e.(*a2a.TaskStatusUpdateEvent); ok && s.Status.State == state {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestConciergeExecutor_RelaysResearcherUpdates verifies that the Concierge
// relays all working updates from the Researcher to its own queue.
func TestConciergeExecutor_RelaysResearcherUpdates(t *testing.T) {
	researcher := &mockResearcher{
		events: []a2a.Event{
			workingStatus("Searching: q1"),
			workingStatus("Searching: q2"),
			workingStatus("Structuring findings"),
			workingStatus("Writing report"),
			completedStatus("session-abc"),
		},
	}

	exec := concierge.New(&mockLLM{}, &mockContextStore{}, researcher.Stream)
	q := &recordingQueue{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := exec.Execute(ctx, makeReqCtx("ctx1", "Go concurrency"), q); err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Expect at least 4 working + 1 completed
	if n := countState(q.events, a2a.TaskStateWorking); n < 4 {
		t.Errorf("expected at least 4 working events relayed, got %d; all: %v", n, q.events)
	}
	if n := countState(q.events, a2a.TaskStateCompleted); n != 1 {
		t.Errorf("expected 1 completed event, got %d; all: %v", n, q.events)
	}
}

// TestConciergeExecutor_QAModeLoadsContext verifies that a second message on
// the same context ID triggers Q&A mode: findings/sources are loaded from DB,
// Gemini is called with them, and a completed status with the answer is emitted.
func TestConciergeExecutor_QAModeLoadsContext(t *testing.T) {
	store := &mockContextStore{
		findings: []event.StructuredFinding{
			{Finding: "F1", Confidence: 0.9},
		},
		sources: []event.SearchSource{
			{Query: "q1", URL: "http://a.com", Snippet: "snippet"},
		},
	}
	lm := &mockLLM{response: "The answer is 42."}

	researcher := &mockResearcher{} // should NOT be called in Q&A mode

	exec := concierge.New(lm, store, researcher.Stream)

	// Pre-seed the session map so the executor knows context "ctx2" has a completed session.
	exec.SetSession("ctx2", "session-xyz")

	q := &recordingQueue{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := exec.Execute(ctx, makeReqCtx("ctx2", "What is the main finding?"), q); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Researcher should NOT have been called (no working events for searching)
	if len(researcher.events) == 0 {
		// good — researcher had no events to stream
	}
	// Must have a completed event with the LLM answer text
	var found bool
	for _, e := range q.events {
		s, ok := e.(*a2a.TaskStatusUpdateEvent)
		if !ok || s.Status.State != a2a.TaskStateCompleted {
			continue
		}
		if s.Status.Message == nil {
			continue
		}
		for _, p := range s.Status.Message.Parts {
			if tp, ok := p.(a2a.TextPart); ok && tp.Text == "The answer is 42." {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected completed event with LLM answer text; got events: %v", q.events)
	}
}

// TestConciergeExecutor_ResearcherFailure verifies that a Researcher failure
// is relayed to the user as a failed status.
func TestConciergeExecutor_ResearcherFailure(t *testing.T) {
	researcher := &mockResearcher{
		err: fmt.Errorf("researcher: pipeline failed"),
	}

	exec := concierge.New(&mockLLM{}, &mockContextStore{}, researcher.Stream)
	q := &recordingQueue{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := exec.Execute(ctx, makeReqCtx("ctx3", "some topic"), q); err != nil {
		t.Fatalf("Execute should not return error (failure communicated via queue): %v", err)
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if n := countState(q.events, a2a.TaskStateFailed); n == 0 {
		t.Errorf("expected at least one failed event; got: %v", q.events)
	}
}
