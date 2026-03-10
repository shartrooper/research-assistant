package concierge

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/gorilla/websocket"
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

// wsJSONResponse resembles the JSON-RPC response or stream event.
type wsJSONResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

// HandleWebSocket upgrades the HTTP connection and bridges JSON to the A2A Executor.
func HandleWebSocket(handler a2asrv.RequestHandler) http.HandlerFunc {
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

			var params *a2a.MessageSendParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				sendWSError(conn, req.ID, -32602, "Invalid params")
				continue
			}

			go func() {
				ctx, callCtx := a2asrv.WithCallContext(context.Background(), a2asrv.NewRequestMeta(r.Header))

				// Re-route context/task IDs properly for the server (v0.3.7 extension headers via meta)
				// Setup basic call context properties if needed
				_ = callCtx

				if req.Method == "message/stream" {
					for ev, err := range handler.OnSendMessageStream(ctx, params) {
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
					result, err := handler.OnSendMessage(ctx, params)
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
