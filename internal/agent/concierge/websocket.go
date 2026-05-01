package concierge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/gorilla/websocket"
	"github.com/user/research-assistant/internal/event"
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
func HandleWebSocket(handler a2asrv.RequestHandler, ps event.Subscriber) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("[WS] Upgrade error: %v", err)
			return
		}
		defer func(conn *websocket.Conn) {
			err := conn.Close()
			if err != nil {

			}
		}(conn)

		// Mutex to protect concurrent writes to the websocket connection.
		var mu sync.Mutex
		writeJSON := func(v any) error {
			mu.Lock()
			defer mu.Unlock()
			return conn.WriteJSON(v)
		}

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
				_ = writeJSON(wsJSONResponse{
					JSONRPC: "2.0",
					ID:      "",
					Error: map[string]any{
						"code":    -32700,
						"message": "Parse error",
					},
				})
				continue
			}

			if req.JSONRPC != "2.0" || req.Method == "" {
				_ = writeJSON(wsJSONResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: map[string]any{
						"code":    -32600,
						"message": "Invalid Request",
					},
				})
				continue
			}

			// We only support message/send or message/stream for now as a POC bridge.
			if req.Method != "message/send" && req.Method != "message/stream" {
				_ = writeJSON(wsJSONResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: map[string]any{
						"code":    -32601,
						"message": "Method not found",
					},
				})
				continue
			}

			var params wsParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				_ = writeJSON(wsJSONResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: map[string]any{
						"code":    -32602,
						"message": "Invalid params",
					},
				})
				continue
			}

			go func() {
				ctx, _ := a2asrv.WithCallContext(context.Background(), a2asrv.NewRequestMeta(r.Header))
				if params.ContextID != "" {
					if params.Message == nil {
						params.Message = &a2a.Message{}
					}
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
								_ = writeJSON(resp)
							}
						}()
					}
				}

				if req.Method == "message/stream" {
					for ev, err := range handler.OnSendMessageStream(ctx, &params.MessageSendParams) {
						if err != nil {
							_ = writeJSON(wsJSONResponse{
								JSONRPC: "2.0",
								ID:      req.ID,
								Error: map[string]any{
									"code":    -32000,
									"message": err.Error(),
								},
							})
							return
						}
						resp := wsJSONResponse{
							JSONRPC: "2.0",
							ID:      req.ID,
							Result:  ev,
						}
						_ = writeJSON(resp)
					}
				} else {
					result, err := handler.OnSendMessage(ctx, &params.MessageSendParams)
					if err != nil {
						_ = writeJSON(wsJSONResponse{
							JSONRPC: "2.0",
							ID:      req.ID,
							Error: map[string]any{
								"code":    -32000,
								"message": err.Error(),
							},
						})
						return
					}
					resp := wsJSONResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result:  result,
					}
					_ = writeJSON(resp)
				}
			}()
		}
	}
}
