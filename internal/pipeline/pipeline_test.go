package pipeline_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/user/research-assistant/internal/event"
	"github.com/user/research-assistant/internal/pipeline"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockLLM struct {
	mu        sync.Mutex
	responses []string
	idx       int
	err       error
}

func (m *mockLLM) GenerateContent(_ context.Context, _ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return "", m.err
	}
	if m.idx >= len(m.responses) {
		return "", fmt.Errorf("mockLLM: no more responses (call %d)", m.idx)
	}
	r := m.responses[m.idx]
	m.idx++
	return r, nil
}

type mockSearcher struct {
	mu      sync.Mutex
	results []pipeline.SearchResult
	err     error
	errIdx  int // -1 = never fail; >=0 = fail on that call index
	calls   int
}

func (m *mockSearcher) search(_ context.Context, _ string) ([]pipeline.SearchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.calls
	m.calls++
	if m.err != nil && (m.errIdx < 0 || idx == m.errIdx) {
		return nil, m.err
	}
	return m.results, nil
}

type mockDB struct{}

func (m *mockDB) CreateSession(_, _ string) error                                      { return nil }
func (m *mockDB) UpdateSessionStatus(_, _, _ string) error                             { return nil }
func (m *mockDB) SaveFindings(_ string, _ []event.StructuredFinding) error             { return nil }
func (m *mockDB) SaveOpenQuestions(_ string, _ []string) error                         { return nil }
func (m *mockDB) SaveSources(_ string, _ []event.SearchSource) error                   { return nil }
func (m *mockDB) MarkSessionComplete(_, _, _ string) error                             { return nil }

type mockBlob struct{}

func (m *mockBlob) SaveBlob(_ string, _ []byte, _ string) (string, error) {
	return "mock-key", nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func collectStatuses(onUpdate func(status, detail string)) (func(string, string), *[]string, *sync.Mutex) {
	var mu sync.Mutex
	var statuses []string
	cb := func(status, detail string) {
		mu.Lock()
		statuses = append(statuses, status)
		mu.Unlock()
		if onUpdate != nil {
			onUpdate(status, detail)
		}
	}
	return cb, &statuses, &mu
}

func lastIndexOf(ss []string, target string) int {
	for i := len(ss) - 1; i >= 0; i-- {
		if ss[i] == target {
			return i
		}
	}
	return -1
}

func firstIndexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}

