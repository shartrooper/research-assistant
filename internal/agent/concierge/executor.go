package concierge

import (
	"context"
	"fmt"
	"iter"
	"log"
	"strings"
	"sync"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"github.com/user/research-assistant/internal/event"
)

// LLMClient generates content given a prompt.
type LLMClient interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}

// ContextStore provides read access to persisted research data for Q&A mode.
type ContextStore interface {
	GetKeyFindings(sessionID string) ([]event.StructuredFinding, error)
	GetSources(sessionID string) ([]event.SearchSource, error)
}

// ResearchStream sends a research topic to the Researcher agent and returns
// a streaming iterator of A2A events.
type ResearchStream func(ctx context.Context, topic string, contextID string) iter.Seq2[a2a.Event, error]

// Executor implements a2asrv.AgentExecutor for the Concierge agent.
type Executor struct {
	llm        LLMClient
	db         ContextStore
	researcher ResearchStream
	sub        event.Subscriber

	mu       sync.RWMutex
	sessions map[string]string // contextID → researchSessionID
}

// New creates a Concierge Executor.
func New(llm LLMClient, db ContextStore, researcher ResearchStream, sub event.Subscriber) *Executor {
	return &Executor{
		llm:        llm,
		db:         db,
		researcher: researcher,
		sub:        sub,
		sessions:   make(map[string]string),
	}
}

// SetSession pre-seeds the contextID→sessionID mapping (used in tests and for
// restoring state after a restart).
func (e *Executor) SetSession(contextID, sessionID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sessions[contextID] = sessionID
}

func (e *Executor) getSession(contextID string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	id, ok := e.sessions[contextID]
	return id, ok
}

func (e *Executor) storeSession(contextID, sessionID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sessions[contextID] = sessionID
}

// Execute handles an incoming A2A task: dispatches to research mode or Q&A
// mode depending on whether a completed session exists for the context.
func (e *Executor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	if sessionID, ok := e.getSession(reqCtx.ContextID); ok {
		log.Printf("[CONCIERGE] %s Q&A turn for session %s", reqCtx.ContextID, sessionID)
		return e.handleQA(ctx, reqCtx, queue, sessionID)
	}
	log.Printf("[CONCIERGE] %s new research request", reqCtx.ContextID)
	return e.handleResearch(ctx, reqCtx, queue)
}

// Cancel signals cancellation for the given task.
func (e *Executor) Cancel(_ context.Context, _ *a2asrv.RequestContext, _ eventqueue.Queue) error {
	return nil
}

// ---------------------------------------------------------------------------
// Research mode
// ---------------------------------------------------------------------------

func (e *Executor) handleResearch(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	topic := extractText(reqCtx.Message)
	if topic == "" {
		return writeStatus(ctx, reqCtx, queue, a2a.TaskStateFailed, "empty research topic", true)
	}

	// Start a Redis listener to relay out-of-band events to the A2A stream.
	// This ensures that even if the streaming Researcher response is buffered,
	// the client still gets granular updates via the A2A queue.
	if e.sub != nil && reqCtx.ContextID != "" {
		relayCtx, cancelRelay := context.WithCancel(ctx)
		defer cancelRelay()

		eventCh, err := e.sub.SubscribeEvents(relayCtx, reqCtx.ContextID)
		if err == nil {
			go func() {
				for ev := range eventCh {
					msg := fmt.Sprintf("%v", ev.Data)
					log.Printf("[CONCIERGE] %s relaying Redis event to A2A: %s", reqCtx.ContextID, ev.Type)
					_ = writeStatus(ctx, reqCtx, queue, a2a.TaskStateWorking, msg, false)
				}
			}()
		}
	}

	stream := e.researcher(ctx, topic, reqCtx.ContextID)
	for ev, err := range stream {
		if err != nil {
			log.Printf("[CONCIERGE] researcher stream error: %v", err)
			_ = writeStatus(ctx, reqCtx, queue, a2a.TaskStateFailed, err.Error(), true)
			return nil
		}

		var status a2a.TaskStatus
		var final bool

		switch typed := ev.(type) {
		case *a2a.TaskStatusUpdateEvent:
			status = typed.Status
			final = typed.Final
		case *a2a.Task:
			status = typed.Status
			final = typed.Status.State == a2a.TaskStateCompleted ||
				typed.Status.State == a2a.TaskStateFailed ||
				typed.Status.State == a2a.TaskStateCanceled
		default:
			continue
		}

		// Relay the event to the user, re-addressed to this task.
		log.Printf("[CONCIERGE] %s relaying researcher event: state=%s, final=%v", reqCtx.ContextID, status.State, final)
		relayed := &a2a.TaskStatusUpdateEvent{
			TaskID:    reqCtx.TaskID,
			ContextID: reqCtx.ContextID,
			Status:    status,
			Final:     final,
		}
		if err := queue.Write(ctx, relayed); err != nil {
			log.Printf("[CONCIERGE] queue write error: %v", err)
		}

		// When the researcher reports completion, extract session_id and store it.
		if status.State == a2a.TaskStateCompleted {
			if sessionID := extractSessionIDFromStatus(status); sessionID != "" {
				e.storeSession(reqCtx.ContextID, sessionID)
				log.Printf("[CONCIERGE] %s research complete, session %s", reqCtx.ContextID, sessionID)
			}
			return nil
		}
		if status.State == a2a.TaskStateFailed || status.State == a2a.TaskStateCanceled {
			return nil
		}
	}
	return nil
}

