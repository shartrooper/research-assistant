package artifacts

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/research-assistant/internal/event"
)

type Bundle struct {
	Topic      string                   `json:"topic"`
	Summary    string                   `json:"summary"`
	Report     string                   `json:"report"`
	Sources    []event.SearchSource     `json:"sources"`
	Structured event.StructuredResearch `json:"structured"`
}

func WriteBundle(outputDir string, bundle Bundle) (string, error) {
	if strings.TrimSpace(outputDir) == "" {
		outputDir = "artifacts"
	}

	timestamp := time.Now().UTC().Format("20060102-150405")
	baseDir := filepath.Join(outputDir, safeName(bundle.Topic)+"-"+timestamp)

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", fmt.Errorf("create artifact dir: %w", err)
	}

	if err := writeMarkdown(baseDir, bundle); err != nil {
		return baseDir, err
	}
	if err := writeJSON(baseDir, bundle); err != nil {
		return baseDir, err
	}
	if err := writeCSV(baseDir, bundle); err != nil {
		return baseDir, err
	}

	return baseDir, nil
}

func writeMarkdown(dir string, bundle Bundle) error {
	filePath := filepath.Join(dir, "report.md")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("write markdown: %w", err)
	}
	defer f.Close()

	_, _ = f.WriteString("# Research Report\n\n")
	_, _ = fmt.Fprintf(f, "## Topic\n\n%s\n\n", bundle.Topic)
	_, _ = fmt.Fprintf(f, "## Executive Summary\n\n%s\n\n", bundle.Summary)
	_, _ = fmt.Fprintf(f, "## Report\n\n%s\n\n", bundle.Report)
	_, _ = f.WriteString("## Report\n\n")
	_, _ = f.WriteString(bundle.Report + "\n\n")
	if len(bundle.Structured.KeyFindings) > 0 {
		_, _ = f.WriteString("## Key Findings\n\n")
		for _, k := range bundle.Structured.KeyFindings {
			_, _ = fmt.Fprintf(f, "- %s (confidence: %.2f)\n", k.Finding, k.Confidence)
		}
		_, _ = f.WriteString("\n")
	}
	_, _ = f.WriteString("## Sources\n\n")
	for i, s := range bundle.Sources {
		_, _ = fmt.Fprintf(f, "%d. %s\n   - Query: %s\n   - Snippet: %s\n\n",
			i+1, s.URL, s.Query, s.Snippet)
	}

	return nil
}

func writeJSON(dir string, bundle Bundle) error {
	filePath := filepath.Join(dir, "report.json")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(bundle); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func writeCSV(dir string, bundle Bundle) error {
	filePath := filepath.Join(dir, "sources.csv")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("write csv: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"query", "url", "snippet"}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, s := range bundle.Sources {
		if err := w.Write([]string{s.Query, s.URL, s.Snippet}); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	w.Flush()
	return w.Error()
}

func safeName(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return "report"
	}
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return -1
	}, s)
	if s == "" {
		return "report"
	}
	return s
}
