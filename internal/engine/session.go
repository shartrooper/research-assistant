package engine

import (
	"sync"

	"github.com/user/research-assistant/internal/event"
)

// ResearchSession tracks the state of a multi-step research task
type ResearchSession struct {
	ID             string
	OriginalPrompt string
	TotalTasks     int
	CompletedTasks int
	Results        []event.SearchResponse
	mu             sync.Mutex
}

// NewSession creates a new research session
func NewSession(id string, prompt string, totalTasks int) *ResearchSession {
	return &ResearchSession{
		ID:             id,
		OriginalPrompt: prompt,
		TotalTasks:     totalTasks,
		Results:        make([]event.SearchResponse, 0, totalTasks),
	}
}

// AddResult records a result and returns true if all tasks are complete
func (s *ResearchSession) AddResult(res event.SearchResponse) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Results = append(s.Results, res)
	s.CompletedTasks++

	return s.CompletedTasks >= s.TotalTasks
}

// SessionManager handles multiple concurrent research sessions
type SessionManager struct {
	sessions map[string]*ResearchSession
	mu       sync.RWMutex
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*ResearchSession),
	}
}

// CreateSession initializes and stores a new session
func (m *SessionManager) CreateSession(id string, prompt string, totalTasks int) *ResearchSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := NewSession(id, prompt, totalTasks)
	m.sessions[id] = s
	return s
}

// GetSession retrieves a session by ID
func (m *SessionManager) GetSession(id string) (*ResearchSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[id]
	return s, ok
}

// DeleteSession removes a session from tracking
func (m *SessionManager) DeleteSession(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, id)
}
