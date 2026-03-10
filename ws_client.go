package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	url := "ws://localhost:8080/ws"
	fmt.Printf("Connecting to %s...\n", url)

	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	// Start reading
	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			fmt.Printf("RECV: %s\n", message)
		}
	}()

	// Send an A2A message request
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      "test-1",
		"method":  "message/stream",
		"params": map[string]any{
			"contextId": "ctx-1",
			"taskId":    "task-1",
			"message": map[string]any{
				"kind":      "message",
				"messageId": "msg-1",
				"role":      "user",
				"parts": []map[string]any{
					{"kind": "text", "text": "What is the future of Go concurrency?"},
				},
			},
		},
	}

	b, _ := json.Marshal(req)
	err = c.WriteMessage(websocket.TextMessage, b)
	if err != nil {
		log.Println("write:", err)
		return
	}

	// Wait a few seconds to receive searching/structuring updates
	time.Sleep(5 * time.Second)
	c.Close()
	<-done
}
