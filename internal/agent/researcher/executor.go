package researcher

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"github.com/google/uuid"
	"github.com/user/research-assistant/internal/event"
	"github.com/user/research-assistant/internal/pipeline"
)

// PipelineRunner is the interface the executor requires from the pipeline.
type PipelineRunner interface {
	RunWithUpdates(ctx context.Context, sessionID, topic string, onUpdate func(status, detail string)) (*pipeline.Result, error)
}

// EventPublisher defines how the agent broadcasts transient status events.
type EventPublisher interface {
	PublishEvent(ctx context.Context, contextID string, ev event.Event) error
}

// Executor implements a2asrv.AgentExecutor for the Researcher agent.
type Executor struct {
	pipeline PipelineRunner
	pub      EventPublisher
}

// New creates an Executor backed by the given PipelineRunner and EventPublisher.
func New(pl PipelineRunner, pub EventPublisher) *Executor {
	return &Executor{pipeline: pl, pub: pub}
}

// Execute runs the research pipeline for the topic extracted from the incoming
// A2A message, streaming status updates via the queue.
func (e *Executor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	topic := extractTopic(reqCtx.Message)
	if topic == "" {
		return writeStatus(ctx, reqCtx, queue, a2a.TaskStateFailed, "empty research topic", true)
	}

	sessionID := uuid.New().String()
	log.Printf("[RESEARCHER] %s starting pipeline for topic: %q, session: %s", reqCtx.ContextID, topic, sessionID)

	result, pipeErr := e.pipeline.RunWithUpdates(ctx, sessionID, topic, func(status, detail string) {
		log.Printf("[RESEARCHER] %s pipeline update: status=%s, detail=%s", reqCtx.ContextID, status, detail)
		// Map internal status to event type for PubSub
		var evType event.EventType
		switch status {
		case "searching":
			evType = event.TypeSearchRequested
		case "structuring":
			evType = event.TypeStructuredDataReady
		case "writing_report":
			evType = event.TypeSummaryRequested
		case "failed":
			evType = event.TypeError
		case "complete":
			evType = event.TypeSummaryComplete
		}

		if evType != "" && e.pub != nil && reqCtx.ContextID != "" {
			if err := e.pub.PublishEvent(ctx, reqCtx.ContextID, event.Event{
				Type: evType,
				Data: detail,
			}); err != nil {
				log.Printf("[RESEARCHER] %s failed to publish event to Redis: %v", reqCtx.ContextID, err)
			}
		}

		switch status {
		case "searching":
			msg := "Searching: " + detail
			if err := writeStatus(ctx, reqCtx, queue, a2a.TaskStateWorking, msg, false); err != nil {
				log.Printf("[RESEARCHER] %s queue write error (searching): %v", reqCtx.ContextID, err)
			}
		case "structuring":
			if err := writeStatus(ctx, reqCtx, queue, a2a.TaskStateWorking, "Structuring findings", false); err != nil {
				log.Printf("[RESEARCHER] %s queue write error (structuring): %v", reqCtx.ContextID, err)
			}
		case "writing_report":
			if err := writeStatus(ctx, reqCtx, queue, a2a.TaskStateWorking, "Writing report", false); err != nil {
				log.Printf("[RESEARCHER] %s queue write error (writing_report): %v", reqCtx.ContextID, err)
			}
		case "failed":
			msg := detail
			if msg == "" {
				msg = "pipeline failed"
			}
			if err := writeStatus(ctx, reqCtx, queue, a2a.TaskStateFailed, msg, true); err != nil {
				log.Printf("[RESEARCHER] %s queue write error (failed): %v", reqCtx.ContextID, err)
			}
		case "complete":
			// Final completed event is emitted after RunWithUpdates returns so we
			// have access to the full Result. Skip here.
		}
	})

	if pipeErr != nil {
		log.Printf("[RESEARCHER] %s pipeline finished with error: %v", reqCtx.ContextID, pipeErr)
		// "failed" status already emitted via onUpdate; no error to return.
		return nil
	}

	log.Printf("[RESEARCHER] %s pipeline complete, writing final status", reqCtx.ContextID)
	// Emit final completed event with artifact data.
	return writeFinal(ctx, reqCtx, queue, result)
}

// Cancel signals the running pipeline to stop. The context passed to Execute
// is cancelled by the A2A server; this implementation writes a cancelled event.
func (e *Executor) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	return writeStatus(ctx, reqCtx, queue, a2a.TaskStateCanceled, "cancelled by client", true)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractTopic(msg *a2a.Message) string {
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
	var statusMsg *a2a.Message
	if text != "" {
		statusMsg = a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: text})
	}
	return queue.Write(ctx, &a2a.TaskStatusUpdateEvent{
		TaskID:    reqCtx.TaskID,
		ContextID: reqCtx.ContextID,
		Status: a2a.TaskStatus{
			State:   state,
			Message: statusMsg,
		},
		Final: final,
	})
}

func writeFinal(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue, result *pipeline.Result) error {
	if result == nil {
		return fmt.Errorf("writeFinal: result is nil")
	}
	dataMsg := a2a.NewMessage(
		a2a.MessageRoleAgent,
		a2a.DataPart{Data: map[string]any{
			"session_id":      result.SessionID,
			"report_md_key":   result.ReportMDKey,
			"report_json_key": result.ReportJSONKey,
		}},
	)
	return queue.Write(ctx, &a2a.TaskStatusUpdateEvent{
		TaskID:    reqCtx.TaskID,
		ContextID: reqCtx.ContextID,
		Status: a2a.TaskStatus{
			State:   a2a.TaskStateCompleted,
			Message: dataMsg,
		},
		Final: true,
	})
}
