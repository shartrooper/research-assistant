package search

import (
	"context"
	"fmt"
	"testing"

	"github.com/user/research-assistant/internal/config"
)

func TestSearchWeb_Integration(t *testing.T) {
	config.LoadEnv()
	apiKey := config.GetEnv("CSE_API_KEY", "")
	cx := config.GetEnv("CSE_CX", "")

	if apiKey == "" || cx == "" {
		t.Skip("Skipping integration test: CSE_API_KEY or CSE_CX not set")
	}

	ctx := context.Background()
	results, err := SearchWeb(ctx, apiKey, cx, "Go 1.24 release features", Options{Num: 3, Safe: "off"})
	fmt.Println(results)

	if err != nil {
		t.Fatalf("CSE search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("CSE returned 0 results for a valid query")
	}
}
