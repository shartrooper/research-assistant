package researcher_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"github.com/user/research-assistant/internal/agent/researcher"
	"github.com/user/research-assistant/internal/event"
	"github.com/user/research-assistant/internal/pipeline"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// mockPublisher captures events published by the executor.
type mockPublisher struct {
	mu     sync.Mutex
	events map[string][]event.Event
}

func (m *mockPublisher) PublishEvent(_ context.Context, contextID string, ev event.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.events == nil {
		m.events = make(map[string][]event.Event)
	}
	m.events[contextID] = append(m.events[contextID], ev)
	return nil
}

// mockPipeline drives onUpdate with a fixed sequence then returns result/err.
type mockPipeline struct {
	sequence []struct{ status, detail string }
	result   *pipeline.Result
	err      error
}

func (m *mockPipeline) RunWithUpdates(_ context.Context, sessionID, _ string, onUpdate func(string, string)) (*pipeline.Result, error) {
	for _, s := range m.sequence {
		onUpdate(s.status, s.detail)
	}
	if m.result != nil {
		m.result.SessionID = sessionID
	}
	return m.result, m.err
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

func makeReqCtx(topic string) *a2asrv.RequestContext {
	return &a2asrv.RequestContext{
		TaskID:    a2a.TaskID("test-task-id"),
		ContextID: "test-context-id",
		Message: a2a.NewMessage(
			a2a.MessageRoleUser,
			a2a.TextPart{Text: topic},
		),
	}
}

func statusEvents(events []a2a.Event) []*a2a.TaskStatusUpdateEvent {
	var out []*a2a.TaskStatusUpdateEvent
	for _, e := range events {
		if s, ok := e.(*a2a.TaskStatusUpdateEvent); ok {
			out = append(out, s)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestResearcherExecutor_StreamsWorkingUpdates verifies that intermediate
// pipeline statuses (searching, structuring, writing_report) produce
// TaskStateWorking events in the queue, and that no error is returned.
func TestResearcherExecutor_StreamsWorkingUpdates(t *testing.T) {
	mock := &mockPipeline{
		sequence: []struct{ status, detail string }{
			{"searching", "query1"},
			{"searching", "query2"},
			{"searching", "query3"},
			{"structuring", ""},
			{"writing_report", ""},
			{"complete", "report-key.md"},
		},
		result: &pipeline.Result{ReportMDKey: "report-key.md", ReportJSONKey: "report-key.json"},
	}

	exec := researcher.New(mock, &mockPublisher{})
	q := &recordingQueue{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := exec.Execute(ctx, makeReqCtx("Go concurrency"), q); err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	statuses := statusEvents(q.events)
	if len(statuses) == 0 {
		t.Fatal("expected status update events in queue, got none")
	}

	// Count working events
	var workingCount int
	for _, s := range statuses {
		if s.Status.State == a2a.TaskStateWorking {
			workingCount++
		}
	}
	if workingCount < 5 { // 3 searching + structuring + writing_report
		t.Errorf("expected at least 5 TaskStateWorking events, got %d; events: %v", workingCount, statuses)
	}
}

// TestResearcherExecutor_CompletesWithArtifact verifies that a successful
// pipeline run produces a final TaskStateCompleted event with Final=true
// and a DataPart containing the session_id.
func TestResearcherExecutor_CompletesWithArtifact(t *testing.T) {
	mock := &mockPipeline{
		sequence: []struct{ status, detail string }{
			{"searching", "q"},
			{"structuring", ""},
			{"writing_report", ""},
			{"complete", "report.md"},
		},
		result: &pipeline.Result{
			ReportMDKey:   "report.md",
			ReportJSONKey: "report.json",
		},
	}

	exec := researcher.New(mock, &mockPublisher{})
	q := &recordingQueue{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := exec.Execute(ctx, makeReqCtx("test topic"), q); err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	statuses := statusEvents(q.events)
	if len(statuses) == 0 {
		t.Fatal("no status events written to queue")
	}

	// Last status event must be completed + Final
	last := statuses[len(statuses)-1]
	if last.Status.State != a2a.TaskStateCompleted {
		t.Errorf("expected final state %q, got %q", a2a.TaskStateCompleted, last.Status.State)
	}
	if !last.Final {
		t.Error("expected Final=true on completed event")
	}

	// Must carry result data (session_id) in a DataPart
	if last.Status.Message == nil {
		t.Fatal("expected status message with artifact data, got nil")
	}
	hasData := false
	for _, p := range last.Status.Message.Parts {
		if dp, ok := p.(a2a.DataPart); ok {
			if _, ok := dp.Data["session_id"]; ok {
				hasData = true
			}
		}
	}
	if !hasData {
		t.Errorf("expected DataPart with 'session_id' in final completed event; parts: %v", last.Status.Message.Parts)
	}
}

// TestResearcherExecutor_FailsOnPipelineError verifies that a pipeline failure
// produces a final TaskStateFailed event with Final=true and returns no error
// from Execute (the error was communicated via the queue).
func TestResearcherExecutor_FailsOnPipelineError(t *testing.T) {
	mock := &mockPipeline{
		sequence: []struct{ status, detail string }{
			{"searching", "q"},
			{"failed", "gemini quota exceeded"},
		},
		err: fmt.Errorf("gemini quota exceeded"),
	}

	exec := researcher.New(mock, &mockPublisher{})
	q := &recordingQueue{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := exec.Execute(ctx, makeReqCtx("test topic"), q); err != nil {
		t.Fatalf("Execute should not return an error (failure is communicated via queue); got: %v", err)
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	statuses := statusEvents(q.events)
	var hasFinalFailed bool
	for _, s := range statuses {
		if s.Status.State == a2a.TaskStateFailed && s.Final {
			hasFinalFailed = true
		}
	}
	if !hasFinalFailed {
		t.Errorf("expected a final TaskStateFailed event; got: %v", statuses)
	}
}
