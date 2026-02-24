package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/user/research-assistant/internal/event"
	"github.com/user/research-assistant/internal/storage"
)

// LLMClient wraps the content-generation method of a language model.
type LLMClient interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}

// SearchResult holds the output of a single web search.
type SearchResult struct {
	Content string
	URL     string
}

// SearchFunc performs a web search for the given query and returns results.
type SearchFunc func(ctx context.Context, query string) ([]SearchResult, error)

// Result holds the persistence keys produced by a completed pipeline run.
type Result struct {
	SessionID     string
	ReportMDKey   string
	ReportJSONKey string
}

// Pipeline orchestrates the full research pipeline for a single topic.
type Pipeline struct {
	llm    LLMClient
	search SearchFunc
	db     storage.StructuredStorage
	blobs  storage.BlobStorage
}

// New creates a Pipeline with the given dependencies.
func New(llm LLMClient, search SearchFunc, db storage.StructuredStorage, blobs storage.BlobStorage) *Pipeline {
	return &Pipeline{llm: llm, search: search, db: db, blobs: blobs}
}

// RunWithUpdates runs the research pipeline for the given sessionID and topic.
// onUpdate is called at each stage transition with a status string and optional detail.
// It blocks until the pipeline completes or ctx is cancelled.
// Returns persistence keys on success, nil on failure.
func (p *Pipeline) RunWithUpdates(ctx context.Context, sessionID, topic string, onUpdate func(status, detail string)) (*Result, error) {
	fail := func(detail string, err error) (*Result, error) {
		onUpdate("failed", detail)
		_ = p.db.UpdateSessionStatus(sessionID, "failed", detail)
		return nil, err
	}

	// 1. Persist session record.
	if err := p.db.CreateSession(sessionID, topic); err != nil {
		return fail(fmt.Sprintf("create session: %v", err), err)
	}

	// 2. Generate search queries via LLM.
	queryPrompt := fmt.Sprintf(
		"Given the following research topic, generate 3 specific search queries to gather comprehensive information. Return ONLY a JSON array of strings.\nTopic: %s\nReturn ONLY the JSON.", topic)
	rawQueries, err := p.llm.GenerateContent(ctx, queryPrompt)
	if err != nil {
		return fail(fmt.Sprintf("generate queries: %v", err), err)
	}
	queries := extractJSONStringArray(rawQueries)
	if len(queries) == 0 {
		queries = []string{topic}
	}

	// 3. Run searches in parallel, emitting a "searching" update per query.
	type rawResult struct {
		query   string
		content string
		url     string
	}
	ch := make(chan rawResult, len(queries))
	for _, q := range queries {
		q := q
		go func() {
			onUpdate("searching", q)
			searchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			items, err := p.search(searchCtx, q)
			if err != nil {
				log.Printf("[PIPELINE] search failed for %q: %v", q, err)
				ch <- rawResult{query: q}
				return
			}
			var sb strings.Builder
			var links []string
			for _, r := range items {
				if r.Content != "" {
					sb.WriteString(r.Content + "\n")
				}
				if r.URL != "" {
					links = append(links, r.URL)
				}
			}
			ch <- rawResult{query: q, content: sb.String(), url: strings.Join(links, " | ")}
		}()
	}

	var sources []event.SearchSource
	for range queries {
		r := <-ch
		if r.content != "" {
			sources = append(sources, event.SearchSource{Query: r.query, URL: r.url, Snippet: r.content})
		}
	}

	// 4. Structure findings via LLM.
	onUpdate("structuring", "")
	_ = p.db.UpdateSessionStatus(sessionID, "structuring", "")

	var sourceBuilder strings.Builder
	for _, s := range sources {
		sourceBuilder.WriteString(fmt.Sprintf("- Source: %s\n  Query: %s\n  Snippet: %s\n\n", s.URL, s.Query, s.Snippet))
	}
	structPrompt := fmt.Sprintf(`You are a research assistant. Convert the search results into the following JSON schema.
Return ONLY valid JSON. No commentary. No markdown.

Schema:
{
  "topic": "string",
  "key_findings": [{"finding": "string","evidence_urls": ["string"],"confidence": 0.0}],
  "challenges": ["string"],
  "open_questions": ["string"],
  "sources": [{"url": "string","query": "string","snippet": "string"}],
  "error": "string"
}

Rules:
- Use only the provided sources. evidence_urls must be URLs from sources. confidence ranges 0.0–1.0.
- If the topic is gibberish, unsafe, or disallowed, set "error" to a short explanation and return empty arrays.

Topic: %s

Sources:
%s`, topic, sourceBuilder.String())

	rawStructured, err := p.llm.GenerateContent(ctx, structPrompt)
	structured := event.StructuredResearch{
		SessionID:     sessionID,
		Topic:         topic,
		Sources:       sources,
		OpenQuestions: []string{"Structured extraction failed; using raw sources."},
	}
	if err != nil {
		log.Printf("[PIPELINE] structuring failed: %v", err)
	} else if parsed, parseErr := ParseStructuredResearch(rawStructured); parseErr == nil {
		structured = parsed
	} else {
		log.Printf("[PIPELINE] JSON parse failed: %v", parseErr)
	}
	structured.SessionID = sessionID

	// 5. Generate report and summary (skip if structured carries an error).
	onUpdate("writing_report", "")
	_ = p.db.UpdateSessionStatus(sessionID, "writing_report", "")

	var report, summary string
	if strings.TrimSpace(structured.Error) != "" {
		msg := fmt.Sprintf("Unable to perform research for this topic: %s", structured.Error)
		report = msg
		summary = msg
	} else {
		structuredJSON, _ := json.MarshalIndent(structured, "", "  ")
		reportPrompt := fmt.Sprintf(`You are a research assistant. Write a comprehensive report based only on the structured data below.
Include key insights, challenges, and a conclusion.
Structured Data:
%s`, string(structuredJSON))

		report, err = p.llm.GenerateContent(ctx, reportPrompt)
		if err != nil {
			return fail(fmt.Sprintf("generate report: %v", err), err)
		}

		summaryPrompt := fmt.Sprintf(`Create a short executive summary (3-5 bullet points) for the following report. Return plain text bullets.
Report:
%s`, report)
		summary, err = p.llm.GenerateContent(ctx, summaryPrompt)
		if err != nil {
			summary = "Executive summary unavailable due to generation error."
		}
	}

	fullReport := fmt.Sprintf("RESEARCH REPORT\n===============\n%s", report)

	// 6. Persist blobs.
	reportMDKey, err := p.blobs.SaveBlob("report", []byte(fullReport), "md")
	if err != nil {
		log.Printf("[PIPELINE] save report.md failed: %v", err)
	}

	type bundleJSON struct {
		Topic      string                   `json:"topic"`
		Summary    string                   `json:"summary"`
		Report     string                   `json:"report"`
		Sources    []event.SearchSource     `json:"sources"`
		Structured event.StructuredResearch `json:"structured"`
	}
	bundleBytes, _ := json.MarshalIndent(bundleJSON{
		Topic:      topic,
		Summary:    summary,
		Report:     fullReport,
		Sources:    sources,
		Structured: structured,
	}, "", "  ")
	reportJSONKey, err := p.blobs.SaveBlob("report", bundleBytes, "json")
	if err != nil {
		log.Printf("[PIPELINE] save report.json failed: %v", err)
	}

	// 7. Persist structured data to DB.
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); _ = p.db.SaveFindings(sessionID, structured.KeyFindings) }()
	go func() { defer wg.Done(); _ = p.db.SaveOpenQuestions(sessionID, structured.OpenQuestions) }()
	go func() { defer wg.Done(); _ = p.db.SaveSources(sessionID, structured.Sources) }()
	wg.Wait()
	_ = p.db.MarkSessionComplete(sessionID, reportMDKey, reportJSONKey)

	onUpdate("complete", reportMDKey)
	return &Result{SessionID: sessionID, ReportMDKey: reportMDKey, ReportJSONKey: reportJSONKey}, nil
}

