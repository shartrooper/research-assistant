package storage_test

import (
	"testing"

	"github.com/user/research-assistant/internal/event"
	"github.com/user/research-assistant/internal/storage"
)

// newTestStore creates an in-memory SQLiteStore for use in tests.
func newTestStore(t *testing.T) *storage.SQLiteStore {
	t.Helper()
	s, err := storage.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSQLiteStore_GetKeyFindings(t *testing.T) {
	s := newTestStore(t)
	const sessionID = "test-session-1"

	if err := s.CreateSession(sessionID, "Test Topic"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	findings := []event.StructuredFinding{
		{Finding: "F1", EvidenceURLs: []string{"http://a.com"}, Confidence: 0.9},
		{Finding: "F2", EvidenceURLs: nil, Confidence: 0.5},
	}
	if err := s.SaveFindings(sessionID, findings); err != nil {
		t.Fatalf("SaveFindings: %v", err)
	}

	got, err := s.GetKeyFindings(sessionID)
	if err != nil {
		t.Fatalf("GetKeyFindings: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(got))
	}
	if got[0].Finding != "F1" {
		t.Errorf("finding[0]: want %q, got %q", "F1", got[0].Finding)
	}
	if got[1].Confidence != 0.5 {
		t.Errorf("finding[1] confidence: want 0.5, got %v", got[1].Confidence)
	}
}

func TestSQLiteStore_GetKeyFindings_Empty(t *testing.T) {
	s := newTestStore(t)
	const sessionID = "test-session-empty"
	if err := s.CreateSession(sessionID, "Empty"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := s.GetKeyFindings(sessionID)
	if err != nil {
		t.Fatalf("GetKeyFindings on empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 findings for session with no data, got %d", len(got))
	}
}

func TestSQLiteStore_GetSources(t *testing.T) {
	s := newTestStore(t)
	const sessionID = "test-session-2"

	if err := s.CreateSession(sessionID, "Test Topic"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	sources := []event.SearchSource{
		{Query: "q1", URL: "http://a.com", Snippet: "snippet a"},
		{Query: "q2", URL: "http://b.com", Snippet: "snippet b"},
		{Query: "q3", URL: "http://c.com", Snippet: ""},
	}
	if err := s.SaveSources(sessionID, sources); err != nil {
		t.Fatalf("SaveSources: %v", err)
	}

	got, err := s.GetSources(sessionID)
	if err != nil {
		t.Fatalf("GetSources: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(got))
	}
	if got[0].URL != "http://a.com" {
		t.Errorf("source[0].URL: want %q, got %q", "http://a.com", got[0].URL)
	}
	if got[2].Snippet != "" {
		t.Errorf("source[2].Snippet: want empty, got %q", got[2].Snippet)
	}
}
