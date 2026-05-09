package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	apperrors "github.com/user/research-assistant/internal/errors"
	"google.golang.org/api/googleapi"
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
			return "", g.wrapError(err)
		}
	}

	if len(resp.Candidates) == 0 {
		return "", apperrors.New(apperrors.CodeInternalFailure, "llm", "No response generated", nil)
	}

	candidate := resp.Candidates[0]
	if candidate.FinishReason == genai.FinishReasonSafety {
		appErr := apperrors.New(apperrors.CodePolicyViolation, "llm", "Content filtered due to safety policies.", nil)
		appErr.Recovery = &apperrors.RecoveryAction{Type: apperrors.RecoveryRephrase}
		appErr.Telemetry = map[string]any{
			"finish_reason": "safety",
		}
		return "", appErr
	}

	var result string
	if candidate.Content != nil {
		for _, part := range candidate.Content.Parts {
			if text, ok := part.(genai.Text); ok {
				result += string(text)
			}
		}
	}

	if result == "" {
		return "", apperrors.New(apperrors.CodeInternalFailure, "llm", "Empty response from provider", nil)
	}

	return result, nil
}

func (g *GeminiClient) wrapError(err error) error {
	var gErr *googleapi.Error
	if errors.As(err, &gErr) {
		if gErr.Code == 429 {
			appErr := apperrors.New(apperrors.CodeQuotaExceeded, "llm", "Daily request limit reached.", err)
			appErr.Recovery = &apperrors.RecoveryAction{
				Type:        apperrors.RecoveryWait,
				WaitSeconds: 60, // Default wait
			}
			return appErr
		}
		if gErr.Code >= 500 {
			return apperrors.New(apperrors.CodeProviderUnavailable, "llm", "Model provider is currently overloaded.", err)
		}
	}

	if strings.Contains(err.Error(), "quota") {
		return apperrors.New(apperrors.CodeQuotaExceeded, "llm", "Quota exceeded.", err)
	}

	return apperrors.New(apperrors.CodeInternalFailure, "llm", "An unexpected error occurred during generation.", err)
}
