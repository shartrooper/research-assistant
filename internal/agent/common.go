package agent

import (
	"context"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	apperrors "github.com/user/research-assistant/internal/errors"
)

// ExtractText concatenates all text parts from an A2A message into a single string.
func ExtractText(msg *a2a.Message) string {
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

// WriteStatus sends a status update event to the A2A queue.
func WriteStatus(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue, state a2a.TaskState, text string, final bool) error {
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

// WriteAppError sends a structured application error to the A2A queue.
func WriteAppError(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue, state a2a.TaskState, appErr *apperrors.AppError) error {
	msg := a2a.NewMessage(a2a.MessageRoleAgent)
	msg.Parts = append(msg.Parts, a2a.TextPart{Text: appErr.UserMessage})

	data := map[string]any{
		"kind":   "error_meta",
		"code":   appErr.Code,
		"source": appErr.Source,
	}
	if appErr.Recovery != nil {
		data["recovery"] = appErr.Recovery
	}
	if len(appErr.Telemetry) > 0 {
		data["telemetry"] = appErr.Telemetry
	}
	msg.Parts = append(msg.Parts, a2a.DataPart{Data: data})

	return queue.Write(ctx, &a2a.TaskStatusUpdateEvent{
		TaskID:    reqCtx.TaskID,
		ContextID: reqCtx.ContextID,
		Status:    a2a.TaskStatus{State: state, Message: msg},
		Final:     true,
	})
}
