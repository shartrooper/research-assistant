package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/user/research-assistant/internal/errors"
)

type Options struct {
	Safe string // off|medium|active
	Num  int    // max results 1-10
}

type ContentResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

type ContentResponse struct {
	Items []ContentResult `json:"items"`
}

func ContentWebSearch(ctx context.Context, apiKey, cx, query string, opts Options) ([]ContentResult, error) {
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(cx) == "" {
		return nil, errors.New(errors.CodeInternalFailure, "search", "Search provider configuration missing.", nil)
	}
	if strings.TrimSpace(query) == "" {
		return nil, errors.New(errors.CodeQueryInvalid, "search", "Search query is empty.", nil)
	}
	if opts.Num <= 0 || opts.Num > 10 {
		opts.Num = 5
	}

	u, _ := url.Parse("https://customsearch.googleapis.com/customsearch/v1")
	q := u.Query()
	q.Set("key", apiKey)
	q.Set("cx", cx)
	q.Set("q", query)
	q.Set("num", fmt.Sprintf("%d", opts.Num))
	if opts.Safe != "" {
		q.Set("safe", opts.Safe)
	}
	u.RawQuery = q.Encode()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.New(errors.CodeProviderUnavailable, "search", "Failed to connect to search provider.", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		appErr := errors.New(errors.CodeQuotaExceeded, "search", "Search quota exceeded.", nil)
		appErr.Recovery = &errors.RecoveryAction{Type: errors.RecoveryWait, WaitSeconds: 60}
		return nil, appErr
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(errors.CodeProviderUnavailable, "search", fmt.Sprintf("Search provider returned error status: %d", resp.StatusCode), nil)
	}

	var sr ContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, errors.New(errors.CodeInternalFailure, "search", "Failed to decode search response.", err)
	}
	if len(sr.Items) == 0 {
		return nil, errors.New(errors.CodeQueryInvalid, "search", "No results found for your query.", nil)
	}

	return sr.Items, nil
}
