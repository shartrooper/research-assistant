package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Options struct {
	Safe string // off|medium|active
	Num  int    // max results 1-10
}

type SearchResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

type SearchResponse struct {
	Items []SearchResult `json:"items"`
}

func SearchWeb(ctx context.Context, apiKey, cx, query string, opts Options) ([]SearchResult, error) {
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(cx) == "" {
		return nil, fmt.Errorf("missing CSE key or cx")
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("empty query")
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
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cse http %d", resp.StatusCode)
	}

	var sr SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}
	if len(sr.Items) == 0 {
		return nil, fmt.Errorf("no results")
	}

	return sr.Items, nil
}
