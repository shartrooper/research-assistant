package llm

import (
	"context"
	"strings"
	"testing"

	"github.com/user/research-assistant/internal/config"
)

func TestGeminiClient_GenerateContent_Integration(t *testing.T) {
	// Load environment variables for the test
	config.LoadEnv()
	apiKey := config.GetEnv("GEMINI_API_KEY", "")

	if apiKey == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	ctx := context.Background()
	client, err := NewGeminiClient(ctx, apiKey)
	if err != nil {
		t.Fatalf("Failed to create Gemini client: %v", err)
	}
	defer client.Close()

	// Structured prompt for a research assistant task
	prompt := `
		You are a research assistant. Please provide a structured summary of the "Technical challenges the development of FZero SNES Nintendo game was faced with" in the following JSON format:
		{
			"topic": "string",
			"key_benefits": ["string"],
			"challenges": ["string"],
			"conclusion": "string"
		}
		Return ONLY the JSON.
	`

	response, err := client.GenerateContent(ctx, prompt)
	if err != nil {
		t.Fatalf("Failed to generate content: %v", err)
	}

	if response == "" {
		t.Error("Received empty response from Gemini API")
	}

	t.Logf("Gemini Response: %s", response)

	// Basic verification of structured output
	if !strings.Contains(response, "{") || !strings.Contains(response, "topic") {
		t.Errorf("Response does not appear to be structured JSON: %s", response)
	}
}
