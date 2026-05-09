package main

import (
	"context"
	"encoding/json"
	"iter"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/gorilla/websocket"
	"github.com/user/research-assistant/internal/agent/concierge"
	"github.com/user/research-assistant/internal/agent/researcher"
	"github.com/user/research-assistant/internal/event"
	"github.com/user/research-assistant/internal/pipeline"
	"github.com/user/research-assistant/internal/storage"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockLLM struct{}

func (m *mockLLM) GenerateContent(_ context.Context, prompt string) (string, error) {
	if strings.Contains(prompt, "generate 3 specific search queries") {
		return `["query 1", "query 2", "query 3"]`, nil
	}
	if strings.Contains(prompt, "Convert the search results into the following JSON schema") {
		return `{"topic": "test", "key_findings": [{"finding": "Mock finding", "confidence": 0.9}], "sources": [{"url": "http://test.com", "query": "query 1", "snippet": "Mock snippet"}]}`, nil
	}
	if strings.Contains(prompt, "Write a comprehensive report") {
		return "Mock Research Report", nil
	}
	if strings.Contains(prompt, "Create a short executive summary") {
		return "- Point 1\n- Point 2", nil
	}
	if strings.Contains(prompt, "Answer the following question using ONLY the research context") {
		return "Mock Answer based on context.", nil
	}
	return "Mock Response", nil
}

type inMemPubSub struct {
	mu   sync.Mutex
	subs map[string][]chan event.Event
}

func newInMemPubSub() *inMemPubSub {
	return &inMemPubSub{subs: make(map[string][]chan event.Event)}
}

func (p *inMemPubSub) PublishEvent(_ context.Context, contextID string, ev event.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, ch := range p.subs[contextID] {
		select {
		case ch <- ev:
		default:
		}
	}
	return nil
}

func (p *inMemPubSub) SubscribeEvents(_ context.Context, contextID string) (<-chan event.Event, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch := make(chan event.Event, 10)
	p.subs[contextID] = append(p.subs[contextID], ch)
	return ch, nil
}

// ---------------------------------------------------------------------------
// Main Harness
// ---------------------------------------------------------------------------

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Setup Infrastructure
	ps := newInMemPubSub()
	llm := &mockLLM{}
	dbPath := "e2e_research.db"
	err := os.Remove(dbPath)
	if err != nil {
		return
	}
	db, _ := storage.NewSQLiteStore(dbPath)
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {

		}
	}(dbPath)
	blobs, _ := storage.NewDiskBlobStore("e2e_artifacts")
	defer func() {
		err := os.RemoveAll("e2e_artifacts")
		if err != nil {

		}
	}()

	searchFn := func(ctx context.Context, query string) ([]pipeline.SearchResult, error) {
		return []pipeline.SearchResult{{Content: "Mock content for " + query, URL: "http://test.com/" + query}}, nil
	}

	// 2. Start Researcher Agent
	resPl := pipeline.New(llm, searchFn, db, blobs)
	resExec := researcher.New(resPl, ps)
	resListener, _ := net.Listen("tcp", "127.0.0.1:0")
	resAddr := "http://" + resListener.Addr().String()
	resSrv := &http.Server{Handler: a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(resExec))}
	go func() {
		err := resSrv.Serve(resListener)
		if err != nil {

		}
	}()
	defer func(resSrv *http.Server, ctx context.Context) {
		err := resSrv.Shutdown(ctx)
		if err != nil {

		}
	}(resSrv, ctx)
	log.Printf("[E2E] Researcher started at %s", resAddr)

	// 3. Start Concierge Agent
	researcherCard := &a2a.AgentCard{URL: resAddr, PreferredTransport: "JSONRPC", ProtocolVersion: "0.2.2"}
	researchStream := func(sctx context.Context, topic string, contextID string) iter.Seq2[a2a.Event, error] {
		return func(yield func(a2a.Event, error) bool) {
			log.Printf("[E2E] Concierge calling Researcher for topic %q", topic)
			client, err := a2aclient.NewFromCard(sctx, researcherCard)
			if err != nil {
				log.Printf("[E2E] Researcher client error: %v", err)
				yield(nil, err)
				return
			}
			msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: topic})
			msg.ContextID = contextID
			params := &a2a.MessageSendParams{Message: msg}
			log.Printf("[E2E] Sending streaming message to Researcher...")
			for ev, err := range client.SendStreamingMessage(sctx, params) {
				if err != nil {
					log.Printf("[E2E] Researcher stream error: %v", err)
				} else {
					log.Printf("[E2E] Concierge received Researcher event: %T", ev)
				}
				if !yield(ev, err) {
					return
				}
			}
			log.Printf("[E2E] Researcher stream closed")
		}
	}
	conExec := concierge.New(llm, db, researchStream, ps)
	conListener, _ := net.Listen("tcp", "127.0.0.1:0")
	conWSAddr := "ws://" + conListener.Addr().String() + "/ws"
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", concierge.HandleWebSocket(a2asrv.NewHandler(conExec), ps))
	conSrv := &http.Server{Handler: mux}
	go func() {
		err := conSrv.Serve(conListener)
		if err != nil {

		}
	}()
	defer func(conSrv *http.Server, ctx context.Context) {
		err := conSrv.Shutdown(ctx)
		if err != nil {

		}
	}(conSrv, ctx)
	log.Printf("[E2E] Concierge started at %s", conWSAddr)

	// 4. Client Simulation
	log.Println("[E2E] Connecting to Concierge WS...")
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(conWSAddr, nil)
	if err != nil {
		log.Fatalf("[E2E] WS Dial error: %v", err)
	}
	defer func(conn *websocket.Conn) {
		err := conn.Close()
		if err != nil {

		}
	}(conn)

	// 5. Send Research Request
	reqID := "req-1"
	ctxID := "context-e2e"
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      reqID,
		"method":  "message/stream",
		"params": map[string]any{
			"contextId": ctxID,
			"message": map[string]any{
				"kind":      "message",
				"messageId": "msg-1",
				"role":      "user",
				"parts": []map[string]any{
					{"kind": "text", "text": "Future of Go"},
				},
			},
		},
	}
	if err := conn.WriteJSON(req); err != nil {
		log.Fatalf("[E2E] WriteJSON error: %v", err)
	}

	// 6. Verify Events
	log.Println("[E2E] Waiting for events...")
	var (
		hasSearching   bool
		hasStructuring bool
		hasWriting     bool
		hasCompleted   bool
		sessionID      string
	)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Fatalf("[E2E] Read error: %v", err)
		}

		var resp struct {
			Result any `json:"result"`
		}
		if err := json.Unmarshal(message, &resp); err != nil {
			log.Printf("[E2E] JSON Unmarshal error: %v, raw: %s", err, string(message))
			continue
		}

		// Check if it's a Redis status update
		if m, ok := resp.Result.(map[string]any); ok {
			if m["kind"] == "status" {
				log.Printf("[E2E] RECV Status: %v - %v", m["type"], m["message"])
				switch m["type"] {
				case string(event.TypeSearchRequested):
					hasSearching = true
				case string(event.TypeStructuredDataReady):
					hasStructuring = true
				case string(event.TypeSummaryRequested):
					hasWriting = true
				}
			}

			// Check if it's an A2A task status update (handle both task_id and taskId)
			taskID := m["task_id"]
			if taskID == nil {
				taskID = m["taskId"]
			}

			if taskID != nil {
				status := m["status"].(map[string]any)
				state := status["state"].(string)
				log.Printf("[E2E] RECV A2A Task: %v, State: %s", taskID, state)

				if state == string(a2a.TaskStateCompleted) {
					hasCompleted = true
					// Extract session_id
					msg := status["message"].(map[string]any)
					parts := msg["parts"].([]any)
					for _, p := range parts {
						part := p.(map[string]any)
						if part["kind"] == "data" {
							data := part["data"].(map[string]any)
							sessionID = data["session_id"].(string)
						}
					}
					break // Successfully received final event
				}
			} else if m["kind"] != "status" {
				log.Printf("[E2E] RECV Unknown: %+v", m)
			}
		} else {
			log.Printf("[E2E] RECV Non-map Result: %T %+v", resp.Result, resp.Result)
		}

		if ctx.Err() != nil {
			break
		}
	}

	// 7. Final Assertions
	if !hasSearching || !hasStructuring || !hasWriting {
		log.Printf("[E2E] FAILED: Missing status updates. Search: %v, Struct: %v, Writing: %v", hasSearching, hasStructuring, hasWriting)
		os.Exit(1)
	}
	if !hasCompleted || sessionID == "" {
		log.Println("[E2E] FAILED: Missing completion event or session_id")
		os.Exit(1)
	}

	log.Printf("[E2E] SUCCESS! Research session %s verified.", sessionID)

	// 8. Verify Q&A Mode
	log.Println("[E2E] Verifying Q&A Mode...")
	qaReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      "req-2",
		"method":  "message/stream",
		"params": map[string]any{
			"contextId": ctxID,
			"message": map[string]any{
				"kind":      "message",
				"messageId": "msg-2",
				"role":      "user",
				"parts": []map[string]any{
					{"kind": "text", "text": "What did you find?"},
				},
			},
		},
	}
	if err := conn.WriteJSON(qaReq); err != nil {
		log.Fatalf("[E2E] QA WriteJSON error: %v", err)
	}

	var hasQAAnswer bool
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Fatalf("[E2E] QA Read error: %v", err)
		}
		var resp struct {
			Result any `json:"result"`
		}
		if err := json.Unmarshal(message, &resp); err != nil {
			continue
		}

		if m, ok := resp.Result.(map[string]any); ok {
			statusMap, _ := m["status"].(map[string]any)
			if statusMap == nil {
				// Try nested Task object
				if t, ok := m["Task"].(map[string]any); ok {
					statusMap, _ = t["status"].(map[string]any)
				}
			}

			if statusMap != nil {
				state := statusMap["state"].(string)
				log.Printf("[E2E] RECV QA A2A State: %s", state)
				if state == string(a2a.TaskStateCompleted) {
					msg, _ := statusMap["message"].(map[string]any)
					if msg != nil {
						parts, _ := msg["parts"].([]any)
						for _, p := range parts {
							part := p.(map[string]any)
							if part["kind"] == "text" && strings.Contains(part["text"].(string), "Mock Answer") {
								hasQAAnswer = true
							}
						}
					}
					break
				}
			} else {
				log.Printf("[E2E] RECV QA Unknown: %+v", m)
			}
		}

		if ctx.Err() != nil {
			break
		}
	}

	if !hasQAAnswer {
		log.Println("[E2E] FAILED: Missing QA answer")
		os.Exit(1)
	}

	log.Println("[E2E] ALL TESTS PASSED!")
}