func extractSessionIDFromStatus(status a2a.TaskStatus) string {
	if status.Message == nil {
		return ""
	}
	for _, p := range status.Message.Parts {
		if dp, ok := p.(a2a.DataPart); ok {
			if id, ok := dp.Data["session_id"].(string); ok {
				return id
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Q&A mode
// ---------------------------------------------------------------------------

func (e *Executor) handleQA(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue, sessionID string) error {
	question := extractText(reqCtx.Message)

	findings, err := e.db.GetKeyFindings(sessionID)
	if err != nil {
		log.Printf("[CONCIERGE] GetKeyFindings error: %v", err)
	}
	sources, err := e.db.GetSources(sessionID)
	if err != nil {
		log.Printf("[CONCIERGE] GetSources error: %v", err)
	}

	prompt := buildQAPrompt(question, findings, sources)
	answer, err := e.llm.GenerateContent(ctx, prompt)
	if err != nil {
		return writeStatus(ctx, reqCtx, queue, a2a.TaskStateFailed, fmt.Sprintf("LLM error: %v", err), true)
	}

	log.Printf("[CONCIERGE] %s generated QA answer, writing to queue", reqCtx.ContextID)
	return queue.Write(ctx, &a2a.TaskStatusUpdateEvent{
		TaskID:    reqCtx.TaskID,
		ContextID: reqCtx.ContextID,
		Status: a2a.TaskStatus{
			State:   a2a.TaskStateCompleted,
			Message: a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: answer}),
		},
		Final: true,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractText(msg *a2a.Message) string {
	if msg == nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range msg.Parts {
		if tp, ok := p.(a2a.TextPart); ok {
			sb.WriteString(tp.Text)
		}
	}
	return strings.TrimSpace(sb.String())
}

func writeStatus(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue, state a2a.TaskState, text string, final bool) error {
	var msg *a2a.Message
	if text != "" {
		msg = a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: text})
	}
	return queue.Write(ctx, &a2a.TaskStatusUpdateEvent{
		TaskID:    reqCtx.TaskID,
		ContextID: reqCtx.ContextID,
		Status:    a2a.TaskStatus{State: state, Message: msg},
		Final:     final,
	})
}

func buildQAPrompt(question string, findings []event.StructuredFinding, sources []event.SearchSource) string {
	var sb strings.Builder
	sb.WriteString("You are a research assistant. Answer the following question using ONLY the research context below.\n\n")
	sb.WriteString("Question: ")
	sb.WriteString(question)
	sb.WriteString("\n\n")

	if len(findings) > 0 {
		sb.WriteString("Key Findings:\n")
		for _, f := range findings {
			sb.WriteString(fmt.Sprintf("- %s (confidence: %.2f)\n", f.Finding, f.Confidence))
		}
		sb.WriteString("\n")
	}

	if len(sources) > 0 {
		sb.WriteString("Sources:\n")
		for _, s := range sources {
			sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", s.Query, s.URL, s.Snippet))
		}
	}

	return sb.String()
}
