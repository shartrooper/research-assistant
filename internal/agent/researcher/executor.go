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
	"github.com/user/research-assistant/internal/pipeline"
)

// PipelineRunner is the interface the executor requires from the pipeline.
type PipelineRunner interface {
	RunWithUpdates(ctx context.Context, sessionID, topic string, onUpdate func(status, detail string)) (*pipeline.Result, error)
}

// Executor implements a2asrv.AgentExecutor for the Researcher agent.
type Executor struct {
	pipeline PipelineRunner
}

// New creates an Executor backed by the given PipelineRunner.
func New(pl PipelineRunner) *Executor {
	return &Executor{pipeline: pl}
}

// Execute runs the research pipeline for the topic extracted from the incoming
// A2A message, streaming status updates via the queue.
func (e *Executor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	topic := extractTopic(reqCtx.Message)
	if topic == "" {
		return writeStatus(ctx, reqCtx, queue, a2a.TaskStateFailed, "empty research topic", true)
	}

	sessionID := uuid.New().String()

	result, pipeErr := e.pipeline.RunWithUpdates(ctx, sessionID, topic, func(status, detail string) {
		switch status {
		case "searching":
			msg := "Searching: " + detail
			if err := writeStatus(ctx, reqCtx, queue, a2a.TaskStateWorking, msg, false); err != nil {
				log.Printf("[RESEARCHER] queue write error: %v", err)
			}
		case "structuring":
			if err := writeStatus(ctx, reqCtx, queue, a2a.TaskStateWorking, "Structuring findings", false); err != nil {
				log.Printf("[RESEARCHER] queue write error: %v", err)
			}
		case "writing_report":
			if err := writeStatus(ctx, reqCtx, queue, a2a.TaskStateWorking, "Writing report", false); err != nil {
				log.Printf("[RESEARCHER] queue write error: %v", err)
			}
		case "failed":
			msg := detail
			if msg == "" {
				msg = "pipeline failed"
			}
			if err := writeStatus(ctx, reqCtx, queue, a2a.TaskStateFailed, msg, true); err != nil {
				log.Printf("[RESEARCHER] queue write error: %v", err)
			}
		case "complete":
			// Final completed event is emitted after RunWithUpdates returns so we
			// have access to the full Result. Skip here.
		}
	})

	if pipeErr != nil {
		// "failed" status already emitted via onUpdate; no error to return.
		return nil
	}

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
