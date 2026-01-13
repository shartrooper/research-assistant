package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TavilyClient is a simple client for the Tavily Search API
type TavilyClient struct {
	APIKey string
	HTTP   *http.Client
}

// NewTavilyClient creates a new Tavily client
func NewTavilyClient(apiKey string) *TavilyClient {
	return &TavilyClient{
		APIKey: apiKey,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TavilySearchRequest is the payload for the Tavily Search API
type TavilySearchRequest struct {
	Query string `json:"query"`
}

// TavilySearchResult is a single result from the Tavily API
type TavilySearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// TavilySearchResponse is the response from the Tavily API
type TavilySearchResponse struct {
	Answer  string               `json:"answer,omitempty"`
	Results []TavilySearchResult `json:"results"`
}

// Search performs a web search using Tavily
func (t *TavilyClient) Search(ctx context.Context, query string) (*TavilySearchResponse, error) {
	reqBody := TavilySearchRequest{
		Query: query,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.tavily.com/search", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	cleanKey := strings.TrimSpace(t.APIKey)
	req.Header.Set("Authorization", "Bearer "+cleanKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := t.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily api error: status %d", resp.StatusCode)
	}

	var searchResp TavilySearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &searchResp, nil
}
