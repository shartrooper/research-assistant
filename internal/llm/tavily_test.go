package llm

import (
	"context"
	"testing"

	"github.com/user/research-assistant/internal/config"
)

func TestTavilyClient_Search_Integration(t *testing.T) {
	// Load environment variables for the test
	config.LoadEnv()
	apiKey := config.GetEnv("TAVILY_API_KEY", "")

	if apiKey == "" {
		t.Skip("Skipping integration test: TAVILY_API_KEY not set")
	}

	client := NewTavilyClient(apiKey)
	ctx := context.Background()

	query := "Current state of Go 1.24 release"
	resp, err := client.Search(ctx, query)
	if err != nil {
		t.Fatalf("Tavily search failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Received nil response from Tavily API")
	}

	t.Logf("Tavily found %d results", len(resp.Results))

	if len(resp.Results) > 0 {
		for i, res := range resp.Results {
			t.Logf("Result %d: %s (%s)", i+1, res.Title, res.URL)
			if res.Content == "" {
				t.Errorf("Result %d has empty content", i+1)
			}
		}
	} else {
		t.Error("Tavily returned 0 results for a valid query")
	}
}