func countOf(ss []string, target string) int {
	n := 0
	for _, s := range ss {
		if s == target {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Pipeline behaviour tests
// ---------------------------------------------------------------------------

// TestPipeline_EmitsExpectedStatusSequence verifies the full happy-path status
// sequence: three "searching" updates (one per query), then "structuring",
// "writing_report", and "complete" — in that order.
func TestPipeline_EmitsExpectedStatusSequence(t *testing.T) {
	lm := &mockLLM{
		responses: []string{
			// 1. query generation — three queries
			`["query1","query2","query3"]`,
			// 2. structuring — valid JSON
			`{"topic":"T","key_findings":[{"finding":"F","evidence_urls":["http://a.com"],"confidence":0.9}],"challenges":[],"open_questions":[],"sources":[{"url":"http://a.com","query":"query1","snippet":"s"}],"error":""}`,
			// 3. report generation
			"Full report content",
			// 4. executive summary
			"• Bullet 1\n• Bullet 2",
		},
	}
	ms := &mockSearcher{
		results: []pipeline.SearchResult{{Content: "content", URL: "http://a.com"}},
		errIdx:  -1,
	}

	p := pipeline.New(lm, ms.search, &mockDB{}, &mockBlob{})
	cb, statuses, mu := collectStatuses(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := p.RunWithUpdates(ctx, uuid.New().String(), "Test Topic", cb)
	if err != nil {
		t.Fatalf("RunWithUpdates returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("RunWithUpdates returned nil result on success")
	}

	mu.Lock()
	defer mu.Unlock()

	if countOf(*statuses, "searching") != 3 {
		t.Errorf("expected 3 'searching' updates, got %d; all: %v", countOf(*statuses, "searching"), *statuses)
	}
	if countOf(*statuses, "structuring") != 1 {
		t.Errorf("expected 1 'structuring' update, got %d; all: %v", countOf(*statuses, "structuring"), *statuses)
	}
	if countOf(*statuses, "writing_report") != 1 {
		t.Errorf("expected 1 'writing_report' update, got %d; all: %v", countOf(*statuses, "writing_report"), *statuses)
	}
	if countOf(*statuses, "complete") != 1 {
		t.Errorf("expected 1 'complete' update, got %d; all: %v", countOf(*statuses, "complete"), *statuses)
	}

	lastSearch := lastIndexOf(*statuses, "searching")
	firstStruct := firstIndexOf(*statuses, "structuring")
	firstReport := firstIndexOf(*statuses, "writing_report")
	firstComplete := firstIndexOf(*statuses, "complete")

	if lastSearch >= firstStruct {
		t.Errorf("all 'searching' must precede 'structuring'; statuses: %v", *statuses)
	}
	if firstStruct >= firstReport {
		t.Errorf("'structuring' must precede 'writing_report'; statuses: %v", *statuses)
	}
	if firstReport >= firstComplete {
		t.Errorf("'writing_report' must precede 'complete'; statuses: %v", *statuses)
	}
	if (*statuses)[len(*statuses)-1] != "complete" {
		t.Errorf("'complete' must be the last status; statuses: %v", *statuses)
	}
}

// TestPipeline_HandlesGeminiFailure checks that a hard LLM error on query
// generation causes the pipeline to emit "failed" and return a non-nil error.
func TestPipeline_HandlesGeminiFailure(t *testing.T) {
	lm := &mockLLM{err: fmt.Errorf("gemini: quota exceeded")}
	ms := &mockSearcher{results: []pipeline.SearchResult{{Content: "c", URL: "http://b.com"}}, errIdx: -1}

	p := pipeline.New(lm, ms.search, &mockDB{}, &mockBlob{})
	cb, statuses, mu := collectStatuses(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := p.RunWithUpdates(ctx, uuid.New().String(), "Test Topic", cb)

	mu.Lock()
	defer mu.Unlock()

	hasFailed := countOf(*statuses, "failed") > 0
	if err == nil && !hasFailed {
		t.Fatalf("expected non-nil error or 'failed' status; got statuses: %v, err: %v", *statuses, err)
	}
	if result != nil {
		t.Errorf("expected nil result on failure, got: %+v", result)
	}
}

// TestPipeline_SearchPartialFailure verifies that one failing search does not
// abort the pipeline — it should complete successfully using the remaining results.
func TestPipeline_SearchPartialFailure(t *testing.T) {
	lm := &mockLLM{
		responses: []string{
			`["query1","query2","query3"]`,
			`{"topic":"T","key_findings":[],"challenges":[],"open_questions":[],"sources":[{"url":"http://c.com","query":"query2","snippet":"s"}],"error":""}`,
			"Report",
			"Summary",
		},
	}
	ms := &mockSearcher{
		results: []pipeline.SearchResult{{Content: "content", URL: "http://c.com"}},
		err:     fmt.Errorf("search: connection timeout"),
		errIdx:  1, // only the second search fails
	}

	p := pipeline.New(lm, ms.search, &mockDB{}, &mockBlob{})
	cb, statuses, mu := collectStatuses(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := p.RunWithUpdates(ctx, uuid.New().String(), "Test Topic", cb)
	if err != nil {
		t.Fatalf("expected pipeline to complete despite partial search failure; got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result despite partial search failure")
	}

	mu.Lock()
	defer mu.Unlock()

	if countOf(*statuses, "complete") != 1 {
		t.Errorf("expected 'complete' status; got: %v", *statuses)
	}
}

// ---------------------------------------------------------------------------
// Parser / validator unit tests (functions promoted from cmd/assistant)
// ---------------------------------------------------------------------------

func TestParseStructuredResearch_Valid(t *testing.T) {
	raw := `{
		"topic": "Test Topic",
		"key_findings": [
			{
				"finding": "Finding A",
				"evidence_urls": ["https://example.com/a"],
				"confidence": 1.2
			}
		],
		"challenges": ["C1"],
		"open_questions": ["Q1"],
		"sources": [
			{
				"url": "https://example.com/a",
				"query": "query a",
				"snippet": "snippet a"
			}
		]
	}`

	sr, err := pipeline.ParseStructuredResearch(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if sr.Topic != "Test Topic" {
		t.Fatalf("unexpected topic: %s", sr.Topic)
	}
	if len(sr.KeyFindings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(sr.KeyFindings))
	}
	if sr.KeyFindings[0].Confidence != 1.0 {
		t.Fatalf("expected confidence clamped to 1.0, got %v", sr.KeyFindings[0].Confidence)
	}
	if len(sr.KeyFindings[0].EvidenceURLs) != 1 {
		t.Fatalf("expected evidence URL retained, got %d URLs", len(sr.KeyFindings[0].EvidenceURLs))
	}
}

func TestParseStructuredResearch_InvalidJSON(t *testing.T) {
	_, err := pipeline.ParseStructuredResearch("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestValidateStructuredResearch_DropsUnknownEvidence(t *testing.T) {
	sr := event.StructuredResearch{
		Topic: "T",
		Sources: []event.SearchSource{
			{URL: "https://source.com/a"},
		},
		KeyFindings: []event.StructuredFinding{
			{
				Finding:      "F",
				EvidenceURLs: []string{"https://source.com/a", "https://bad.com/b"},
				Confidence:   -0.2,
			},
		},
	}

	pipeline.ValidateStructuredResearch(&sr)

	if sr.KeyFindings[0].Confidence != 0 {
		t.Fatalf("expected confidence clamped to 0, got %v", sr.KeyFindings[0].Confidence)
	}
	if len(sr.KeyFindings[0].EvidenceURLs) != 1 {
		t.Fatalf("expected unknown evidence URL dropped; got %d URLs", len(sr.KeyFindings[0].EvidenceURLs))
	}
}
