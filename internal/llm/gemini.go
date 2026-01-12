package llm

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type GeminiClient struct {
	client *genai.Client
	model  *genai.GenerativeModel
}

func NewGeminiClient(ctx context.Context, apiKey string) (*GeminiClient, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	model := client.GenerativeModel("gemini-2.5-flash")

	return &GeminiClient{
		client: client,
		model:  model,
	}, nil
}

func (g *GeminiClient) Close() error {
	return g.client.Close()
}

func (g *GeminiClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	resp, err := g.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		fmt.Printf("[LLM] Primary model failed, attempting fallback to gemini-2.5-flash-lite: %v\n", err)
		fallbackModel := g.client.GenerativeModel("gemini-2.5-flash-lite")
		resp, err = fallbackModel.GenerateContent(ctx, genai.Text(prompt))
		if err != nil {
			return "", fmt.Errorf("primary and fallback models failed: %w", err)
		}
	}

	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("no candidates returned from gemini")
	}

	var result string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			result += string(text)
		}
	}

	if result == "" {
		return "", fmt.Errorf("empty response from gemini")
	}

	return result, nil
}