// extractJSONStringArray extracts a JSON string array from raw LLM output,
// tolerating markdown code-block wrappers. Falls back to nil if parsing fails.
func extractJSONStringArray(raw string) []string {
	s := strings.TrimSpace(raw)
	if idx := strings.Index(s, "["); idx != -1 {
		s = s[idx:]
	}
	if idx := strings.LastIndex(s, "]"); idx != -1 {
		s = s[:idx+1]
	}
	var queries []string
	if err := json.Unmarshal([]byte(s), &queries); err != nil {
		return nil
	}
	return queries
}

// ParseStructuredResearch parses and validates the raw LLM JSON output into a
// StructuredResearch value. Confidence scores are clamped to [0,1], evidence
// URLs are validated against the sources list, and an empty topic is replaced
// with "Unknown Topic".
func ParseStructuredResearch(raw string) (event.StructuredResearch, error) {
	cleaned := strings.TrimSpace(raw)
	start := strings.Index(cleaned, "{")
	end := strings.LastIndex(cleaned, "}")
	if start == -1 || end == -1 || end <= start {
		return event.StructuredResearch{}, fmt.Errorf("no JSON object found in LLM output")
	}

	cleaned = cleaned[start : end+1]
	var sr event.StructuredResearch
	if err := json.Unmarshal([]byte(cleaned), &sr); err != nil {
		return event.StructuredResearch{}, err
	}
	if sr.Topic == "" {
		sr.Topic = "Unknown Topic"
	}
	if sr.Error != "" {
		sr.KeyFindings = nil
		sr.Challenges = nil
		sr.OpenQuestions = nil
		sr.Sources = nil
	}
	ValidateStructuredResearch(&sr)
	return sr, nil
}

// ValidateStructuredResearch normalises sources and key findings in place:
// trims whitespace, clamps confidence to [0,1], and removes evidence URLs
// that do not appear in the sources list.
func ValidateStructuredResearch(sr *event.StructuredResearch) {
	sourceSet := make(map[string]struct{}, len(sr.Sources))
	for i := range sr.Sources {
		s := &sr.Sources[i]
		s.URL = strings.TrimSpace(s.URL)
		s.Query = strings.TrimSpace(s.Query)
		s.Snippet = strings.TrimSpace(s.Snippet)
		if s.URL != "" {
			sourceSet[s.URL] = struct{}{}
		}
	}

	for i := range sr.KeyFindings {
		f := &sr.KeyFindings[i]
		f.Finding = strings.TrimSpace(f.Finding)
		if f.Confidence < 0 {
			f.Confidence = 0
		}
		if f.Confidence > 1 {
			f.Confidence = 1
		}
		if len(sourceSet) == 0 {
			continue
		}
		filtered := make([]string, 0, len(f.EvidenceURLs))
		for _, u := range f.EvidenceURLs {
			u = strings.TrimSpace(u)
			if _, ok := sourceSet[u]; ok {
				filtered = append(filtered, u)
			}
		}
		f.EvidenceURLs = filtered
	}
}
