package concierge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/gorilla/websocket"
	"github.com/user/research-assistant/internal/event"
	"github.com/user/research-assistant/internal/pubsub"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// wsJSONRequest resembles the JSON-RPC request expected over the socket.
type wsJSONRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// wsParams captures standard A2A params plus the extra contextId.
type wsParams struct {
	a2a.MessageSendParams
	ContextID string `json:"contextId"`
}

// wsJSONResponse resembles the JSON-RPC response or stream event.
type wsJSONResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

// HandleWebSocket upgrades the HTTP connection and bridges JSON to the A2A Executor.
func HandleWebSocket(handler a2asrv.RequestHandler, ps *pubsub.RedisPubSub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("[WS] Upgrade error: %v", err)
			return
		}
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("[WS] Read error: %v", err)
				}
				break
			}

			var req wsJSONRequest
			if err := json.Unmarshal(msg, &req); err != nil {
				sendWSError(conn, "", -32700, "Parse error")
				continue
			}

			if req.JSONRPC != "2.0" || req.Method == "" {
				sendWSError(conn, req.ID, -32600, "Invalid Request")
				continue
			}

			// We only support message/send or message/stream for now as a POC bridge.
			if req.Method != "message/send" && req.Method != "message/stream" {
				sendWSError(conn, req.ID, -32601, "Method not found")
				continue
			}

			var params wsParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				sendWSError(conn, req.ID, -32602, "Invalid params")
				continue
			}

			go func() {
				ctx, _ := a2asrv.WithCallContext(context.Background(), a2asrv.NewRequestMeta(r.Header))
				if params.ContextID != "" {
					params.Message.ContextID = params.ContextID
				}

				// Start Redis listener for this context
				if params.ContextID != "" {
					redisCtx, cancelRedis := context.WithCancel(ctx)
					defer cancelRedis()

					eventCh, err := ps.SubscribeEvents(redisCtx, params.ContextID)
					if err == nil {
						go func() {
							for ev := range eventCh {
								resp := wsJSONResponse{
									JSONRPC: "2.0",
									ID:      req.ID,
									Result: event.StatusUpdate{
										Kind:    "status",
										Type:    ev.Type,
										Message: fmt.Sprintf("%v", ev.Data),
									},
								}
								_ = conn.WriteJSON(resp)
							}
						}()
					}
				}

				if req.Method == "message/stream" {
					for ev, err := range handler.OnSendMessageStream(ctx, &params.MessageSendParams) {
						if err != nil {
							sendWSError(conn, req.ID, -32000, err.Error())
							return
						}
						resp := wsJSONResponse{
							JSONRPC: "2.0",
							ID:      req.ID,
							Result:  ev,
						}
						_ = conn.WriteJSON(resp)
					}
				} else {
					result, err := handler.OnSendMessage(ctx, &params.MessageSendParams)
					if err != nil {
						sendWSError(conn, req.ID, -32000, err.Error())
						return
					}
					resp := wsJSONResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result:  result,
					}
					_ = conn.WriteJSON(resp)
				}
			}()
		}
	}
}

func sendWSError(conn *websocket.Conn, id string, code int, message string) {
	resp := wsJSONResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: map[string]any{
			"code":    code,
			"message": message,
		},
	}
	_ = conn.WriteJSON(resp)
}
