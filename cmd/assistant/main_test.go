package main

import (
	"testing"

	"github.com/user/research-assistant/internal/event"
)

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

	sr, err := parseStructuredResearch(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if sr.Topic != "Test Topic" {
		t.Fatalf("unexpected topic: %s", sr.Topic)
	}
	if len(sr.KeyFindings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(sr.KeyFindings))
	}
	if sr.KeyFindings[0].Confidence != 1 {
		t.Fatalf("expected confidence clamped to 1, got %v", sr.KeyFindings[0].Confidence)
	}
	if len(sr.KeyFindings[0].EvidenceURLs) != 1 {
		t.Fatalf("expected evidence URL retained, got %d", len(sr.KeyFindings[0].EvidenceURLs))
	}
}

func TestParseStructuredResearch_InvalidJSON(t *testing.T) {
	_, err := parseStructuredResearch("not json")
	if err == nil {
		t.Fatal("expected error for invalid json")
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

	validateStructuredResearch(&sr)
	if sr.KeyFindings[0].Confidence != 0 {
		t.Fatalf("expected confidence clamped to 0, got %v", sr.KeyFindings[0].Confidence)
	}
	if len(sr.KeyFindings[0].EvidenceURLs) != 1 {
		t.Fatalf("expected unknown evidence dropped")
	}
}
